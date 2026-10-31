package bridge

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	blogapp "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/app"
	blogcompiler "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/compiler"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/internal/version"
	blogplatform "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/platform"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/publisher"
	blogsearch "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/search"
)

type articleSummary struct {
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	Language      string `json:"language"`
	EnglishMirror bool   `json:"englishMirror"`
	SourcePath    string `json:"sourcePath"`
}

type explicitTaskTarget struct {
	Platform  string `json:"platform"`
	ID        string `json:"id"`
	URL       string `json:"url,omitempty"`
	State     string `json:"state"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type syncRequest struct {
	Targets                []explicitTaskTarget `json:"targets,omitempty"`
	RequestID              string               `json:"requestId,omitempty"`
	PublishAfter           bool                 `json:"publishAfter,omitempty"`
	Article                string               `json:"article"`
	Platforms              []string             `json:"platforms"`
	DryRun                 bool                 `json:"dryRun"`
	Changed                bool                 `json:"changed"`
	UsePlatformChangedOnly bool                 `json:"usePlatformChangedOnly,omitempty"`
	Draft                  bool                 `json:"draft"`
	Operation              string               `json:"operation,omitempty"`
	ResumeMediumDraftID    string               `json:"resumeMediumDraftId,omitempty"`
}

type syncPlatformResult struct {
	State   string `json:"state"`
	Result  string `json:"result,omitempty"`
	URL     string `json:"url,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type syncJobEvent struct {
	TargetID string `json:"targetId,omitempty"`
	At       string `json:"at"`
	Platform string `json:"platform"`
	State    string `json:"state"`
	Result   string `json:"result,omitempty"`
	URL      string `json:"url,omitempty"`
	Message  string `json:"message,omitempty"`
}

type syncJob struct {
	ID         string                        `json:"id"`
	Article    string                        `json:"article"`
	Platforms  []string                      `json:"platforms"`
	Operation  string                        `json:"operation"`
	Request    syncRequest                   `json:"-"`
	Results    map[string]syncPlatformResult `json:"results"`
	Events     []syncJobEvent                `json:"events,omitempty"`
	State      string                        `json:"state"`
	StartedAt  string                        `json:"startedAt"`
	FinishedAt string                        `json:"finishedAt,omitempty"`
	Output     string                        `json:"output,omitempty"`
	Error      string                        `json:"error,omitempty"`
	DryRun     bool                          `json:"dryRun"`
}

type toolField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Default     any      `json:"default,omitempty"`
	Min         int      `json:"min,omitempty"`
	Max         int      `json:"max,omitempty"`
	Options     []string `json:"options,omitempty"`
}

type toolToggle struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type toolAction struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type toolConfigView struct {
	Scope           string         `json:"scope"`
	Values          map[string]any `json:"values"`
	Schema          []toolField    `json:"schema,omitempty"`
	Toggle          *toolToggle    `json:"toggle,omitempty"`
	DefaultExpanded bool           `json:"defaultExpanded,omitempty"`
}

type toolHealth struct {
	OK      bool   `json:"ok"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

type toolDescriptor struct {
	Name        string         `json:"name"`
	DisplayName string         `json:"displayName"`
	Kind        string         `json:"kind"`
	Description string         `json:"description"`
	Required    bool           `json:"required"`
	Actions     []toolAction   `json:"actions,omitempty"`
	Health      toolHealth     `json:"health"`
	Config      toolConfigView `json:"config"`
}

type toolConfigRequest struct {
	Config map[string]any `json:"config"`
}

type publishingPlatformView struct {
	ID           string                         `json:"id"`
	Label        string                         `json:"label"`
	Language     string                         `json:"language"`
	ChangedOnly  bool                           `json:"changedOnly"`
	Capabilities publisher.PlatformCapabilities `json:"capabilities"`
	Footer       publishingFooterConfig         `json:"footer"`
	Canonical    publishingCanonicalConfig      `json:"canonical"`
	Tracking     publishingTrackingConfig       `json:"tracking"`
}

func readFrontmatterScalar(path, name string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inFrontmatter := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break
		}
		if !inFrontmatter {
			continue
		}
		prefix := name + ":"
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		value = strings.Trim(value, "\"'")
		return value
	}
	return ""
}

func listArticles(contentRoot string) ([]articleSummary, error) {
	if contentRoot == "" {
		return nil, errors.New("content repository path is not configured")
	}
	articleRoot := filepath.Join(contentRoot, "src", "content", "articles")
	if info, err := os.Stat(articleRoot); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("article directory was not found: %s", articleRoot)
	}

	articles := []articleSummary{}
	err := filepath.WalkDir(articleRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(articleRoot, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if relative == "en" || strings.HasPrefix(relative, "en"+string(filepath.Separator)) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			return nil
		}
		status := readFrontmatterScalar(path, "status")
		if status != "" && status != "published" {
			return nil
		}
		slug := strings.TrimSuffix(filepath.ToSlash(relative), filepath.Ext(relative))
		title := readFrontmatterScalar(path, "title")
		if title == "" {
			title = slug
		}
		englishPath := filepath.Join(articleRoot, "en", filepath.FromSlash(slug)+".md")
		englishStatus := readFrontmatterScalar(englishPath, "status")
		englishMirror := filePresent(englishPath) && (englishStatus == "" || englishStatus == "published")
		articles = append(articles, articleSummary{
			Slug: slug, Title: title, Status: "published", Language: "zh-CN",
			EnglishMirror: englishMirror, SourcePath: filepath.ToSlash(relative),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(articles, func(i, j int) bool { return articles[i].Slug < articles[j].Slug })
	return articles, nil
}

func isEngineWorkspace(path string) bool {
	return filePresent(filepath.Join(path, "package.json")) && filePresent(filepath.Join(path, "astro.config.mjs"))
}

func isContentWorkspace(path string) bool {
	return directoryPresent(filepath.Join(path, "src", "content", "articles")) && filePresent(filepath.Join(path, "src", "data", "content-manifest.json"))
}

func filePresent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func directoryPresent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func distributionRootHealth(path string) toolHealth {
	path = strings.TrimSpace(path)
	if path == "" {
		return toolHealth{Status: "missing", Summary: "未配置路径"}
	}
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return toolHealth{Status: "error", Summary: "路径无效", Path: path, Detail: "Distribution 必须是目录"}
		}
		return toolHealth{OK: true, Status: "ok", Summary: "可用", Path: path}
	}
	if errors.Is(err, os.ErrNotExist) {
		return toolHealth{OK: true, Status: "ok", Summary: "待创建", Path: path}
	}
	return toolHealth{Status: "error", Summary: "无法访问", Path: path, Detail: err.Error()}
}

func publicationBindingsHealth(path string) toolHealth {
	path = strings.TrimSpace(path)
	if path == "" {
		return toolHealth{Status: "missing", Summary: "未配置路径"}
	}
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return toolHealth{Status: "error", Summary: "路径无效", Path: path, Detail: "Publication Bindings 必须是文件路径"}
		}
		return toolHealth{OK: true, Status: "ok", Summary: "已加载", Path: path}
	}
	if errors.Is(err, os.ErrNotExist) {
		return toolHealth{OK: true, Status: "ok", Summary: "待创建", Path: path}
	}
	return toolHealth{Status: "error", Summary: "无法访问", Path: path, Detail: err.Error()}
}

func workspaceHealth(path, kind string) toolHealth {
	if path == "" {
		return toolHealth{Status: "missing", Summary: "未配置路径"}
	}
	valid := isEngineWorkspace(path)
	if kind == "content" {
		valid = isContentWorkspace(path)
	}
	if !valid {
		return toolHealth{Status: "error", Summary: "路径无效", Path: path}
	}
	return toolHealth{OK: true, Status: "ok", Summary: "工作区正常", Path: path}
}

func configuredExecutable(config bridgeConfig, name string) (string, error) {
	configured := strings.TrimSpace(config.ToolPaths[name])
	if configured != "" {
		info, err := os.Stat(configured)
		if err != nil || info.IsDir() {
			return "", fmt.Errorf("configured %s executable was not found: %s", name, configured)
		}
		return configured, nil
	}
	return exec.LookPath(name)
}

func executableHealth(config bridgeConfig, name string) toolHealth {
	path, err := configuredExecutable(config, name)
	if err != nil {
		return toolHealth{Status: "missing", Summary: "未检测到", Detail: err.Error()}
	}
	version, detail := dependencyVersion(config, name)
	if name == "node" && config.ProxyEnabled && version != "" && !nodeVersionSupportsProxy(version) {
		return toolHealth{
			Status: "error", Summary: "版本不兼容", Path: path, Version: version,
			Detail: "BlogCTL Network Proxy 需要 Node.js 22.21+ 或 24+；请点击更新切换到当前 LTS。",
		}
	}
	return toolHealth{OK: true, Status: "ok", Summary: "可用", Path: path, Version: version, Detail: detail}
}

func devtoAPIKey(config bridgeConfig) string {
	return strings.TrimSpace(config.DevtoAPIKey)
}

func devtoAPIHealth(config bridgeConfig) toolHealth {
	if devtoAPIKey(config) == "" {
		return toolHealth{Status: "missing", Summary: "API Key 未配置"}
	}
	return toolHealth{OK: true, Status: "ok", Summary: "已配置"}
}

func devtoAPIPlaceholder(config bridgeConfig) string {
	if devtoAPIKey(config) != "" {
		return "已配置；留空保存时保持不变"
	}
	return "DEV.to API Key"
}

func indexNowEndpoint(config bridgeConfig) string {
	if configured := strings.TrimSpace(config.IndexNowEndpoint); configured != "" {
		return configured
	}
	return "https://www.bing.com/indexnow"
}

func indexNowKey(config bridgeConfig) string {
	if configured := strings.TrimSpace(config.IndexNowKey); configured != "" {
		return configured
	}
	return blogsearch.DefaultIndexNowKey
}

func indexNowKeyLocation(config bridgeConfig) string {
	if configured := strings.TrimSpace(config.IndexNowKeyLocation); configured != "" {
		return configured
	}
	return config.Search.Origin + "/" + indexNowKey(config) + ".txt"
}

func indexNowHealth(config bridgeConfig) toolHealth {
	key := indexNowKey(config)
	if key == "" {
		return toolHealth{Status: "missing", Summary: "Key 未配置"}
	}
	return toolHealth{
		OK: true, Status: "ok", Summary: "已配置",
		Detail: fmt.Sprintf("%s · %s", indexNowEndpoint(config), indexNowKeyLocation(config)),
	}
}

func indexNowKeyPlaceholder(config bridgeConfig) string {
	if indexNowKey(config) != "" {
		return "已配置；留空保存时保持不变"
	}
	return "IndexNow key"
}

func baiduSite(config bridgeConfig) string {
	return config.Search.Origin
}

func baiduToken(config bridgeConfig) string {
	return strings.TrimSpace(config.BaiduToken)
}

func baiduHealth(config bridgeConfig) toolHealth {
	token := baiduToken(config)
	if token == "" {
		return toolHealth{Status: "missing", Summary: "推送 Token 未配置"}
	}
	resolved, err := blogsearch.ResolveBaiduConfig(config.Search.Origin, baiduSite(config), token)
	if err != nil {
		return toolHealth{Status: "error", Summary: "配置无效", Detail: err.Error()}
	}
	return toolHealth{
		OK: true, Status: "ok", Summary: "已配置",
		Detail: resolved.Site,
	}
}

func baiduTokenPlaceholder(config bridgeConfig) string {
	if baiduToken(config) != "" {
		return "已配置；留空保存时保持不变"
	}
	return "百度资源推送 Token"
}

func googleSearchConsoleServiceJSON(config bridgeConfig) string {
	return strings.TrimSpace(config.GoogleSearchConsoleServiceJSON)
}

func validateGoogleServiceAccountJSON(raw string) error {
	_, err := blogsearch.ParseGoogleServiceAccount(raw)
	return err
}

func googleSearchConsoleAPIHealth(config bridgeConfig) toolHealth {
	raw := googleSearchConsoleServiceJSON(config)
	if raw == "" {
		return toolHealth{Status: "missing", Summary: "Service Account 未配置"}
	}
	if err := validateGoogleServiceAccountJSON(raw); err != nil {
		return toolHealth{Status: "error", Summary: "Service Account 配置无效", Detail: err.Error()}
	}
	return toolHealth{OK: true, Status: "ok", Summary: "已配置"}
}

func googleSearchConsoleAPIPlaceholder(config bridgeConfig) string {
	if googleSearchConsoleServiceJSON(config) != "" {
		return "已配置；留空保存时保持不变"
	}
	return "Google service-account JSON"
}

func bufferAPIPlaceholder(config bridgeConfig) string {
	if config.Social.Buffer.APIKey != "" || os.Getenv("BUFFER_API_KEY") != "" {
		return "已配置；留空保持不变"
	}
	return "Buffer Personal API Key"
}

func toolRegistry(config bridgeConfig) []toolDescriptor {
	configPath, _ := ConfigPath()
	pathField := func(key, label, description string) []toolField {
		return []toolField{{Key: key, Label: label, Type: "file", Description: description}}
	}
	return []toolDescriptor{
		{
			Name: "search-site", DisplayName: "索引站点与 URL", Kind: "runtime", Required: true,
			Description: "所有搜索引擎共用的站点身份和索引资源；索引页只展示这里保存的配置。",
			Health:      toolHealth{OK: true, Status: "ok", Summary: "已配置", Detail: config.Search.Origin},
			Config: toolConfigView{Scope: "bridge", Values: map[string]any{
				"origin": config.Search.Origin, "urlListURL": config.Search.URLListURL,
				"sitemapURL": config.Search.SitemapURL, "fingerprintURL": config.Search.FingerprintURL,
				"googleProperty": config.Search.GoogleProperty,
			}, Schema: []toolField{
				{Key: "origin", Label: "站点域名", Type: "text", Description: "HTTPS Origin，例如 https://thinkerqaq.com；不包含页面路径。"},
				{Key: "urlListURL", Label: "TXT URL 清单", Type: "text", Description: "必须属于站点域名；用于增量和全量提交的 URL 清单。"},
				{Key: "sitemapURL", Label: "XML Sitemap", Type: "text", Description: "必须属于站点域名；用于 Sitemap 提交和索引页链接。"},
				{Key: "fingerprintURL", Label: "内容指纹清单", Type: "text", Description: "可选，同站点 JSON 清单；留空不读取指纹，增量将保守视为内容变化。"},
				{Key: "googleProperty", Label: "Google Property", Type: "text", Description: "必须覆盖站点域名；填写已授权的 sc-domain:域名或 HTTPS URL 前缀属性。"},
			}},
		},
		{
			Name: "bridge", DisplayName: "BlogCTL Bridge", Kind: "runtime", Required: true,
			Description: "Extension 与本机 BlogCTL 的持久控制连接。",
			Health: toolHealth{
				OK: true, Status: "ok", Summary: "运行中", Version: version.Current,
				Detail: fmt.Sprintf("PID %d · %s", os.Getpid(), DefaultAddress),
				Path:   configPath,
			},
			Actions: []toolAction{{
				ID: "restart", Label: "重启 Bridge",
				Description: "重新启动本地服务；存在运行中的同步任务时会拒绝操作。",
			}},
		},
		{
			Name: "content-workspace", DisplayName: "Content Repository", Kind: "runtime", Required: true,
			Description: "Articles / Notes / Series / Projects 的 canonical source。",
			Health:      workspaceHealth(config.ContentRoot, "content"),
			Config:      toolConfigView{Scope: "bridge", Values: map[string]any{"contentRoot": config.ContentRoot}, Schema: []toolField{{Key: "contentRoot", Label: "Content Repository", Type: "directory", Description: "例如 C:\\Users\\zsk\\code\\blog\\blog-content"}}, DefaultExpanded: true},
		},
		{
			Name: "engine-workspace", DisplayName: "Public Engine", Kind: "runtime", Required: true,
			Description: "Astro、BlogCTL scripts 与 publishing adapters 所在仓库。",
			Health:      workspaceHealth(config.EngineRoot, "engine"),
			Config:      toolConfigView{Scope: "bridge", Values: map[string]any{"engineRoot": config.EngineRoot}, Schema: []toolField{{Key: "engineRoot", Label: "Engine Repository", Type: "directory", Description: "例如 C:\\Users\\zsk\\code\\blog\\ThinkerQAQ.github.io"}}, DefaultExpanded: true},
		},
		{
			Name: "distribution-root", DisplayName: "Distribution", Kind: "runtime", Required: true,
			Description: "BlogCTL 生成的发布产物、图片缓存与平台临时输出目录。",
			Health:      distributionRootHealth(config.DistributionRoot),
			Config: toolConfigView{
				Scope:           "bridge",
				Values:          map[string]any{"path": config.DistributionRoot},
				Schema:          []toolField{{Key: "path", Label: "Distribution Directory", Type: "directory", Description: "默认位于 C:\\software\\Coding\\blogctl\\Data\\distribution"}},
				DefaultExpanded: true,
			},
		},
		{
			Name: "publication-bindings", DisplayName: "Publication Bindings", Kind: "runtime", Required: true,
			Description: "文章与各发布平台远端草稿 / 已发布文章的持久关联状态。",
			Health:      publicationBindingsHealth(config.PublicationBindingsPath),
			Config: toolConfigView{
				Scope:           "bridge",
				Values:          map[string]any{"path": config.PublicationBindingsPath},
				Schema:          pathField("path", "Bindings File", "默认位于 BlogCTL 配置目录，例如 C:\\software\\Coding\\blogctl\\Data\\publications.json"),
				DefaultExpanded: true,
			},
		},
		{
			Name: "network-proxy", DisplayName: "Network Proxy", Kind: "runtime", Required: false,
			Description: "启用后只代理 BlogCTL 自己的网络组件：Bridge HTTP Client 与 BlogCTL 启动的外部工具进程；不会修改 Chrome/Edge、系统或其他应用的代理。",
			Health: func() toolHealth {
				if !config.ProxyEnabled {
					return toolHealth{OK: true, Status: "disabled", Summary: "直连"}
				}
				return toolHealth{OK: true, Status: "ok", Summary: proxySummary(config)}
			}(),
			Config: toolConfigView{
				Scope: "bridge", Values: map[string]any{"proxyEnabled": config.ProxyEnabled, "proxyHost": config.ProxyHost, "proxyPort": config.ProxyPort},
				Toggle: &toolToggle{Key: "proxyEnabled", Label: "启用代理", Description: "只影响 BlogCTL Bridge 与子进程网络；浏览器和系统代理不变"},
				Schema: []toolField{
					{Key: "proxyHost", Label: "代理主机", Type: "text", Placeholder: "127.0.0.1"},
					{Key: "proxyPort", Label: "代理端口", Type: "integer", Placeholder: "7890", Min: 1, Max: 65535},
				},
			},
		},
		{
			Name: "logging", DisplayName: "日志", Kind: "runtime", Required: false,
			Description: "BlogCTL Bridge 的结构化运行日志。可配置日志目录和最低日志级别；日志页实时读取当前日志文件。",
			Health:      loggingHealth(config),
			Config: toolConfigView{
				Scope: "bridge",
				Values: map[string]any{
					"directory": func() string {
						if value := strings.TrimSpace(config.LogDirectory); value != "" {
							return value
						}
						value, _ := resolvedLogDirectory(config)
						return value
					}(),
					"level": config.LogLevel,
				},
				Schema: []toolField{
					{
						Key: "directory", Label: "日志目录", Type: "directory",
						Description: "日志文件固定为 bridge.log；目录不存在时自动创建。",
					},
					{
						Key: "level", Label: "日志级别", Type: "select",
						Description: "只记录该级别及以上日志。排查问题时使用 debug。",
						Options:     []string{"debug", "info", "warn", "error"},
					},
				},
			},
		},
		{
			Name: "buffer-api", DisplayName: "Buffer API", Kind: "social", Required: false,
			Description: "LinkedIn / X / Bluesky 社交动态通过 Buffer 官方 GraphQL API 发布；密钥仅保存在本机 blogctl.toml。",
			Health: func() toolHealth {
				if config.Social.Buffer.APIKey == "" {
					if os.Getenv("BUFFER_API_KEY") != "" {
						return toolHealth{OK: true, Status: "ok", Summary: "已通过环境变量配置"}
					}
					return toolHealth{Status: "missing", Summary: "API Key 未配置"}
				}
				return toolHealth{OK: true, Status: "ok", Summary: "已配置"}
			}(),
			Config: toolConfigView{
				Scope: "bridge", Values: map[string]any{},
				Schema: []toolField{{
					Key: "apiKey", Label: "API Key", Type: "secret",
					Placeholder: bufferAPIPlaceholder(config),
					Description: "在 Buffer → API Settings 创建。留空不清除已保存的密钥；请勿提交到 Git。",
				}},
			},
		},
		{
			Name: "devto-api", DisplayName: "DEV.to API", Kind: "publishing", Required: false,
			Description: "DEV.to 文章生命周期使用官方 API；图片优先通过浏览器会话上传到 DEV.to，失败时回退 R2。API Key 仅保存在本机 BlogCTL 配置中。",
			Health:      devtoAPIHealth(config),
			Config: toolConfigView{
				Scope:  "bridge",
				Values: map[string]any{},
				Schema: []toolField{{
					Key: "apiKey", Label: "API Key", Type: "secret",
					Placeholder: devtoAPIPlaceholder(config),
					Description: "在 DEV.to Settings → Extensions 中生成。留空保存不会清除已有 Key。",
				}},
			},
		},
		{
			Name: "indexnow", DisplayName: "IndexNow", Kind: "runtime", Required: false,
			Description: "索引通知通过 IndexNow HTTP API。检测只验证 Endpoint 配置和站点 Key 文件，不会提交测试 URL。",
			Health:      indexNowHealth(config),
			Actions: []toolAction{{
				ID: "check", Label: "检测配置",
				Description: "验证 IndexNow Key Location 可访问且内容与配置 Key 一致。",
			}},
			Config: toolConfigView{
				Scope: "bridge",
				Values: map[string]any{
					"endpoint":    indexNowEndpoint(config),
					"keyLocation": config.IndexNowKeyLocation,
				},
				Schema: []toolField{
					{
						Key: "endpoint", Label: "Endpoint", Type: "text",
						Placeholder: "https://www.bing.com/indexnow",
						Description: "IndexNow endpoint；通常保持默认值。",
					},
					{
						Key: "key", Label: "IndexNow Key", Type: "secret",
						Placeholder: indexNowKeyPlaceholder(config),
						Description: "留空保存时保持当前 Key；对应 Key 文件必须可从站点公开访问。",
					},
					{
						Key: "keyLocation", Label: "Key Location", Type: "text",
						Placeholder: indexNowKeyLocation(config),
						Description: "公开 Key 文件 URL；留空时按站点根目录和 Key 自动推导。",
					},
				},
			},
		},
		{
			Name: "baidu-search-resource", DisplayName: "Baidu Search Resource", Kind: "runtime", Required: false,
			Description: "百度普通资源推送使用官方 URL 提交 API。只提交新增或内容变化的 URL；删除 URL 仅统计，不通过普通推送接口提交。",
			Health:      baiduHealth(config),
			Config: toolConfigView{
				Scope:  "bridge",
				Values: map[string]any{},
				Schema: []toolField{
					{
						Key: "token", Label: "Push Token", Type: "secret",
						Placeholder: baiduTokenPlaceholder(config),
						Description: "百度搜索资源平台普通收录 API Token；留空保存时保持当前 Token。",
					},
				},
			},
		},
		{
			Name: "google-search-console-api", DisplayName: "Google Search Console API", Kind: "runtime", Required: false,
			Description: "Sitemap 与 URL Inspection 使用 Service Account。先在 Google Cloud 启用 Search Console API、创建 Service Account JSON，再把 JSON 中 client_email 加到对应 Search Console Property 的 Users and permissions（Full user）。凭据只保存在本机 Bridge。",
			Health:      googleSearchConsoleAPIHealth(config),
			Actions: []toolAction{{
				ID: "check", Label: "检测配置",
				Description: "交换 OAuth token，并验证 Service Account 能访问配置的 Google Property。",
			}},
			Config: toolConfigView{
				Scope:  "bridge",
				Values: map[string]any{},
				Schema: []toolField{{
					Key: "serviceAccountJson", Label: "Service Account JSON", Type: "secret",
					Placeholder: googleSearchConsoleAPIPlaceholder(config),
					Description: "粘贴完整 JSON；留空保存时保持不变。JSON 中 client_email 还必须加入 Search Console Property 权限。",
				}},
			},
		},
		{
			Name: "node", DisplayName: "Node.js", Kind: "dependency", Required: true,
			Description: "必选。用于 Markdown HTML 渲染、发布图片 renderer，以及 Astro / Pagefind / Mermaid 等 Node 工具链；建议 Node.js 22.21+ 或 24+。",
			Health:      executableHealth(config, "node"),
			Actions: []toolAction{{
				ID: "update", Label: "更新",
				Description: "升级到当前 Node.js LTS；Windows 使用 WinGet，并沿用 BlogCTL Network Proxy。",
			}},
			Config: toolConfigView{Scope: "bridge", Values: map[string]any{"path": config.ToolPaths["node"]}, Schema: pathField("path", "Executable", "留空时从 PATH 自动检测 node")},
		},
		{
			Name: "npm", DisplayName: "npm", Kind: "dependency", Required: true,
			Description: "必选。负责安装并运行 Public Engine 的 Node 依赖，包括 Astro、Pagefind 与 Mermaid CLI。", Health: executableHealth(config, "npm"),
			Actions: []toolAction{{
				ID: "update", Label: "更新",
				Description: "执行 npm install -g npm@latest，并沿用 BlogCTL Network Proxy。",
			}},
			Config: toolConfigView{Scope: "bridge", Values: map[string]any{"path": config.ToolPaths["npm"]}, Schema: pathField("path", "Executable", "留空时从 PATH 自动检测 npm")},
		},
		{
			Name: "git", DisplayName: "Git", Kind: "dependency", Required: true,
			Description: "必选。用于仓库版本工作流，以及 AI Search 增量同步时计算提交差异。", Health: executableHealth(config, "git"),
			Actions: []toolAction{{
				ID: "update", Label: "更新",
				Description: "升级 Git for Windows；Windows 使用 WinGet，并沿用 BlogCTL Network Proxy。",
			}},
			Config: toolConfigView{Scope: "bridge", Values: map[string]any{"path": config.ToolPaths["git"]}, Schema: pathField("path", "Executable", "留空时从 PATH 自动检测 git")},
		},
		{
			Name: "java", DisplayName: "Java", Kind: "dependency", Required: false,
			Description: "可选。只有网站构建或发布内容包含 PlantUML（puml / plantuml / UML）时才需要，用于运行 PlantUML JAR 渲染图表。",
			Health:      executableHealth(config, "java"),
			Actions: []toolAction{{
				ID: "update", Label: "更新",
				Description: "按当前 JDK vendor/major 识别 WinGet 包后升级；无法安全识别时不会切换发行版。",
			}},
			Config: toolConfigView{Scope: "bridge", Values: map[string]any{"path": config.ToolPaths["java"]}, Schema: pathField("path", "Executable", "留空时从 PATH 自动检测 java")},
		},
	}
}

func stringConfig(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func boolConfig(values map[string]any, key string) bool {
	value, _ := values[key].(bool)
	return value
}

func intConfig(values map[string]any, key string) int {
	switch value := values[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func updateToolConfig(config bridgeConfig, name string, values map[string]any) (bridgeConfig, error) {
	switch name {
	case "search-site":
		for key := range values {
			if key != "origin" && key != "urlListURL" && key != "sitemapURL" && key != "fingerprintURL" && key != "googleProperty" {
				return config, fmt.Errorf("unknown search-site configuration field: %s", key)
			}
		}
		site := blogsearch.SiteConfig{
			Origin: stringConfig(values, "origin"), URLListURL: stringConfig(values, "urlListURL"),
			SitemapURL: stringConfig(values, "sitemapURL"), FingerprintURL: stringConfig(values, "fingerprintURL"),
			GoogleProperty: stringConfig(values, "googleProperty"),
		}
		config.Search = &site
	case "content-workspace":
		config.ContentRoot = stringConfig(values, "contentRoot")
	case "engine-workspace":
		config.EngineRoot = stringConfig(values, "engineRoot")
	case "distribution-root":
		config.DistributionRoot = stringConfig(values, "path")
	case "publication-bindings":
		config.PublicationBindingsPath = stringConfig(values, "path")
	case "network-proxy":
		config.ProxyEnabled = boolConfig(values, "proxyEnabled")
		config.ProxyHost = stringConfig(values, "proxyHost")
		config.ProxyPort = intConfig(values, "proxyPort")
	case "logging":
		config.LogDirectory = stringConfig(values, "directory")
		config.LogLevel = stringConfig(values, "level")
	case "buffer-api":
		if key := stringConfig(values, "apiKey"); key != "" {
			config.Social.Buffer.APIKey = key
		}
	case "devto-api":
		if key := stringConfig(values, "apiKey"); key != "" {
			config.DevtoAPIKey = key
		}
	case "indexnow":
		if endpoint := stringConfig(values, "endpoint"); endpoint != "" {
			config.IndexNowEndpoint = endpoint
		}
		if key := stringConfig(values, "key"); key != "" {
			config.IndexNowKey = key
		}
		if keyLocation := stringConfig(values, "keyLocation"); keyLocation != "" {
			config.IndexNowKeyLocation = keyLocation
		}
	case "baidu-search-resource":
		nextSite := config.Search.Origin
		nextToken := config.BaiduToken
		if token := stringConfig(values, "token"); token != "" {
			nextToken = token
		}
		if strings.TrimSpace(nextToken) != "" {
			if _, err := blogsearch.ResolveBaiduConfig(config.Search.Origin, nextSite, nextToken); err != nil {
				return config, err
			}
		} else if strings.TrimSpace(nextSite) != "" {
			if _, err := blogsearch.ResolveBaiduConfig(config.Search.Origin, nextSite, "validation-token"); err != nil {
				return config, err
			}
		}
		config.BaiduToken = nextToken
	case "google-search-console-api":
		if value := stringConfig(values, "serviceAccountJson"); value != "" {
			if err := validateGoogleServiceAccountJSON(value); err != nil {
				return config, err
			}
			config.GoogleSearchConsoleServiceJSON = value
		}
	case "node", "npm", "git", "java":
		if config.ToolPaths == nil {
			config.ToolPaths = map[string]string{}
		}
		config.ToolPaths[name] = stringConfig(values, "path")
	default:
		return config, fmt.Errorf("tool configuration is not supported: %s", name)
	}
	return normalizeBridgeConfig(config)
}

func publishingViews(config bridgeConfig) []publishingPlatformView {
	views := make([]publishingPlatformView, 0, len(publishingPlatformOrder))
	for _, id := range publishingPlatformOrder {
		value := config.Publishing.Platforms[id]
		views = append(views, publishingPlatformView{
			ID: id, Label: blogplatform.Label(id), Language: value.Language, ChangedOnly: value.ChangedOnly,
			Capabilities: publisher.PlatformCapabilitiesFor(id), Footer: value.Footer,
			Canonical: value.Canonical, Tracking: value.Tracking,
		})
	}
	return views
}

func updatePublishing(config bridgeConfig, views []publishingPlatformView) (bridgeConfig, error) {
	if config.Publishing.Platforms == nil {
		config.Publishing = defaultPublishingConfig()
	}
	for _, view := range views {
		if !blogplatform.Supported(view.ID) {
			return config, fmt.Errorf("unsupported publishing platform: %s", view.ID)
		}
		if view.ChangedOnly && !publisher.PlatformCapabilitiesFor(view.ID).DraftUpdate {
			return config, fmt.Errorf("%s cannot safely update an existing draft", view.ID)
		}
		config.Publishing.Platforms[view.ID] = publishingPlatformConfig{
			Language: view.Language, ChangedOnly: view.ChangedOnly, Footer: view.Footer, Canonical: view.Canonical, Tracking: view.Tracking,
		}
	}
	return normalizeBridgeConfig(config)
}

func normalizeSyncRequest(request syncRequest) (syncRequest, error) {
	if request.RequestID != "" {
		if len(request.RequestID) > 128 || !mcpRequestID.MatchString(request.RequestID) {
			return request, errors.New("invalid requestId")
		}
	}

	// Explicit create/update/publish-draft use the same article/platform validation but a
	// distinct task contract; they NEVER read PublicationBinding for targets.
	operation := strings.ToLower(strings.TrimSpace(request.Operation))
	if operation == "create" || operation == "update" || operation == "publish-draft" {
		if request.ResumeMediumDraftID != "" {
			return request, errors.New("Medium draft resume ID can only be set by the failed-job retry handler")
		}
		base := blogapp.SyncRequest{Articles: []string{request.Article}, Platforms: request.Platforms,
			DryRun: request.DryRun, Operation: "draft"}
		normalized, err := blogapp.NormalizeSyncRequest(base)
		if err != nil {
			return request, err
		}
		request.Article = normalized.Articles[0]
		request.Platforms = normalized.Platforms
		request.Operation = operation
		allowed := make(map[string]struct{}, len(request.Platforms))
		for _, platform := range request.Platforms {
			allowed[platform] = struct{}{}
			if (request.PublishAfter || operation == "publish-draft") && !publisher.PlatformCapabilitiesFor(platform).ExplicitPublish {
				return request, fmt.Errorf("%s does not support publishing the selected draft", platform)
			}
		}
		if operation == "create" {
			if len(request.Targets) != 0 {
				return request, errors.New("creation must not specify remote targets")
			}
			return request, nil
		}
		if len(request.Targets) == 0 {
			return request, errors.New("update or publish-draft requires selected remote article IDs")
		}
		if operation == "publish-draft" && request.PublishAfter {
			return request, errors.New("publish-draft cannot set publishAfter")
		}
		seen := map[string]struct{}{}
		for i, target := range request.Targets {
			if _, ok := allowed[target.Platform]; !ok {
				return request, fmt.Errorf("target %d is not in selected platforms", i)
			}
			if len(target.ID) == 0 || len(target.ID) > 100 || !validExplicitRemoteID(target.ID) {
				return request, fmt.Errorf("target %d has an invalid remote article ID", i)
			}
			if target.State != "draft" && target.State != "published" {
				return request, fmt.Errorf("target %d has an invalid publication state", i)
			}
			if operation == "publish-draft" && target.State != "draft" {
				return request, fmt.Errorf("target %d must be a draft to publish", i)
			}
			if target.State == "published" && request.PublishAfter {
				return request, errors.New("published targets cannot be published again")
			}
			key := target.Platform + "/" + target.ID
			if _, ok := seen[key]; ok {
				return request, fmt.Errorf("duplicate remote target %s", key)
			}
			seen[key] = struct{}{}
			if target.URL != "" {
				parsed, err := url.Parse(target.URL)
				if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Hostname() == "" {
					return request, fmt.Errorf("target %d has an invalid remote URL", i)
				}
				host := parsed.Hostname()
				suffixes := publisherCookieHostSuffixes(target.Platform)
				if target.Platform == "cnblogs" {
					suffixes = []string{"cnblogs.com"}
				}
				if target.Platform == "devto" {
					suffixes = []string{"dev.to"}
				}
				valid := false
				for _, suffix := range suffixes {
					if host == suffix || strings.HasSuffix(host, "."+suffix) {
						valid = true
						break
					}
				}
				if !valid {
					return request, fmt.Errorf("target %d URL does not belong to %s", i, target.Platform)
				}
			}
		}
		return request, nil
	}
	normalized, err := blogapp.NormalizeSyncRequest(blogapp.SyncRequest{
		Articles:  []string{request.Article},
		Platforms: request.Platforms, DryRun: request.DryRun,
		Changed: request.Changed, Draft: request.Draft, Operation: request.Operation,
	})
	if err != nil {
		return request, err
	}
	request.Article = normalized.Articles[0]
	request.Platforms = normalized.Platforms
	request.Operation = normalized.Operation
	return request, nil
}

func numberValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func newJobID() string {
	value := make([]byte, 8)
	if _, err := rand.Read(value); err == nil {
		return hex.EncodeToString(value)
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

type syncRunner func(context.Context, bridgeConfig, syncRequest, func(blogapp.SyncEvent)) (string, error)

func allExplicitPublishPlatforms(platforms []string) bool {
	if len(platforms) == 0 {
		return false
	}
	for _, platform := range platforms {
		if !publisher.PlatformCapabilitiesFor(platform).ExplicitPublish {
			return false
		}
	}
	return true
}

type bridgeNativePublisher struct {
	server *Server
}

func (s *Server) publicationBindingsPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.config.PublicationBindingsPath
}

func cloneStringMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func draftInputFromCompiled(article blogcompiler.CompiledArticle, contentRoot, distributionRoot string, config bridgeConfig) publisher.DraftInput {
	assets := make([]publisher.PublishingAsset, 0, len(article.Assets))
	for _, asset := range article.Assets {
		assets = append(assets, publisher.PublishingAsset{
			Kind: asset.Kind, ID: asset.ID, ObjectKey: asset.ObjectKey,
			PublicURL: asset.PublicURL, Source: asset.Source,
		})
	}
	r2 := config.Publishing.Assets.R2
	return publisher.DraftInput{
		Slug: article.Slug, Title: article.Title, Description: article.Description, Series: article.Series,
		Markdown: article.Markdown, HTML: article.HTML, Language: article.Language,
		ContentHash: article.ContentHash, SourceDir: article.SourceDir, ContentRoot: contentRoot,
		DistributionRoot: strings.TrimSpace(distributionRoot),
		Tags:             append([]string{}, article.Tags...), CoverImageURL: article.CoverImageURL,
		NativeCanonicalURL: article.NativeCanonicalURL, Published: article.Published,
		Assets: assets,
		R2Fallback: publisher.R2FallbackConfig{
			AccessKeyID:     r2.AccessKeyID,
			SecretAccessKey: r2.SecretAccessKey,
			AccountID:       r2.AccountID,
			Endpoint:        r2.Endpoint,
			Bucket:          r2.Bucket,
			PublicBaseURL:   r2.PublicBaseURL,
		},
	}
}

var platformCookieHostSuffixes = map[string][]string{
	"juejin":       {"juejin.cn"},
	"csdn":         {"csdn.net"},
	"segmentfault": {"segmentfault.com"},
	"zhihu":        {"zhihu.com"},
	"51cto":        {"51cto.com"},
	"oschina":      {"oschina.net"},
	"toutiao":      {"toutiao.com"},
	"devto":        {"dev.to"},
	"medium":       {"medium.com"},
	"hashnode":     {"hashnode.com"},
}

func publisherCookieHostSuffixes(platform string) []string {
	return append([]string{}, platformCookieHostSuffixes[platform]...)
}

func (p bridgeNativePublisher) publisherSession(platform string) (publisher.Session, *http.Client, error) {
	p.server.mu.Lock()
	if platform == "devto" {
		apiKey := devtoAPIKey(p.server.config)
		httpClient := p.server.httpClient
		session, ok := p.server.sessions[platform]
		if ok && !session.ExpiresAt.After(p.server.now()) {
			delete(p.server.sessions, platform)
			ok = false
		}
		p.server.mu.Unlock()
		if apiKey == "" {
			return publisher.Session{}, nil, errors.New("DEV.to API key is required")
		}
		result := publisher.Session{APIKey: apiKey}
		if ok {
			result.UserAgent = session.UserAgent
			result.RequestCookieHeader = session.RequestCookieHeader
			result.CookieHostSuffixes = publisherCookieHostSuffixes(platform)
			result.Cookies = make([]publisher.BrowserCookie, 0, len(session.BrowserCookies))
			for _, cookie := range session.BrowserCookies {
				result.Cookies = append(result.Cookies, publisher.BrowserCookie{
					Name: cookie.Name, Value: cookie.Value, Domain: cookie.Domain, Path: cookie.Path,
					Secure: cookie.Secure, HTTPOnly: cookie.HTTPOnly, HostOnly: cookie.HostOnly,
					SameSite: cookie.SameSite, ExpirationDate: cookie.ExpirationDate,
				})
			}
		}
		return result, httpClient, nil
	}
	session, ok := p.server.sessions[platform]
	if ok && !session.ExpiresAt.After(p.server.now()) {
		delete(p.server.sessions, platform)
		ok = false
	}
	httpClient := p.server.httpClient
	p.server.mu.Unlock()
	if !ok {
		return publisher.Session{}, nil, fmt.Errorf("%s browser session is required", platform)
	}

	cookies := make([]publisher.BrowserCookie, 0, len(session.BrowserCookies))
	for _, cookie := range session.BrowserCookies {
		cookies = append(cookies, publisher.BrowserCookie{
			Name: cookie.Name, Value: cookie.Value, Domain: cookie.Domain, Path: cookie.Path,
			Secure: cookie.Secure, HTTPOnly: cookie.HTTPOnly, HostOnly: cookie.HostOnly,
			SameSite: cookie.SameSite, ExpirationDate: cookie.ExpirationDate,
		})
	}
	publisherSession := publisher.Session{
		Cookies: cookies, UserAgent: session.UserAgent, RequestCookieHeader: session.RequestCookieHeader,
		RequestCookieHeaders: cloneStringMap(session.RequestCookieHeaders),
		RequestHeaders:       cloneStringMap(session.RequestHeaders),
		CookieHostSuffixes:   publisherCookieHostSuffixes(platform),
	}
	// Reuse the standard Bridge HTTP client; do not spoof browser TLS fingerprints.
	if platform == "toutiao" {
		if base, production := httpClient.Transport.(*http.Transport); production {
			copyClient := *httpClient
			copyClient.Transport = toutiaoBrowserTransport{base: base, hub: p.server.toutiaoRequests}
			return publisherSession, &copyClient, nil
		}
	}
	return publisherSession, httpClient, nil
}

func mediumFallbackPath(distributionRoot, slug string) (string, error) {
	relative := filepath.Clean(filepath.FromSlash(strings.TrimSpace(slug)))
	if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", errors.New("invalid Medium article slug")
	}
	return filepath.Join(distributionRoot, "medium", relative+".html"), nil
}

func writeMediumFallback(distributionRoot string, article blogcompiler.CompiledArticle) (string, error) {
	if strings.TrimSpace(article.FallbackHTML) == "" {
		return "", nil
	}
	path, err := mediumFallbackPath(distributionRoot, article.Slug)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(article.FallbackHTML), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func mediumPlatformSession(session publisher.Session) platformSession {
	cookies := map[string]string{}
	for _, cookie := range session.Cookies {
		if strings.TrimSpace(cookie.Name) != "" && cookie.Value != "" {
			cookies[cookie.Name] = cookie.Value
		}
	}
	return platformSession{
		Cookies: cookies, UserAgent: session.UserAgent,
		RequestCookieHeader: session.RequestCookieHeader,
	}
}

func (p bridgeNativePublisher) createOrUpdateMediumDraft(ctx context.Context, request blogapp.NativeDraftRequest, session publisher.Session, httpClient *http.Client) (blogapp.NativeDraftResult, error) {
	state, _, err := publisher.LoadPublicationState(p.server.publicationBindingsPath(), request.Article, "medium")
	if err != nil {
		return blogapp.NativeDraftResult{}, err
	}
	targetID := strings.TrimSpace(state.RemoteDraftID)
	targetURL := strings.TrimSpace(state.DraftURL)
	if targetID == "" && (strings.TrimSpace(state.PublishedRemoteID) != "" || strings.TrimSpace(state.PublishedURL) != "") {
		return blogapp.NativeDraftResult{}, errors.New("Medium story is already published; safe published-article updates are not supported yet")
	}
	if targetID != "" && state.DraftHash == request.Compiled.ContentHash {
		return blogapp.NativeDraftResult{
			Result: "skipped", URL: targetURL,
			Message: "Medium draft is unchanged; keeping the existing draft.",
		}, nil
	}

	fallbackPath, err := writeMediumFallback(strings.TrimSpace(request.DistributionRoot), request.Compiled)
	if err != nil {
		return blogapp.NativeDraftResult{}, err
	}
	if len(request.Compiled.Payload) == 0 {
		return blogapp.NativeDraftResult{}, errors.New("compiled Medium article is missing transport payload")
	}

	var draft mediumDraft
	if err := json.Unmarshal(request.Compiled.Payload, &draft); err != nil {
		return blogapp.NativeDraftResult{}, fmt.Errorf("invalid compiled Medium payload: %w", err)
	}
	input := draftInputFromCompiled(request.Compiled, request.ContentRoot, request.DistributionRoot, p.server.config)
	client := mediumClient{httpClient: httpClient}
	var result map[string]any
	if targetID == "" {
		result, err = client.createDraft(ctx, mediumPlatformSession(session), draft, input)
	} else {
		result, err = client.updateDraft(ctx, mediumPlatformSession(session), targetID, draft, input)
	}
	if err != nil {
		return blogapp.NativeDraftResult{}, err
	}

	postID, _ := result["postId"].(string)
	draftURL, _ := result["draftUrl"].(string)
	if strings.TrimSpace(postID) == "" || strings.TrimSpace(draftURL) == "" {
		return blogapp.NativeDraftResult{}, errors.New("Medium draft response is missing post id or draft URL")
	}
	created, _ := result["created"].(bool)
	updated, _ := result["updated"].(bool)
	if !created && !updated {
		created = targetID == ""
		updated = targetID != ""
	}

	pending := []string{}
	if value, _ := result["canonicalPending"].(bool); value {
		pending = append(pending, "canonical")
	}
	if value, _ := result["tagsPending"].(bool); value {
		pending = append(pending, "tags")
	}
	if value, _ := result["coverImagePending"].(bool); value {
		pending = append(pending, "coverImage")
	}

	if err := publisher.SavePublicationDraftResult(
		p.server.publicationBindingsPath(),
		request.Article,
		"medium",
		request.Compiled.ContentHash,
		publisher.DraftResult{ID: postID, URL: draftURL, Created: created, Updated: updated},
		p.server.now(),
	); err != nil {
		return blogapp.NativeDraftResult{}, err
	}
	if err := publisher.SavePublicationPendingFields(p.server.publicationBindingsPath(), request.Article, "medium", pending); err != nil {
		return blogapp.NativeDraftResult{}, err
	}

	pendingLabels := make([]string, 0, len(pending))
	for _, field := range pending {
		switch field {
		case "coverImage":
			pendingLabels = append(pendingLabels, "cover image")
		default:
			pendingLabels = append(pendingLabels, field)
		}
	}
	action := "created"
	resultName := "draft-created"
	if updated {
		action = "updated"
		resultName = "updated"
	}
	message := ""
	if len(pendingLabels) > 0 {
		message = "Medium draft " + action + "; pending editor fields: " + strings.Join(pendingLabels, ", ")
	}
	if fallbackCount := int(numberValue(result["imageFallbacks"])); fallbackCount > 0 {
		if message != "" {
			message += ". "
		}
		message += fmt.Sprintf("%d image(s) used R2 fallback links after Medium upload failed", fallbackCount)
	}
	if fallbackPath != "" {
		if message != "" {
			message += ". "
		}
		message += "Fallback saved to " + fallbackPath
	}
	return blogapp.NativeDraftResult{Result: resultName, URL: draftURL, Message: message}, nil
}

func (p bridgeNativePublisher) CreateOrUpdateDraft(ctx context.Context, request blogapp.NativeDraftRequest) (blogapp.NativeDraftResult, error) {
	session, httpClient, err := p.publisherSession(request.Platform)
	if err != nil {
		return blogapp.NativeDraftResult{}, err
	}
	if request.Platform == "medium" {
		return p.createOrUpdateMediumDraft(ctx, request, session, httpClient)
	}
	service := publisher.Service{HTTPClient: httpClient, PublicationPath: p.server.publicationBindingsPath()}
	result, err := service.CreateOrUpdateDraftInput(
		ctx, request.Platform, session, request.ContentRoot, draftInputFromCompiled(request.Compiled, request.ContentRoot, request.DistributionRoot, p.server.config), request.ChangedOnly,
	)
	if err != nil {
		return blogapp.NativeDraftResult{}, err
	}
	resultName := "draft-created"
	if request.Platform == "devto" {
		resultName = "created"
	}
	if result.Updated {
		resultName = "updated"
	}
	if result.Skipped {
		resultName = "skipped"
	}
	return blogapp.NativeDraftResult{Result: resultName, URL: result.URL}, nil
}

func (p bridgeNativePublisher) PublishDraft(ctx context.Context, request blogapp.NativePublishRequest) (blogapp.NativePublishResult, error) {
	session, httpClient, err := p.publisherSession(request.Platform)
	if err != nil {
		return blogapp.NativePublishResult{}, err
	}
	if request.Platform == "medium" {
		state, _, err := publisher.LoadPublicationState(p.server.publicationBindingsPath(), request.Article, "medium")
		if err != nil {
			return blogapp.NativePublishResult{}, err
		}
		postID := strings.TrimSpace(state.RemoteDraftID)
		if postID == "" {
			postID = strings.TrimSpace(state.PublishedRemoteID)
		}
		if postID == "" {
			return blogapp.NativePublishResult{}, errors.New("Medium remote draft id is missing; save the article first")
		}
		if state.DraftHash == "" || state.DraftHash != request.Compiled.ContentHash {
			return blogapp.NativePublishResult{}, errors.New("Medium source changed after the remote draft was prepared; save the draft again before publishing")
		}
		result, err := (mediumClient{httpClient: httpClient}).publishDraft(
			ctx, mediumPlatformSession(session), postID, request.Compiled.Title, request.Compiled.Description,
		)
		if err != nil {
			return blogapp.NativePublishResult{}, err
		}
		publishedID, _ := result["postId"].(string)
		publishedURL, _ := result["url"].(string)
		if strings.TrimSpace(publishedID) == "" || strings.TrimSpace(publishedURL) == "" {
			return blogapp.NativePublishResult{}, errors.New("Medium publish response is missing article id or URL")
		}
		if err := publisher.SavePublicationPublishResult(
			p.server.publicationBindingsPath(),
			request.Article,
			"medium",
			request.Compiled.ContentHash,
			publisher.PublishResult{ID: publishedID, URL: publishedURL},
			p.server.now(),
		); err != nil {
			return blogapp.NativePublishResult{}, err
		}
		return blogapp.NativePublishResult{Result: "published", URL: publishedURL}, nil
	}
	service := publisher.Service{HTTPClient: httpClient, PublicationPath: p.server.publicationBindingsPath()}
	result, err := service.PublishDraftInput(ctx, request.Platform, session, request.ContentRoot, draftInputFromCompiled(request.Compiled, request.ContentRoot, request.DistributionRoot, p.server.config))
	if err != nil {
		return blogapp.NativePublishResult{}, err
	}
	return blogapp.NativePublishResult{Result: "published", URL: result.URL}, nil
}

func (s *Server) runSyncApplication(ctx context.Context, config bridgeConfig, request syncRequest, onEvent func(blogapp.SyncEvent)) (string, error) {
	// Publication state is shared by all platform tasks.
	// Serialize live jobs so independent tasks cannot lose each other's state updates.
	s.distributionMu.Lock()
	defer s.distributionMu.Unlock()

	applicationConfig := blogapp.SyncConfig{
		EngineRoot:       config.EngineRoot,
		ContentRoot:      config.ContentRoot,
		DistributionRoot: config.DistributionRoot,
		Publishing:       config.Publishing,
		ToolPaths:        config.ToolPaths,
	}
	if request.Operation == "create" || request.Operation == "update" || request.Operation == "publish-draft" {
		return s.runExplicitSync(ctx, config, request, onEvent)
	}
	if request.Operation == "update-published" {
		if request.DryRun || len(request.Platforms) != 1 {
			return "", errors.New("published update requires one real platform operation")
		}
		platform := request.Platforms[0]
		if !blogplatform.For(platform).PublishedUpdate {
			return "", fmt.Errorf("published update is not supported for %s", platform)
		}
		started := time.Now()
		compiledArticles, compileErr := blogcompiler.CompilePlatform(ctx, blogcompiler.CompileOptions{
			EngineRoot: config.EngineRoot, ContentRoot: config.ContentRoot,
			Publishing: config.Publishing, Node: config.ToolPaths["node"],
			Platform: platform, Articles: []string{request.Article}, DryRun: true,
		})
		if compileErr != nil {
			onEvent(blogapp.SyncEvent{Platform: platform, State: "failed", Message: compileErr.Error()})
			return "", compileErr
		}
		if len(compiledArticles) != 1 {
			compileErr = errors.New("publishing compiler returned no article for " + platform)
			onEvent(blogapp.SyncEvent{Platform: platform, State: "failed", Message: compileErr.Error()})
			return "", compileErr
		}
		session, client, err := (bridgeNativePublisher{server: s}).publisherSession(platform)
		if err != nil {
			onEvent(blogapp.SyncEvent{Platform: platform, State: "failed", Message: err.Error()})
			return "", err
		}
		service := publisher.Service{HTTPClient: client, PublicationPath: config.PublicationBindingsPath}
		input := draftInputFromCompiled(compiledArticles[0], config.ContentRoot, config.DistributionRoot, config)
		var result publisher.PublishResult
		var skipped bool
		switch platform {
		case "cnblogs":
			result, skipped, err = service.UpdateCNBlogsPublishedInput(ctx, session, config.ContentRoot, input)
		case "toutiao":
			result, skipped, err = service.UpdateToutiaoPublishedInput(ctx, session, config.ContentRoot, input)
		default:
			err = fmt.Errorf("published update is not implemented for %s", platform)
		}
		if err != nil {
			slog.Warn("published update failed", "operation", "update-published", "platform", platform, "slug", request.Article, "errorType", fmt.Sprintf("%T", err))
			onEvent(blogapp.SyncEvent{Platform: platform, State: "failed", Message: err.Error()})
			return "", err
		}
		resultName := "published-updated"
		if platform == "toutiao" {
			resultName = "published-update-submitted"
		}
		if skipped {
			resultName = "skipped"
		}
		onEvent(blogapp.SyncEvent{Platform: platform, State: "completed", Result: resultName, URL: result.URL})
		slog.Info("published update completed", "operation", "update-published", "platform", platform, "slug", request.Article, "result", resultName, "durationMs", time.Since(started).Milliseconds())
		return "", nil
	}
	service := blogapp.NewSyncService()
	service.NativePublisher = bridgeNativePublisher{server: s}
	service.OnEvent = onEvent
	changedByPlatform := changedPoliciesForRequest(config, request)
	for platform, enabled := range changedByPlatform {
		slog.Info("draft changed-only policy selected", "operation", "draft-policy", "platform", platform, "enabled", enabled)
	}
	return service.Run(ctx, applicationConfig, blogapp.SyncRequest{
		Articles:          []string{request.Article},
		Platforms:         append([]string{}, request.Platforms...),
		DryRun:            request.DryRun,
		Changed:           request.Changed,
		ChangedByPlatform: changedByPlatform,
		Draft:             request.Draft,
		Operation:         request.Operation,
	})
}

func changedPoliciesForRequest(config bridgeConfig, request syncRequest) map[string]bool {
	if !request.UsePlatformChangedOnly || request.Operation != "draft" {
		return nil
	}
	policies := make(map[string]bool, len(request.Platforms))
	for _, platform := range request.Platforms {
		policies[platform] = config.Publishing.Platforms[platform].ChangedOnly
	}
	return policies
}

func applySyncEventToJob(job *syncJob, event blogapp.SyncEvent, at time.Time) {
	if job == nil || event.Platform == "" {
		return
	}
	if job.Results == nil {
		job.Results = map[string]syncPlatformResult{}
	}
	result := job.Results[event.Platform]
	result.State = event.State
	if event.Result != "" {
		result.Result = event.Result
	}
	// Final platform events may carry only a summary. Keep the last durable
	// successful remote URL; per-target links are retained in job.Events.
	if event.URL != "" {
		result.URL = event.URL
	}
	result.Message = event.Message
	if event.State == "failed" {
		result.Error = event.Message
	} else {
		result.Error = ""
	}
	job.Results[event.Platform] = result
	job.Events = append(job.Events, syncJobEvent{
		At: at.UTC().Format(time.RFC3339), Platform: event.Platform, TargetID: event.TargetID, State: event.State,
		Result: event.Result, URL: event.URL, Message: event.Message,
	})
}

func (s *Server) recordSyncEvent(jobID string, event blogapp.SyncEvent) {
	s.mu.Lock()
	applySyncEventToJob(s.jobs[jobID], event, s.now())
	s.mirrorSyncJobLocked(s.jobs[jobID])
	s.mu.Unlock()

	attributes := []any{"jobId", jobID, "platform", event.Platform, "state", event.State}
	if strings.TrimSpace(event.Message) != "" {
		attributes = append(attributes, "message", event.Message)
	}
	switch event.State {
	case "failed":
		slog.Error("publishing task event", attributes...)
	case "completed":
		slog.Info("publishing task event", attributes...)
	default:
		slog.Debug("publishing task event", attributes...)
	}
}

func moveJobToFront(order []string, id string) []string {
	result := make([]string, 0, len(order)+1)
	result = append(result, id)
	for _, candidate := range order {
		if candidate != id {
			result = append(result, candidate)
		}
	}
	return result
}

func newSyncJob(id string, request syncRequest, startedAt time.Time) *syncJob {
	results := make(map[string]syncPlatformResult, len(request.Platforms))
	events := make([]syncJobEvent, 0, len(request.Platforms))
	for _, platform := range request.Platforms {
		results[platform] = syncPlatformResult{State: "queued"}
		events = append(events, syncJobEvent{
			At: startedAt.Format(time.RFC3339), Platform: platform, State: "queued",
		})
	}
	return &syncJob{
		ID: id, Article: request.Article, Platforms: append([]string{}, request.Platforms...),
		Operation: request.Operation, Request: request, Results: results, Events: events, State: "running",
		StartedAt: startedAt.Format(time.RFC3339), DryRun: request.DryRun,
	}
}

func (s *Server) pruneSyncJobHistoryLocked(limit int) {
	if limit < 1 || len(s.jobOrder) <= limit {
		return
	}
	kept := make([]string, 0, len(s.jobOrder))
	for _, id := range s.jobOrder {
		job := s.jobs[id]
		if len(kept) < limit || (job != nil && job.State == "running") {
			kept = append(kept, id)
			continue
		}
		delete(s.jobs, id)
	}
	s.jobOrder = kept
}

func (s *Server) launchSyncJob(jobID string, request syncRequest, config bridgeConfig) {
	runner := s.syncRunner
	if runner == nil {
		runner = s.runSyncApplication
	}
	go func() {
		output, err := runner(context.Background(), config, request, func(event blogapp.SyncEvent) {
			s.recordSyncEvent(jobID, event)
		})
		s.mu.Lock()
		defer s.mu.Unlock()
		stored := s.jobs[jobID]
		if stored == nil {
			return
		}
		stored.Output = output
		stored.FinishedAt = s.now().UTC().Format(time.RFC3339)
		if err != nil {
			stored.State = "failed"
			stored.Error = err.Error()
			for _, platform := range stored.Platforms {
				result := stored.Results[platform]
				if result.State == "completed" || result.State == "failed" {
					continue
				}
				applySyncEventToJob(stored, blogapp.SyncEvent{
					Platform: platform, State: "failed", Message: err.Error(),
				}, s.now())
			}
			s.mirrorSyncJobLocked(stored)
			s.pruneSyncJobHistoryLocked(20)
			slog.Error("publishing task failed", "jobId", jobID, "article", request.Article, "operation", request.Operation, "error", err.Error())
			return
		}
		stored.State = "completed"
		s.mirrorSyncJobLocked(stored)
		s.pruneSyncJobHistoryLocked(20)
		slog.Info("publishing task completed", "jobId", jobID, "article", request.Article, "operation", request.Operation)
	}()
}

func (s *Server) startSyncJob(request syncRequest) (*syncJob, error) {
	startedAt := s.now().UTC()
	job := newSyncJob(newJobID(), request, startedAt)
	s.mu.Lock()
	if request.RequestID != "" {
		for _, existing := range s.jobs {
			if existing == nil || existing.Request.RequestID != request.RequestID {
				continue
			}
			if !reflect.DeepEqual(existing.Request, request) {
				s.mu.Unlock()
				return nil, errors.New("requestId was already used for a different publishing operation")
			}
			found := cloneSyncJob(existing)
			s.mu.Unlock()
			return found, nil
		}
	}
	if s.jobs == nil {
		s.jobs = map[string]*syncJob{}
	}
	s.jobs[job.ID] = job
	s.jobOrder = append([]string{job.ID}, s.jobOrder...)
	s.mirrorSyncJobLocked(job)
	s.pruneSyncJobHistoryLocked(20)
	config := s.config
	response := cloneSyncJob(job)
	s.mu.Unlock()

	slog.Info("publishing task started", "jobId", job.ID, "article", request.Article, "operation", request.Operation, "platformCount", len(request.Platforms))
	s.launchSyncJob(job.ID, request, config)
	return response, nil
}

func cloneSyncJob(job *syncJob) *syncJob {
	if job == nil {
		return nil
	}
	clone := *job
	clone.Platforms = append([]string{}, job.Platforms...)
	clone.Events = append([]syncJobEvent{}, job.Events...)
	clone.Results = make(map[string]syncPlatformResult, len(job.Results))
	for platform, result := range job.Results {
		clone.Results[platform] = result
	}
	return &clone
}

func (s *Server) syncJobs() []syncJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]syncJob, 0, len(s.jobOrder))
	for _, id := range s.jobOrder {
		if job := cloneSyncJob(s.jobs[id]); job != nil {
			result = append(result, *job)
		}
	}
	return result
}

func (s *Server) runningSyncJobs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	running := 0
	for _, job := range s.jobs {
		if job != nil && job.State == "running" {
			running++
		}
	}
	return running
}

func (s *Server) deleteSyncJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.restoreSyncJobLocked(id)
	if job == nil {
		return errors.New("sync job not found")
	}
	if job.State == "running" {
		return errors.New("running sync job cannot be deleted")
	}
	delete(s.jobs, id)
	filtered := s.jobOrder[:0]
	for _, candidate := range s.jobOrder {
		if candidate != id {
			filtered = append(filtered, candidate)
		}
	}
	s.jobOrder = filtered
	s.removeMirroredSyncJobLocked(id)
	return nil
}

func (s *Server) clearFinishedSyncJobs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.jobOrder[:0]
	removed := 0
	for _, id := range s.jobOrder {
		job := s.jobs[id]
		if job != nil && job.State == "running" {
			kept = append(kept, id)
			continue
		}
		delete(s.jobs, id)
		s.removeMirroredSyncJobLocked(id)
		removed++
	}
	s.jobOrder = kept
	return removed
}

func (s *Server) retrySyncJob(id string) (*syncJob, error) {
	startedAt := s.now().UTC()
	s.mu.Lock()
	job := s.restoreSyncJobLocked(id)
	if job == nil {
		s.mu.Unlock()
		return nil, errors.New("sync job not found")
	}
	if job.State == "running" {
		s.mu.Unlock()
		return nil, errors.New("running sync job cannot be retried")
	}
	resumeID := mediumRetryDraftID(*job)
	if job.Operation == "publish" || job.Operation == "publish-draft" ||
		(job.Operation == "create" && resumeID == "") ||
		(job.Operation == "update" && (job.Request.PublishAfter || len(job.Request.Targets) > 1)) {
		s.mu.Unlock()
		return nil, errors.New("create/publish or multi-target update cannot be retried safely; inspect the remote outcomes and explicitly select failed targets")
	}
	request := job.Request
	if (job.Operation == "create" || job.Operation == "update") && resumeID != "" {
		request.ResumeMediumDraftID = resumeID
	}
	replacement := newSyncJob(id, request, startedAt)
	s.jobs[id] = replacement
	s.jobOrder = moveJobToFront(s.jobOrder, id)
	s.mirrorSyncJobLocked(replacement)
	config := s.config
	response := cloneSyncJob(replacement)
	s.mu.Unlock()

	s.launchSyncJob(id, request, config)
	return response, nil
}
