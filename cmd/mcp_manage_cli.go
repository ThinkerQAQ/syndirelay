package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	blogbridge "github.com/ThinkerQAQ/syndirelay/bridge"
)

// CLI uses the same native management implementation as the GUI.
// Credentials are read from standard input and never accepted as CLI flags.
func runMCPManagement(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: syndirelay mcp status|configure|start|stop|restart|connect|disconnect|auto-status|auto-enable|auto-disable|auto-connect-enable|auto-connect-disable")
	}
	if args[0] == "configure" {
		var in struct {
			TunnelID      string `json:"tunnel_id"`
			RuntimeAPIKey string `json:"runtime_api_key"`
			ReuseKey      bool   `json:"reuse_key"`
		}
		decoder := json.NewDecoder(io.LimitReader(os.Stdin, 8193))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil {
			return fmt.Errorf("read MCP configuration from stdin: %w", err)
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return errors.New("unexpected extra MCP configuration input")
		}
		if in.RuntimeAPIKey != "" && in.ReuseKey {
			return errors.New("cannot provide a key and reuse the saved key simultaneously")
		}
		return blogbridge.ConfigureLocalMCP(context.Background(), in.TunnelID, in.RuntimeAPIKey, in.ReuseKey)
	}
	status, err := blogbridge.ManageLocalMCP(args[0])
	if err != nil {
		return err
	}
	if status != nil {
		return json.NewEncoder(os.Stdout).Encode(status)
	}
	return nil
}
