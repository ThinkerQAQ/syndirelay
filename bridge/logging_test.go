package bridge

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggingDefaultsAndToolRegistry(t *testing.T) {
	useIsolatedUserConfigDir(t)
	config := defaultBridgeConfig()
	if config.LogLevel != "info" {
		t.Fatalf("default log level = %q", config.LogLevel)
	}
	found := false
	for _, tool := range toolRegistry(config) {
		if tool.Name != "logging" {
			continue
		}
		found = true
		if tool.Health.Status != "ok" {
			t.Fatalf("logging health = %#v", tool.Health)
		}
		if len(tool.Config.Schema) != 2 {
			t.Fatalf("logging schema = %#v", tool.Config.Schema)
		}
	}
	if !found {
		t.Fatal("logging tool missing")
	}
}

func TestApplyLoggingConfigWritesAndReadsStructuredLog(t *testing.T) {
	useIsolatedUserConfigDir(t)
	config := defaultBridgeConfig()
	config.LogDirectory = filepath.Join(t.TempDir(), "logs")
	config.LogLevel = "debug"

	if err := applyLoggingConfig(config); err != nil {
		t.Fatal(err)
	}
	defer CloseLogging()

	slog.Debug("debug message", "component", "test")
	slog.Info("info message", "operation", "test")

	entries, path, err := readLogTail(config, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(filepath.ToSlash(path), "/logs/bridge.log") {
		t.Fatalf("log path = %q", path)
	}
	if len(entries) < 2 {
		t.Fatalf("entries = %#v", entries)
	}
	foundDebug := false
	foundInfo := false
	for _, entry := range entries {
		switch entry.Message {
		case "debug message":
			foundDebug = entry.Level == "DEBUG"
		case "info message":
			foundInfo = entry.Level == "INFO"
		}
	}
	if !foundDebug || !foundInfo {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestNormalizeBridgeConfigRejectsInvalidLogLevel(t *testing.T) {
	config := defaultBridgeConfig()
	config.LogLevel = "verbose"
	if _, err := normalizeBridgeConfig(config); err == nil {
		t.Fatal("invalid log level unexpectedly accepted")
	}
}
