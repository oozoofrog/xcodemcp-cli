package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oozoofrog/xcodemcp-cli/internal/agent"
	"github.com/oozoofrog/xcodemcp-cli/internal/bridge"
	"github.com/oozoofrog/xcodemcp-cli/internal/mcp"
)

func TestParseCLIDefaultBridge(t *testing.T) {
	cfg, usage, err := parseCLI([]string{"--xcode-pid", "123", "--session-id", "11111111-1111-1111-1111-111111111111", "--debug"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandBridge {
		t.Fatalf("command = %q, want %q", cfg.Command, commandBridge)
	}
	if cfg.XcodePID != "123" || cfg.SessionID != "11111111-1111-1111-1111-111111111111" || !cfg.Debug {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if !strings.Contains(usage, "xcodemcp bridge") {
		t.Fatalf("usage missing bridge help: %q", usage)
	}
}

func TestParseCLIToolsList(t *testing.T) {
	cfg, _, err := parseCLI([]string{"tools", "list", "--json", "--timeout", "45s"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandToolsList {
		t.Fatalf("command = %q, want %q", cfg.Command, commandToolsList)
	}
	if !cfg.JSONOutput || cfg.Timeout != 45*time.Second {
		t.Fatalf("unexpected tools list config: %+v", cfg)
	}
}

func TestParseCLIToolCall(t *testing.T) {
	cfg, _, err := parseCLI([]string{"tool", "call", "build_sim", "--json", `{"scheme":"Demo"}`, "--timeout", "15s"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandToolCall {
		t.Fatalf("command = %q, want %q", cfg.Command, commandToolCall)
	}
	if cfg.ToolName != "build_sim" || cfg.ToolInputJSON != `{"scheme":"Demo"}` || cfg.Timeout != 15*time.Second {
		t.Fatalf("unexpected tool call config: %+v", cfg)
	}
}

func TestParseCLIAgentStatus(t *testing.T) {
	cfg, _, err := parseCLI([]string{"agent", "status"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandAgentStatus {
		t.Fatalf("command = %q, want %q", cfg.Command, commandAgentStatus)
	}
}

func TestParseCLIHelp(t *testing.T) {
	_, usage, err := parseCLI([]string{"help", "agent", "status"})
	if err != errUsageRequested {
		t.Fatalf("err = %v, want errUsageRequested", err)
	}
	if !strings.Contains(usage, "xcodemcp agent status") {
		t.Fatalf("usage missing agent status help: %q", usage)
	}
}

func TestParseCLIUnknownCommand(t *testing.T) {
	_, _, err := parseCLI([]string{"unknown"})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func TestRunRejectsInvalidBridgeOptions(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder
	code := run(context.Background(), []string{"--xcode-pid", "0"}, strings.NewReader(""), &stdout, &stderr, []string{})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "invalid bridge options") {
		t.Fatalf("stderr = %q, want invalid options message", stderr.String())
	}
}

func TestRunToolsListJSON(t *testing.T) {
	withAgentStubs(t, func() {
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			return []map[string]any{{"name": "build_sim", "description": "Build for simulator"}, {"name": "launch_app_sim"}}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"tools", "list", "--json"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		var tools []map[string]any
		if err := json.Unmarshal([]byte(stdout.String()), &tools); err != nil {
			t.Fatalf("stdout is not JSON array: %v (stdout=%q)", err, stdout.String())
		}
		if len(tools) != 2 {
			t.Fatalf("len(tools) = %d, want 2", len(tools))
		}
	})
}

func TestRunToolsListGeneratesPersistentSessionID(t *testing.T) {
	withAgentStubs(t, func() {
		oldSessionPathFunc := defaultSessionPathFunc
		sessionPath := filepath.Join(t.TempDir(), "session-id")
		defaultSessionPathFunc = func() (string, error) { return sessionPath, nil }
		defer func() { defaultSessionPathFunc = oldSessionPathFunc }()

		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			if !bridge.IsValidUUID(req.SessionID) {
				t.Fatalf("req.SessionID = %q, want valid UUID", req.SessionID)
			}
			return []map[string]any{{"name": "list_windows"}}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"tools", "list", "--json", "--debug"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		data, err := os.ReadFile(sessionPath)
		if err != nil {
			t.Fatalf("ReadFile(%q) failed: %v", sessionPath, err)
		}
		sessionID := strings.TrimSpace(string(data))
		if !bridge.IsValidUUID(sessionID) {
			t.Fatalf("persisted session ID is invalid: %q", sessionID)
		}
		if !strings.Contains(stderr.String(), "generated persistent MCP_XCODE_SESSION_ID "+sessionID) {
			t.Fatalf("stderr = %q, want generated session debug log", stderr.String())
		}
	})
}

func TestRunToolsListReusesPersistentSessionID(t *testing.T) {
	withAgentStubs(t, func() {
		oldSessionPathFunc := defaultSessionPathFunc
		sessionPath := filepath.Join(t.TempDir(), "session-id")
		wantSessionID := "44444444-4444-4444-8444-444444444444"
		if err := os.WriteFile(sessionPath, []byte(wantSessionID+"\n"), 0o600); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		defaultSessionPathFunc = func() (string, error) { return sessionPath, nil }
		defer func() { defaultSessionPathFunc = oldSessionPathFunc }()

		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			if req.SessionID != wantSessionID {
				t.Fatalf("req.SessionID = %q, want %q", req.SessionID, wantSessionID)
			}
			return []map[string]any{{"name": "list_windows"}}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"tools", "list", "--json", "--debug"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		if !strings.Contains(stderr.String(), "using persisted MCP_XCODE_SESSION_ID "+wantSessionID) {
			t.Fatalf("stderr = %q, want persisted session debug log", stderr.String())
		}
	})
}

func TestRunToolCallIsErrorExitsOne(t *testing.T) {
	withAgentStubs(t, func() {
		defaultToolCallFunc = func(ctx context.Context, cfg agent.Config, req agent.Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
			return mcp.CallResult{Result: map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": "boom"}}}, IsError: true}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"tool", "call", "build_sim", "--json", `{"scheme":"Demo"}`}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stdout.String(), `"isError": true`) {
			t.Fatalf("stdout = %q, want tool result JSON", stdout.String())
		}
		if stderr.String() != "" {
			t.Fatalf("stderr = %q, want empty stderr", stderr.String())
		}
	})
}

func TestRunRejectsNonObjectToolJSON(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder
	code := run(context.Background(), []string{"tool", "call", "build_sim", "--json", `[]`}, strings.NewReader(""), &stdout, &stderr, os.Environ())
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "must decode to a JSON object") {
		t.Fatalf("stderr = %q, want JSON object error", stderr.String())
	}
}

func TestRunAgentStatus(t *testing.T) {
	withAgentStubs(t, func() {
		defaultAgentStatusFunc = func(ctx context.Context, cfg agent.Config) (agent.Status, error) {
			return agent.Status{
				Label:             agent.LaunchAgentLabel,
				PlistInstalled:    true,
				PlistPath:         "/tmp/io.oozoofrog.xcodemcp.plist",
				RegisteredBinary:  "/tmp/xcodemcp",
				CurrentBinary:     "/tmp/xcodemcp",
				BinaryPathMatches: true,
				SocketPath:        "/tmp/daemon.sock",
				SocketReachable:   true,
				Running:           true,
				PID:               123,
				IdleTimeout:       10 * time.Minute,
				BackendSessions:   2,
			}, nil
		}
		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"agent", "status"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "backend sessions: 2") {
			t.Fatalf("stdout = %q, want status output", stdout.String())
		}
	})
}

func withAgentStubs(t *testing.T, fn func()) {
	t.Helper()
	oldConfig := defaultAgentConfigFunc
	oldList := defaultToolsListFunc
	oldCall := defaultToolCallFunc
	oldStatus := defaultAgentStatusFunc
	oldStop := defaultAgentStopFunc
	oldUninstall := defaultAgentUninstallFunc
	oldRun := defaultAgentRunFunc
	defaultAgentConfigFunc = func(command mcp.Command, env []string, errOut io.Writer) (agent.Config, error) {
		return agent.Config{}, nil
	}
	defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
		return nil, errors.New("unexpected tools list call")
	}
	defaultToolCallFunc = func(ctx context.Context, cfg agent.Config, req agent.Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
		return mcp.CallResult{}, errors.New("unexpected tool call")
	}
	defaultAgentStatusFunc = func(ctx context.Context, cfg agent.Config) (agent.Status, error) {
		return agent.Status{}, nil
	}
	defaultAgentStopFunc = func(ctx context.Context, cfg agent.Config) error { return nil }
	defaultAgentUninstallFunc = func(ctx context.Context, cfg agent.Config) error { return nil }
	defaultAgentRunFunc = func(ctx context.Context, cfg agent.Config) error { return nil }
	defer func() {
		defaultAgentConfigFunc = oldConfig
		defaultToolsListFunc = oldList
		defaultToolCallFunc = oldCall
		defaultAgentStatusFunc = oldStatus
		defaultAgentStopFunc = oldStop
		defaultAgentUninstallFunc = oldUninstall
		defaultAgentRunFunc = oldRun
	}()
	fn()
}
