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

	"github.com/oozoofrog/xcodecli/internal/agent"
	"github.com/oozoofrog/xcodecli/internal/bridge"
	"github.com/oozoofrog/xcodecli/internal/doctor"
	"github.com/oozoofrog/xcodecli/internal/mcp"
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
	if !strings.Contains(usage, "xcodecli bridge") {
		t.Fatalf("usage missing bridge help: %q", usage)
	}
}

func TestParseCLIVersionCommand(t *testing.T) {
	cfg, usage, err := parseCLI([]string{"version"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandVersion {
		t.Fatalf("command = %q, want %q", cfg.Command, commandVersion)
	}
	if !strings.Contains(usage, "xcodecli version") {
		t.Fatalf("usage missing version help: %q", usage)
	}
}

func TestParseCLIVersionFlag(t *testing.T) {
	cfg, _, err := parseCLI([]string{"--version"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandVersion {
		t.Fatalf("command = %q, want %q", cfg.Command, commandVersion)
	}
}

func TestParseCLIWithoutArgsShowsHelp(t *testing.T) {
	withVersionState(t, "v1.2.3", "dev", func() {
		_, usage, err := parseCLI(nil)
		if err != errUsageRequested {
			t.Fatalf("err = %v, want errUsageRequested", err)
		}
		if !strings.Contains(usage, "START HERE:") {
			t.Fatalf("usage missing root help banner: %q", usage)
		}
		if !strings.HasPrefix(usage, "xcodecli v1.2.3 (dev)\n\n") {
			t.Fatalf("usage missing version header: %q", usage)
		}
	})
}

func TestParseCLIDoctorJSON(t *testing.T) {
	cfg, _, err := parseCLI([]string{"doctor", "--json"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandDoctor || !cfg.JSONOutput {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestParseCLIServe(t *testing.T) {
	cfg, _, err := parseCLI([]string{"serve", "--debug", "--session-id", "11111111-1111-1111-1111-111111111111"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandServe {
		t.Fatalf("command = %q, want %q", cfg.Command, commandServe)
	}
	if !cfg.Debug || cfg.SessionID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected serve config: %+v", cfg)
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

func TestParseCLIToolsListDefaultTimeout(t *testing.T) {
	cfg, _, err := parseCLI([]string{"tools", "list", "--json"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Timeout != defaultToolsListRequestTimeout {
		t.Fatalf("timeout = %s, want %s", cfg.Timeout, defaultToolsListRequestTimeout)
	}
}

func TestParseCLIToolInspect(t *testing.T) {
	cfg, _, err := parseCLI([]string{"tool", "inspect", "BuildProject", "--json", "--xcode-pid", "123"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandToolInspect {
		t.Fatalf("command = %q, want %q", cfg.Command, commandToolInspect)
	}
	if cfg.ToolName != "BuildProject" || !cfg.JSONOutput || cfg.XcodePID != "123" || cfg.Timeout != defaultToolInspectRequestTimeout {
		t.Fatalf("unexpected tool inspect config: %+v", cfg)
	}
}

func TestParseCLIToolInspectCustomTimeout(t *testing.T) {
	cfg, _, err := parseCLI([]string{"tool", "inspect", "BuildProject", "--timeout", "75s"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Timeout != 75*time.Second {
		t.Fatalf("timeout = %s, want 75s", cfg.Timeout)
	}
}

func TestParseCLIToolInspectCustomTimeoutBeforeName(t *testing.T) {
	cfg, _, err := parseCLI([]string{"tool", "inspect", "--timeout", "75s", "BuildProject"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.ToolName != "BuildProject" {
		t.Fatalf("tool name = %q, want BuildProject", cfg.ToolName)
	}
	if cfg.Timeout != 75*time.Second {
		t.Fatalf("timeout = %s, want 75s", cfg.Timeout)
	}
}

func TestParseCLIToolCallInlineJSON(t *testing.T) {
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

func TestParseCLIToolCallDefaultTimeoutsByTool(t *testing.T) {
	cases := []struct {
		args []string
		want time.Duration
	}{
		{[]string{"tool", "call", "BuildProject", "--json", "{}"}, defaultToolCallLongRequestTimeout},
		{[]string{"tool", "call", "XcodeRead", "--json", "{}"}, defaultToolCallReadRequestTimeout},
		{[]string{"tool", "call", "DocumentationSearch", "--json", "{}"}, defaultToolCallReadRequestTimeout},
		{[]string{"tool", "call", "XcodeWrite", "--json", "{}"}, defaultToolCallWriteRequestTimeout},
		{[]string{"tool", "call", "build_sim", "--json", "{}"}, defaultToolCallFallbackTimeout},
	}
	for _, tc := range cases {
		cfg, _, err := parseCLI(tc.args)
		if err != nil {
			t.Fatalf("parseCLI(%v) returned error: %v", tc.args, err)
		}
		if cfg.Timeout != tc.want {
			t.Fatalf("parseCLI(%v) timeout = %s, want %s", tc.args, cfg.Timeout, tc.want)
		}
	}
}

func TestParseCLIToolCallJSONStdin(t *testing.T) {
	cfg, _, err := parseCLI([]string{"tool", "call", "build_sim", "--json-stdin"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if !cfg.ToolInputFromStdin || cfg.ToolInputJSON != "" {
		t.Fatalf("unexpected tool call stdin config: %+v", cfg)
	}
}

func TestParseCLIToolCallRejectsConflictingInputs(t *testing.T) {
	_, _, err := parseCLI([]string{"tool", "call", "build_sim", "--json", `{"scheme":"Demo"}`, "--json-stdin"})
	if err == nil || !strings.Contains(err.Error(), "exactly one of --json or --json-stdin") {
		t.Fatalf("expected conflicting input error, got %v", err)
	}
}

func TestParseCLIAgentStatus(t *testing.T) {
	cfg, _, err := parseCLI([]string{"agent", "status", "--json"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandAgentStatus || !cfg.JSONOutput {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestParseCLIHelp(t *testing.T) {
	_, usage, err := parseCLI([]string{"help", "tool", "inspect"})
	if err != errUsageRequested {
		t.Fatalf("err = %v, want errUsageRequested", err)
	}
	if !strings.Contains(usage, "tool inspect") {
		t.Fatalf("usage missing tool inspect help: %q", usage)
	}
}

func TestRootUsageIncludesHumanAndAgentGuidance(t *testing.T) {
	withVersionState(t, "v9.9.9", "dev", func() {
		usage := rootUsage()
		for _, want := range []string{"xcodecli v9.9.9 (dev)", "START HERE:", "For humans:", "For agents:", "xcodecli version", "xcodecli serve", "xcodecli agent guide", "xcodecli agent demo", "xcodecli doctor --json", "xcodecli mcp codex", "xcodecli tool inspect <name> --json"} {
			if !strings.Contains(usage, want) {
				t.Fatalf("root usage missing %q: %s", want, usage)
			}
		}
	})
}

func TestVersionUsageMentionsVersionFlag(t *testing.T) {
	usage := versionUsage()
	for _, want := range []string{"xcodecli version", "xcodecli --version"} {
		if !strings.Contains(usage, want) {
			t.Fatalf("version usage missing %q: %s", want, usage)
		}
	}
}

func TestDoctorUsageMentionsJSONForAgents(t *testing.T) {
	usage := doctorUsage()
	for _, want := range []string{"doctor reports environment readiness", "Prefer --json", "--json"} {
		if !strings.Contains(usage, want) {
			t.Fatalf("doctor usage missing %q: %s", want, usage)
		}
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

func TestRunVersionCommand(t *testing.T) {
	withVersionState(t, "v9.9.9", "release", func() {
		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"version"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if strings.TrimSpace(stdout.String()) != "xcodecli v9.9.9" {
			t.Fatalf("stdout = %q, want version line", stdout.String())
		}
		if stderr.String() != "" {
			t.Fatalf("stderr = %q, want empty stderr", stderr.String())
		}
	})
}

func TestRunVersionFlag(t *testing.T) {
	withVersionState(t, "v1.2.3", "release", func() {
		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"--version"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if strings.TrimSpace(stdout.String()) != "xcodecli v1.2.3" {
			t.Fatalf("stdout = %q, want version line", stdout.String())
		}
	})
}

func TestRunHelpShowsVersionHeader(t *testing.T) {
	withVersionState(t, "v2.0.0", "dev", func() {
		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"help"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.HasPrefix(stdout.String(), "xcodecli v2.0.0 (dev)\n\n") {
			t.Fatalf("stdout = %q, want version header", stdout.String())
		}
		if stderr.String() != "" {
			t.Fatalf("stderr = %q, want empty stderr", stderr.String())
		}
	})
}

func TestCurrentVersionFallsBackToSourceVersion(t *testing.T) {
	withVersionState(t, "", "dev", func() {
		if got := currentVersion(); got != sourceVersion {
			t.Fatalf("currentVersion() = %q, want %q", got, sourceVersion)
		}
	})
}

func TestVersionLineMarksDevBuilds(t *testing.T) {
	withVersionState(t, "v1.2.3", "dev", func() {
		if got := versionLine(); got != "xcodecli v1.2.3 (dev)" {
			t.Fatalf("versionLine() = %q, want dev marker", got)
		}
	})
}

func TestRunDoctorJSON(t *testing.T) {
	withStubs(t, func() {
		defaultDoctorRunFunc = func(ctx context.Context, opts doctor.Options) doctor.Report {
			return doctor.Report{Checks: []doctor.Check{{Name: "stub", Status: doctor.StatusOK, Detail: "ok"}}}
		}
		defaultAgentStatusFunc = func(ctx context.Context, cfg agent.Config) (agent.Status, error) {
			return agent.Status{Label: agent.LaunchAgentLabel}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"doctor", "--json"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		var report doctor.JSONReport
		if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil {
			t.Fatalf("stdout is not JSON report: %v (stdout=%q)", err, stdout.String())
		}
		if !report.Success || len(report.Checks) == 0 {
			t.Fatalf("unexpected report: %+v", report)
		}
	})
}

func TestRunServeUsesPersistentSessionID(t *testing.T) {
	withStubs(t, func() {
		oldSessionPathFunc := defaultSessionPathFunc
		sessionPath := filepath.Join(t.TempDir(), "session-id")
		defaultSessionPathFunc = func() (string, error) { return sessionPath, nil }
		defer func() { defaultSessionPathFunc = oldSessionPathFunc }()

		oldServe := defaultMCPServeFunc
		defer func() { defaultMCPServeFunc = oldServe }()
		defaultMCPServeFunc = func(ctx context.Context, cfg mcp.ServerConfig, handler mcp.ServerHandler) error {
			tools, err := handler.ListTools(ctx)
			if err != nil {
				t.Fatalf("ListTools handler returned error: %v", err)
			}
			if len(tools) != 1 || tools[0]["name"] != "list_windows" {
				t.Fatalf("unexpected tools: %+v", tools)
			}
			return nil
		}
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			if !bridge.IsValidUUID(req.SessionID) {
				t.Fatalf("req.SessionID = %q, want valid UUID", req.SessionID)
			}
			return []map[string]any{{"name": "list_windows"}}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"serve", "--debug"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		if !strings.Contains(stderr.String(), "generated persistent MCP_XCODE_SESSION_ID") {
			t.Fatalf("stderr = %q, want session debug log", stderr.String())
		}
	})
}

func TestRunServePassesAgentRequestContextToHandlers(t *testing.T) {
	withStubs(t, func() {
		oldSessionPathFunc := defaultSessionPathFunc
		sessionPath := filepath.Join(t.TempDir(), "session-id")
		defaultSessionPathFunc = func() (string, error) { return sessionPath, nil }
		defer func() { defaultSessionPathFunc = oldSessionPathFunc }()

		defaultMCPServeFunc = func(ctx context.Context, cfg mcp.ServerConfig, handler mcp.ServerHandler) error {
			if cfg.ServerName != "xcodecli" {
				t.Fatalf("ServerName = %q, want xcodecli", cfg.ServerName)
			}
			if cfg.ServerVersion == "" {
				t.Fatal("ServerVersion should not be empty")
			}
			if !cfg.Debug {
				t.Fatal("Debug = false, want true")
			}

			tools, err := handler.ListTools(ctx)
			if err != nil {
				t.Fatalf("ListTools handler returned error: %v", err)
			}
			if len(tools) != 1 || tools[0]["name"] != "list_windows" {
				t.Fatalf("unexpected tools: %+v", tools)
			}

			callResult, err := handler.CallTool(ctx, "BuildProject", map[string]any{"tabIdentifier": "demo"})
			if err != nil {
				t.Fatalf("CallTool handler returned error: %v", err)
			}
			if callResult.Result["echoName"] != "BuildProject" {
				t.Fatalf("unexpected call result: %+v", callResult.Result)
			}
			return nil
		}

		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			if req.Timeout != 0 {
				t.Fatalf("req.Timeout = %s, want 0", req.Timeout)
			}
			if req.DeveloperDir != "/Applications/Xcode-beta.app/Contents/Developer" {
				t.Fatalf("DeveloperDir = %q, want inherited DEVELOPER_DIR", req.DeveloperDir)
			}
			if !req.Debug {
				t.Fatal("Debug = false, want true")
			}
			return []map[string]any{{"name": "list_windows"}}, nil
		}
		defaultToolCallFunc = func(ctx context.Context, cfg agent.Config, req agent.Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
			if req.Timeout != 0 {
				t.Fatalf("req.Timeout = %s, want 0", req.Timeout)
			}
			if req.DeveloperDir != "/Applications/Xcode-beta.app/Contents/Developer" {
				t.Fatalf("DeveloperDir = %q, want inherited DEVELOPER_DIR", req.DeveloperDir)
			}
			if toolName != "BuildProject" || arguments["tabIdentifier"] != "demo" {
				t.Fatalf("unexpected call args: tool=%q arguments=%+v", toolName, arguments)
			}
			return mcp.CallResult{Result: map[string]any{"echoName": toolName}}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		env := append(os.Environ(), "DEVELOPER_DIR=/Applications/Xcode-beta.app/Contents/Developer")
		code := run(context.Background(), []string{"serve", "--debug"}, strings.NewReader(""), &stdout, &stderr, env)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
	})
}

func TestRunServeReportsServerError(t *testing.T) {
	withStubs(t, func() {
		defaultMCPServeFunc = func(ctx context.Context, cfg mcp.ServerConfig, handler mcp.ServerHandler) error {
			return errors.New("serve failed")
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"serve"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "serve failed") {
			t.Fatalf("stderr = %q, want serve error", stderr.String())
		}
	})
}

func TestRunToolsListJSON(t *testing.T) {
	withStubs(t, func() {
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

func TestRunToolInspectText(t *testing.T) {
	withStubs(t, func() {
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			return []map[string]any{{
				"name":        "BuildProject",
				"description": "Build the active Xcode project",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"scheme": map[string]any{"type": "string"}}},
			}}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"tool", "inspect", "BuildProject"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		text := stdout.String()
		for _, want := range []string{"name: BuildProject", "description: Build the active Xcode project", "inputSchema:", `"scheme"`} {
			if !strings.Contains(text, want) {
				t.Fatalf("inspect output missing %q: %s", want, text)
			}
		}
	})
}

func TestRunToolInspectJSON(t *testing.T) {
	withStubs(t, func() {
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			return []map[string]any{{"name": "BuildProject", "description": "Build"}}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"tool", "inspect", "BuildProject", "--json"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		var tool map[string]any
		if err := json.Unmarshal([]byte(stdout.String()), &tool); err != nil {
			t.Fatalf("stdout is not JSON tool object: %v", err)
		}
		if tool["name"] != "BuildProject" {
			t.Fatalf("unexpected tool object: %+v", tool)
		}
	})
}

func TestRunToolInspectMissingTool(t *testing.T) {
	withStubs(t, func() {
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			return []map[string]any{{"name": "OtherTool"}}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"tool", "inspect", "BuildProject"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "tool not found") {
			t.Fatalf("stderr = %q, want tool not found error", stderr.String())
		}
	})
}

func TestRunToolsListGeneratesPersistentSessionID(t *testing.T) {
	withStubs(t, func() {
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
	withStubs(t, func() {
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

func TestRunToolsListUsesRequestTimeoutContext(t *testing.T) {
	withStubs(t, func() {
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("tools list context did not include a deadline")
			}
			if remaining := time.Until(deadline); remaining <= 0 || remaining > time.Second {
				t.Fatalf("unexpected tools list deadline window: %s", remaining)
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"tools", "list", "--json", "--timeout", "50ms"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "context deadline exceeded") {
			t.Fatalf("stderr = %q, want timeout error", stderr.String())
		}
	})
}

func TestRunToolInspectUsesTimeoutContext(t *testing.T) {
	withStubs(t, func() {
		parentCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("tool inspect context did not include a deadline")
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(parentCtx, []string{"tool", "inspect", "BuildProject"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "context deadline exceeded") {
			t.Fatalf("stderr = %q, want timeout error", stderr.String())
		}
	})
}

func TestRunToolCallIsErrorExitsOne(t *testing.T) {
	withStubs(t, func() {
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

func TestRunToolCallFromFile(t *testing.T) {
	withStubs(t, func() {
		payloadPath := filepath.Join(t.TempDir(), "payload.json")
		if err := os.WriteFile(payloadPath, []byte(`{"scheme":"Demo"}`), 0o600); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		defaultToolCallFunc = func(ctx context.Context, cfg agent.Config, req agent.Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
			if arguments["scheme"] != "Demo" {
				t.Fatalf("arguments = %+v, want scheme Demo", arguments)
			}
			return mcp.CallResult{Result: map[string]any{"ok": true}}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"tool", "call", "build_sim", "--json", "@" + payloadPath}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
	})
}

func TestRunToolCallFromStdin(t *testing.T) {
	withStubs(t, func() {
		defaultToolCallFunc = func(ctx context.Context, cfg agent.Config, req agent.Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
			if arguments["scheme"] != "Demo" {
				t.Fatalf("arguments = %+v, want scheme Demo", arguments)
			}
			return mcp.CallResult{Result: map[string]any{"ok": true}}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"tool", "call", "build_sim", "--json-stdin"}, strings.NewReader(`{"scheme":"Demo"}`), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
	})
}

func TestRunToolCallUsesRequestTimeoutContext(t *testing.T) {
	withStubs(t, func() {
		defaultToolCallFunc = func(ctx context.Context, cfg agent.Config, req agent.Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("tool call context did not include a deadline")
			}
			if remaining := time.Until(deadline); remaining <= 0 || remaining > time.Second {
				t.Fatalf("unexpected tool call deadline window: %s", remaining)
			}
			<-ctx.Done()
			return mcp.CallResult{}, ctx.Err()
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"tool", "call", "build_sim", "--json", `{"scheme":"Demo"}`, "--timeout", "50ms"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "context deadline exceeded") {
			t.Fatalf("stderr = %q, want timeout error", stderr.String())
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
	if !strings.Contains(stderr.String(), "JSON payload must decode to a JSON object") {
		t.Fatalf("stderr = %q, want JSON object error", stderr.String())
	}
}

func TestRunAgentStatusText(t *testing.T) {
	withStubs(t, func() {
		defaultAgentStatusFunc = func(ctx context.Context, cfg agent.Config) (agent.Status, error) {
			return agent.Status{
				Label:             agent.LaunchAgentLabel,
				PlistInstalled:    true,
				PlistPath:         "/tmp/io.oozoofrog.xcodecli.plist",
				RegisteredBinary:  "/tmp/xcodecli",
				CurrentBinary:     "/tmp/xcodecli",
				BinaryPathMatches: true,
				SocketPath:        "/tmp/daemon.sock",
				SocketReachable:   true,
				Running:           true,
				PID:               123,
				IdleTimeout:       24 * time.Hour,
				BackendSessions:   2,
			}, nil
		}
		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"agent", "status"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		for _, want := range []string{"backend sessions: 2", "mcpbridge session idle timeout: 24h"} {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("stdout = %q, want status output containing %q", stdout.String(), want)
			}
		}
	})
}

func TestRunAgentStatusJSON(t *testing.T) {
	withStubs(t, func() {
		defaultAgentStatusFunc = func(ctx context.Context, cfg agent.Config) (agent.Status, error) {
			return agent.Status{Label: agent.LaunchAgentLabel, Running: true}, nil
		}
		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"agent", "status", "--json"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		var status agent.Status
		if err := json.Unmarshal([]byte(stdout.String()), &status); err != nil {
			t.Fatalf("stdout is not JSON status: %v", err)
		}
		if status.Label != agent.LaunchAgentLabel || !status.Running {
			t.Fatalf("unexpected status: %+v", status)
		}
	})
}

func TestRunAgentCommandsDoNotCreatePersistentSession(t *testing.T) {
	withStubs(t, func() {
		oldSessionPathFunc := defaultSessionPathFunc
		sessionPath := filepath.Join(t.TempDir(), "session-id")
		defaultSessionPathFunc = func() (string, error) { return sessionPath, nil }
		defer func() { defaultSessionPathFunc = oldSessionPathFunc }()

		cases := [][]string{
			{"agent", "status"},
			{"agent", "status", "--json"},
			{"agent", "stop"},
			{"agent", "uninstall"},
			{"agent", "run", "--launch-agent"},
		}
		for _, args := range cases {
			var stdout strings.Builder
			var stderr strings.Builder
			if code := run(context.Background(), args, strings.NewReader(""), &stdout, &stderr, os.Environ()); code != 0 {
				t.Fatalf("run(%v) exit code = %d, stderr=%q", args, code, stderr.String())
			}
			if _, err := os.Stat(sessionPath); !os.IsNotExist(err) {
				t.Fatalf("run(%v) created persistent session file: %v", args, err)
			}
		}
	})
}

func withStubs(t *testing.T, fn func()) {
	t.Helper()
	oldConfig := defaultAgentConfigFunc
	oldList := defaultToolsListFunc
	oldCall := defaultToolCallFunc
	oldExecutablePath := defaultExecutablePathFunc
	oldMCPRunner := defaultMCPCommandRunner
	oldServe := defaultMCPServeFunc
	oldArgv0 := defaultArgv0Func
	oldGetwd := defaultGetwdFunc
	oldLookPath := defaultLookPathFunc
	oldOSExecutable := defaultOSExecutableFunc
	oldTempDir := defaultTempDirFunc
	oldStatus := defaultAgentStatusFunc
	oldStop := defaultAgentStopFunc
	oldUninstall := defaultAgentUninstallFunc
	oldRun := defaultAgentRunFunc
	oldDoctor := defaultDoctorRunFunc
	defaultAgentConfigFunc = func(command mcp.Command, env []string, errOut io.Writer) (agent.Config, error) {
		return agent.Config{}, nil
	}
	defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
		return nil, errors.New("unexpected tools list call")
	}
	defaultToolCallFunc = func(ctx context.Context, cfg agent.Config, req agent.Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
		return mcp.CallResult{}, errors.New("unexpected tool call")
	}
	defaultExecutablePathFunc = func() (string, error) {
		return "/tmp/xcodecli-test", nil
	}
	defaultMCPCommandRunner = func(ctx context.Context, name string, args []string) (externalCommandResult, error) {
		return externalCommandResult{}, errors.New("unexpected mcp config command")
	}
	defaultMCPServeFunc = func(ctx context.Context, cfg mcp.ServerConfig, handler mcp.ServerHandler) error {
		return nil
	}
	defaultArgv0Func = func() string { return "/tmp/xcodecli-test" }
	defaultGetwdFunc = func() (string, error) { return "/tmp", nil }
	defaultLookPathFunc = func(file string) (string, error) { return "", errors.New("unexpected lookpath") }
	defaultOSExecutableFunc = func() (string, error) { return "/tmp/xcodecli-test", nil }
	defaultTempDirFunc = func() string { return "/tmp" }
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
		defaultExecutablePathFunc = oldExecutablePath
		defaultMCPCommandRunner = oldMCPRunner
		defaultMCPServeFunc = oldServe
		defaultArgv0Func = oldArgv0
		defaultGetwdFunc = oldGetwd
		defaultLookPathFunc = oldLookPath
		defaultOSExecutableFunc = oldOSExecutable
		defaultTempDirFunc = oldTempDir
		defaultAgentStatusFunc = oldStatus
		defaultAgentStopFunc = oldStop
		defaultAgentUninstallFunc = oldUninstall
		defaultAgentRunFunc = oldRun
		defaultDoctorRunFunc = oldDoctor
	}()
	fn()
}

func withVersionState(t *testing.T, version string, buildChannel string, fn func()) {
	t.Helper()
	oldVersion := cliVersion
	oldBuildChannel := cliBuildChannel
	cliVersion = version
	cliBuildChannel = buildChannel
	defer func() {
		cliVersion = oldVersion
		cliBuildChannel = oldBuildChannel
	}()
	fn()
}
