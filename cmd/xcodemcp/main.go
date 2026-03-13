package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/oozoofrog/xcodemcp-cli/internal/bridge"
	"github.com/oozoofrog/xcodemcp-cli/internal/doctor"
	"github.com/oozoofrog/xcodemcp-cli/internal/mcp"
)

var defaultBridgeCommand = bridge.Command{Path: "xcrun", Args: []string{"mcpbridge"}}
var defaultMCPCommand = mcp.Command{Path: "xcrun", Args: []string{"mcpbridge"}}
var defaultSessionPathFunc = bridge.DefaultSessionFilePath

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Environ()))
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, env []string) int {
	cfg, usage, err := parseCLI(args)
	if err != nil {
		if errors.Is(err, errUsageRequested) {
			fmt.Fprint(stdout, usage)
			return 0
		}
		fmt.Fprintf(stderr, "xcodemcp: %v\n", err)
		if usage != "" {
			fmt.Fprint(stderr, usage)
		}
		return 1
	}

	if runtime.GOOS != "darwin" {
		fmt.Fprintln(stderr, "xcodemcp: only macOS (darwin) is supported")
		return 1
	}

	sessionPath, err := defaultSessionPathFunc()
	if err != nil {
		fmt.Fprintf(stderr, "xcodemcp: %v\n", err)
		return 1
	}

	resolved, err := bridge.ResolveOptions(env, bridge.EnvOptions{
		XcodePID:  cfg.XcodePID,
		SessionID: cfg.SessionID,
	}, sessionPath)
	if err != nil {
		fmt.Fprintf(stderr, "xcodemcp: %v\n", err)
		return 1
	}
	effective := resolved.EnvOptions
	if cfg.Debug {
		logResolvedSession(stderr, resolved)
	}

	switch cfg.Command {
	case commandDoctor:
		report := doctor.NewInspector().Run(ctx, doctor.Options{
			BaseEnv:       env,
			XcodePID:      effective.XcodePID,
			SessionID:     effective.SessionID,
			SessionSource: resolved.SessionSource,
			SessionPath:   resolved.SessionPath,
		})
		fmt.Fprint(stdout, report.String())
		if report.Success() {
			return 0
		}
		return 1
	case commandBridge:
		if err := bridge.ValidateEnvOptions(effective); err != nil {
			fmt.Fprintf(stderr, "xcodemcp: invalid bridge options: %v\n", err)
			return 1
		}

		result, err := bridge.Run(ctx, bridge.Config{
			Command: defaultBridgeCommand,
			Env:     bridge.ApplyEnvOverrides(env, effective),
			In:      stdin,
			Out:     stdout,
			ErrOut:  stderr,
			Debug:   cfg.Debug,
		})
		if err != nil {
			fmt.Fprintf(stderr, "xcodemcp: %v\n", err)
			return 1
		}
		return result.ExitCode
	case commandToolsList:
		if err := bridge.ValidateEnvOptions(effective); err != nil {
			fmt.Fprintf(stderr, "xcodemcp: invalid MCP options: %v\n", err)
			return 1
		}
		cmdCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
		tools, err := mcp.ListTools(cmdCtx, mcp.Config{
			Command: defaultMCPCommand,
			Env:     bridge.ApplyEnvOverrides(env, effective),
			Debug:   cfg.Debug,
			ErrOut:  stderr,
		})
		if err != nil {
			fmt.Fprintf(stderr, "xcodemcp: %v\n", err)
			return 1
		}
		if cfg.JSONOutput {
			if err := writeJSON(stdout, tools); err != nil {
				fmt.Fprintf(stderr, "xcodemcp: %v\n", err)
				return 1
			}
			return 0
		}
		for _, tool := range tools {
			name, _ := tool["name"].(string)
			description, _ := tool["description"].(string)
			if description != "" {
				fmt.Fprintf(stdout, "%s\t%s\n", name, description)
			} else {
				fmt.Fprintln(stdout, name)
			}
		}
		return 0
	case commandToolCall:
		if err := bridge.ValidateEnvOptions(effective); err != nil {
			fmt.Fprintf(stderr, "xcodemcp: invalid MCP options: %v\n", err)
			return 1
		}
		arguments, err := parseJSONObject(cfg.ToolInputJSON)
		if err != nil {
			fmt.Fprintf(stderr, "xcodemcp: %v\n", err)
			return 1
		}
		cmdCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
		result, err := mcp.CallTool(cmdCtx, mcp.Config{
			Command: defaultMCPCommand,
			Env:     bridge.ApplyEnvOverrides(env, effective),
			Debug:   cfg.Debug,
			ErrOut:  stderr,
		}, cfg.ToolName, arguments)
		if err != nil {
			fmt.Fprintf(stderr, "xcodemcp: %v\n", err)
			return 1
		}
		if err := writeJSON(stdout, result.Result); err != nil {
			fmt.Fprintf(stderr, "xcodemcp: %v\n", err)
			return 1
		}
		if result.IsError {
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "xcodemcp: unsupported command %q\n", cfg.Command)
		return 1
	}
}

func logResolvedSession(w io.Writer, resolved bridge.ResolvedOptions) {
	switch resolved.SessionSource {
	case bridge.SessionSourceExplicit:
		fmt.Fprintln(w, "[debug] using MCP_XCODE_SESSION_ID from --session-id")
	case bridge.SessionSourceEnv:
		fmt.Fprintln(w, "[debug] using MCP_XCODE_SESSION_ID from environment")
	case bridge.SessionSourcePersisted:
		fmt.Fprintf(w, "[debug] using persisted MCP_XCODE_SESSION_ID %s from %s\n", resolved.SessionID, resolved.SessionPath)
	case bridge.SessionSourceGenerated:
		fmt.Fprintf(w, "[debug] generated persistent MCP_XCODE_SESSION_ID %s at %s\n", resolved.SessionID, resolved.SessionPath)
	}
}

func parseJSONObject(raw string) (map[string]any, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("--json must be valid JSON: %w", err)
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("--json must decode to a JSON object")
	}
	return obj, nil
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write JSON output: %w", err)
	}
	return nil
}
