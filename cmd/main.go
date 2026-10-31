package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	blogbridge "github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/bridge"
)

type app struct {
	root        string
	contentRoot string
	runner      commandRunner
	out         io.Writer
}

func main() {
	if isNativeMessagingInvocation(os.Args[1:]) {
		if err := runNativeHost(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "install" || os.Args[1] == "uninstall") {
		if err := runNativeHostSetup(os.Args[1], os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "blogctl:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "-mcp-stdio" || os.Args[1] == "mcp-stdio") {
		if err := runMCPStdio(); err != nil {
			fmt.Fprintln(os.Stderr, "blogctl mcp:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--bridge" {
		if err := runBridgeProcess(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	root, contentRoot, err := rootsForCommand(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	a := app{root: root, contentRoot: contentRoot, runner: osRunner{dir: root}, out: os.Stdout}
	if err := a.run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "blogctl:", err)
		os.Exit(1)
	}
}

func rootsForCommand(args []string) (string, string, error) {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		return "", "", nil
	}
	command := args[0]
	if command == "social" {
		return "", "", nil
	}
	if command == "sync" || command == "toutiao" {
		return resolveSyncWorkspace()
	}
	root, err := findRepositoryRoot()
	return root, "", err
}

func (a app) run(args []string) error {
	command := "help"
	if len(args) > 0 {
		command, args = args[0], args[1:]
	}
	switch command {
	case "help", "-h", "--help":
		a.printHelp()
		return nil
	case "preview":
		return a.runNPM(true, "run", "preview")
	case "dev":
		return a.runNPM(true, "run", "dev")
	case "build":
		return a.runSite([]string{"build"})
	case "site":
		return a.runSite(args)
	case "ai-search":
		return a.runAISearch(args)
	case "check":
		return a.runCheck()
	case "test":
		return a.runNPM(true, "test")
	case "diagrams":
		return a.runDiagrams(args)
	case "search":
		return a.runSearch(args)
	case "sync":
		return a.runSync(args)
	case "social":
		return a.runSocial(args)
	case "doctor":
		return a.doctor()
	case "toutiao":
		return runToutiaoProbe(args, a.out)
	default:
		if cliLocale() == "zh-CN" {
			return fmt.Errorf("未知命令 %q；请运行 blogctl help", command)
		}
		return fmt.Errorf("unknown command %q; run blogctl help", command)
	}
}

func (a app) runCheck() error {
	return a.runNPM(true, "run", "check")
}

func (a app) doctor() error {
	failed := false
	for _, name := range []string{"node", "npm", "git"} {
		path, err := lookPath(name)
		if err != nil {
			if cliLocale() == "zh-CN" {
				fmt.Fprintf(a.out, "%-8s 未安装\n", name)
			} else {
				fmt.Fprintf(a.out, "%-8s missing\n", name)
			}
			failed = true
			continue
		}
		fmt.Fprintf(a.out, "%-8s %s\n", name, path)
	}
	fmt.Fprintf(a.out, "%-8s %s/%s\n", "platform", runtime.GOOS, runtime.GOARCH)
	if failed {
		if cliLocale() == "zh-CN" {
			return errors.New("缺少一个或多个必需工具")
		}
		return errors.New("one or more required tools are missing")
	}
	return nil
}

func cliLocale() string {
	configured := strings.TrimSpace(os.Getenv("BLOGCTL_LANG"))
	if configured == "" {
		configured = blogbridge.UILocalePreference()
	}
	switch strings.ToLower(configured) {
	case "zh", "zh-cn", "zh_cn":
		return "zh-CN"
	case "en", "en-us", "en_gb":
		return "en"
	}
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		locale := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
		if strings.HasPrefix(locale, "zh") {
			return "zh-CN"
		}
		if strings.HasPrefix(locale, "en") {
			return "en"
		}
	}
	return "en"
}

func (a app) printHelp() {
	if cliLocale() == "zh-CN" {
		fmt.Fprintln(a.out, `blogctl - ThinkerQAQ 博客开发与分发工具

用法：
  blogctl help
  blogctl preview | dev | build | check | test
  blogctl site build --content-root <path>
  blogctl ai-search <prepare|sync|verify> [options]
  blogctl diagrams [plantuml|drawio]
  blogctl search <build|inventory|submit|audit|notify> [options]
  blogctl sync --article <slug> --platforms <list> [--dry-run] [--changed] [--draft]
  blogctl social <channels|source|create|list|show|stage|reconcile|publish> [options]
  blogctl doctor
  blogctl install | uninstall
  blogctl toutiao probe --har <HAR-file> [--confirm-create-draft]

文章分发 sync 请在 blog-content 仓库执行；网站构建命令在 ThinkerQAQ.github.io 引擎仓库执行。
scripts/ 存放仓库级 Node 构建、验证和渲染脚本。`)
		return
	}

	fmt.Fprintln(a.out, `blogctl - Inkerio blog developer tool

Usage:
  blogctl
  blogctl preview | dev | build | check | test
  blogctl site build --content-root <path>
  blogctl ai-search <prepare|sync|verify> [options]
  blogctl diagrams [plantuml|drawio]
  blogctl search <build|inventory|submit|audit|notify> [options]
  blogctl sync --article <slug> --platforms <list> [--dry-run] [--changed] [--draft]
  blogctl social <channels|source|create|list|show|stage|reconcile|publish> [options]
  blogctl doctor
  blogctl install | uninstall
  blogctl toutiao probe --har <HAR-file> [--confirm-create-draft]

Run sync from the blog-content repository. Engine commands still run from ThinkerQAQ.github.io.
scripts/ contains repository-level Node build, validation and rendering entrypoints.`)
}

func findRepositoryRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root := findAncestor(current, isEngineRoot)
	if root == "" {
		return "", errors.New("not inside the ThinkerQAQ public engine repository")
	}
	return root, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func cleanAbsolutePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}
