package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/bridge"
	"github.com/ThinkerQAQ/ThinkerQAQ.github.io/tools/blogctl/internal/version"
)

const (
	nativeHostName       = "com.thinkerqaq.blogctl"
	maxNativeMessageSize = 256 << 10
)

type bridgeState struct {
	BaseURL string `json:"baseUrl"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
}

type nativeRequest struct {
	Command string `json:"command"`
}

func isNativeMessagingInvocation(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return args[0] == "--native-host" ||
		strings.HasPrefix(args[0], "chrome-extension://") ||
		strings.HasPrefix(args[0], "edge-extension://")
}

func bridgeStatePath() (string, error) {
	dir, err := bridge.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bridge.json"), nil
}

func openBridgeLog() (*os.File, error) {
	statePath, err := bridgeStatePath()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(statePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(dir, "bridge.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

func writeBridgeState(state bridgeState) error {
	path, err := bridgeStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readBridgeState() (bridgeState, error) {
	path, err := bridgeStatePath()
	if err != nil {
		return bridgeState{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return bridgeState{}, err
	}
	var state bridgeState
	if err := json.Unmarshal(data, &state); err != nil {
		return bridgeState{}, err
	}
	if state.BaseURL == "" || len(state.Token) < 32 || state.PID <= 0 {
		return bridgeState{}, errors.New("invalid BlogCTL bridge state")
	}
	return state, nil
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func pingBridge(state bridgeState) error {
	request, err := http.NewRequest(http.MethodGet, state.BaseURL+"/v1/health", nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("bridge returned %s", response.Status)
	}
	return nil
}

func ensureBridgeProcess() (bridgeState, error) {
	if state, err := readBridgeState(); err == nil && pingBridge(state) == nil {
		return state, nil
	}

	command, err := newBridgeCommand()
	if err != nil {
		return bridgeState{}, err
	}
	if err := command.Start(); err != nil {
		return bridgeState{}, err
	}
	if command.Process != nil {
		_ = command.Process.Release()
	}

	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		state, err := readBridgeState()
		if err != nil {
			lastErr = err
			continue
		}
		if err := pingBridge(state); err != nil {
			lastErr = err
			continue
		}
		return state, nil
	}
	if lastErr != nil {
		return bridgeState{}, fmt.Errorf("BlogCTL Bridge did not start: %w", lastErr)
	}
	return bridgeState{}, errors.New("BlogCTL Bridge did not start")
}

func runBridgeProcess() error {
	if err := bridge.ConfigureLogging(); err != nil {
		return fmt.Errorf("configure bridge logging: %w", err)
	}
	defer bridge.CloseLogging()
	slog.Info("bridge starting", "operation", "startup", "pid", os.Getpid())
	token, err := randomToken()
	if err != nil {
		return err
	}
	server, err := bridge.New(token)
	if err != nil {
		return err
	}
	listener, httpServer, err := server.Listen(bridge.DefaultAddress)
	if err != nil {
		return err
	}
	state := bridgeState{BaseURL: bridge.Origin(listener), Token: token, PID: os.Getpid()}
	if err := writeBridgeState(state); err != nil {
		_ = listener.Close()
		return err
	}
	slog.Info("bridge ready", "operation", "startup", "address", bridge.DefaultAddress, "pid", state.PID)

	restartRequested := make(chan struct{}, 1)
	server.SetRestart(func() {
		select {
		case restartRequested <- struct{}{}:
		default:
		}
	})
	<-restartRequested
	slog.Info("bridge restarting", "operation", "restart", "pid", state.PID)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := bridge.Shutdown(ctx, httpServer); err != nil {
		return err
	}

	command, err := newBridgeCommand()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	if command.Process != nil {
		_ = command.Process.Release()
	}
	return nil
}

func runNativeHost() error {
	for {
		message, err := readNativeMessage(os.Stdin)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		var request nativeRequest
		if err := json.Unmarshal(message, &request); err != nil {
			_ = writeNativeMessage(os.Stdout, map[string]any{"ok": false, "error": "invalid request"})
			continue
		}
		if request.Command != "ensure_bridge" && request.Command != "status" {
			_ = writeNativeMessage(os.Stdout, map[string]any{"ok": false, "error": "unknown command"})
			continue
		}
		state, err := ensureBridgeProcess()
		if err != nil {
			_ = writeNativeMessage(os.Stdout, map[string]any{"ok": false, "error": err.Error()})
			continue
		}
		executablePath, _ := os.Executable()
		if err := writeNativeMessage(os.Stdout, map[string]any{
			"ok": true, "baseUrl": state.BaseURL, "token": state.Token, "pid": state.PID,
			"nativeHostVersion": version.Current, "executablePath": executablePath,
		}); err != nil {
			return err
		}
	}
}

func readNativeMessage(reader io.Reader) ([]byte, error) {
	var size uint32
	if err := binary.Read(reader, binary.LittleEndian, &size); err != nil {
		return nil, err
	}
	if size == 0 || size > maxNativeMessageSize {
		return nil, errors.New("invalid native message size")
	}
	message := make([]byte, size)
	_, err := io.ReadFull(reader, message)
	return message, err
}

func writeNativeMessage(writer io.Writer, value any) error {
	message, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := binary.Write(writer, binary.LittleEndian, uint32(len(message))); err != nil {
		return err
	}
	_, err = writer.Write(message)
	return err
}
