package bridge

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	logFileName     = "bridge.log"
	defaultLogTail  = 500
	maxLogTail      = 2000
	maxLogReadBytes = 2 * 1024 * 1024
)

type logEntry struct {
	Time       string         `json:"time,omitempty"`
	Level      string         `json:"level,omitempty"`
	Message    string         `json:"message,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Raw        string         `json:"raw,omitempty"`
}

var bridgeLoggingState struct {
	sync.Mutex
	file *os.File
	path string
}

func resolvedLogDirectory(config bridgeConfig) (string, error) {
	if directory := strings.TrimSpace(config.LogDirectory); directory != "" {
		if filepath.IsAbs(directory) {
			return filepath.Clean(directory), nil
		}
		absolute, err := filepath.Abs(directory)
		if err != nil {
			return "", err
		}
		return filepath.Clean(absolute), nil
	}
	configPath, err := ConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Dir(configPath), nil
}

func resolvedLogFilePath(config bridgeConfig) (string, error) {
	directory, err := resolvedLogDirectory(config)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, logFileName), nil
}

func logLevelValue(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, errors.New("log level must be debug, info, warn, or error")
	}
}

func applyLoggingConfig(config bridgeConfig) error {
	level, err := logLevelValue(config.LogLevel)
	if err != nil {
		return err
	}
	path, err := resolvedLogFilePath(config)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(file, &slog.HandlerOptions{Level: level}))

	bridgeLoggingState.Lock()
	previous := bridgeLoggingState.file
	bridgeLoggingState.file = file
	bridgeLoggingState.path = path
	slog.SetDefault(logger)
	bridgeLoggingState.Unlock()

	if previous != nil {
		_ = previous.Close()
	}
	return nil
}

// ConfigureLogging loads the persisted BlogCTL configuration and installs the
// process-wide structured logger before the Bridge starts serving requests.
func ConfigureLogging() error {
	return applyLoggingConfig(loadBridgeConfig())
}

// CloseLogging releases the current log file during Bridge shutdown.
func CloseLogging() {
	bridgeLoggingState.Lock()
	file := bridgeLoggingState.file
	bridgeLoggingState.file = nil
	bridgeLoggingState.path = ""
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	bridgeLoggingState.Unlock()
	if file != nil {
		_ = file.Close()
	}
}

func loggingHealth(config bridgeConfig) toolHealth {
	path, err := resolvedLogFilePath(config)
	if err != nil {
		return toolHealth{Status: "error", Summary: "日志配置无效", Detail: err.Error()}
	}
	level := strings.ToUpper(strings.TrimSpace(config.LogLevel))
	if level == "" {
		level = "INFO"
	}
	return toolHealth{
		OK: true, Status: "ok", Summary: level,
		Path: path, Detail: "结构化 JSON 日志；配置保存后立即生效。",
	}
}

func normalizedLogTailLimit(limit int) int {
	if limit <= 0 {
		return defaultLogTail
	}
	if limit > maxLogTail {
		return maxLogTail
	}
	return limit
}

func readLogTail(config bridgeConfig, limit int) ([]logEntry, string, error) {
	path, err := resolvedLogFilePath(config)
	if err != nil {
		return nil, "", err
	}
	limit = normalizedLogTailLimit(limit)

	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []logEntry{}, path, nil
	}
	if err != nil {
		return nil, path, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, path, err
	}
	start := int64(0)
	if info.Size() > maxLogReadBytes {
		start = info.Size() - maxLogReadBytes
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return nil, path, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, path, err
	}
	lines := strings.Split(string(data), "\n")
	if start > 0 && len(lines) > 0 {
		// The first line may be partial because we intentionally read only the tail.
		lines = lines[1:]
	}

	entries := make([]logEntry, 0, min(limit, len(lines)))
	for index := len(lines) - 1; index >= 0 && len(entries) < limit; index-- {
		line := strings.TrimSpace(lines[index])
		if line == "" {
			continue
		}
		entry := logEntry{Raw: line}
		var payload map[string]any
		if json.Unmarshal([]byte(line), &payload) == nil {
			if value, ok := payload["time"].(string); ok {
				entry.Time = value
			}
			if value, ok := payload["level"].(string); ok {
				entry.Level = value
			}
			if value, ok := payload["msg"].(string); ok {
				entry.Message = value
			}
			delete(payload, "time")
			delete(payload, "level")
			delete(payload, "msg")
			if len(payload) > 0 {
				entry.Attributes = payload
			}
			entry.Raw = ""
		}
		entries = append(entries, entry)
	}
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
	return entries, path, nil
}

func clearLog(config bridgeConfig) (string, error) {
	path, err := resolvedLogFilePath(config)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return path, err
	}
	if err := os.Truncate(path, 0); err != nil && !errors.Is(err, os.ErrNotExist) {
		return path, err
	}
	slog.Info("log cleared", "operation", "logs-clear")
	return path, nil
}

func (s *Server) handleLogsGet(response http.ResponseWriter, request *http.Request) {
	if !allowReadOnlyBridgeStatus(response, request) {
		return
	}
	limit, _ := strconv.Atoi(request.URL.Query().Get("limit"))
	s.mu.Lock()
	config := s.config
	s.mu.Unlock()
	entries, path, err := readLogTail(config, limit)
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "log_read_failed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"path":    path,
		"level":   config.LogLevel,
		"entries": entries,
	})
}

func (s *Server) handleLogsClear(response http.ResponseWriter, request *http.Request) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	s.mu.Lock()
	config := s.config
	s.mu.Unlock()
	path, err := clearLog(config)
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "log_clear_failed", err.Error(), nil)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"ok": true, "path": path})
}

func (s *Server) handleLogsEvent(response http.ResponseWriter, request *http.Request) {
	if _, ok := allowExtensionWrite(response, request); !ok {
		return
	}
	var input struct {
		Level   string         `json:"level"`
		Message string         `json:"message"`
		Fields  map[string]any `json:"fields"`
	}
	if err := readJSON(request, maxBodyBytes, &input); err != nil {
		writeError(response, err)
		return
	}
	input.Level = strings.ToLower(strings.TrimSpace(input.Level))
	input.Message = strings.TrimSpace(input.Message)
	if input.Message == "" {
		writeAPIError(response, http.StatusBadRequest, "invalid_log_event", "message is required", nil)
		return
	}
	attrs := make([]any, 0, len(input.Fields)*2+2)
	attrs = append(attrs, "source", "extension")
	for key, value := range input.Fields {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "cookie", "authorization", "token", "private_key", "serviceaccountjson":
			continue
		}
		attrs = append(attrs, key, value)
	}
	switch input.Level {
	case "debug":
		slog.Debug(input.Message, attrs...)
	case "warn":
		slog.Warn(input.Message, attrs...)
	case "error":
		slog.Error(input.Message, attrs...)
	default:
		slog.Info(input.Message, attrs...)
	}
	writeJSON(response, http.StatusOK, map[string]any{"ok": true})
}
