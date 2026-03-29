package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/oozoofrog/xcodecli/internal/agent"
	"github.com/oozoofrog/xcodecli/internal/doctor"
	"github.com/oozoofrog/xcodecli/internal/mcp"
)

func TestParseCLIAgentDemo(t *testing.T) {
	cfg, _, err := parseCLI([]string{"agent", "demo", "--json", "--timeout", "45s", "--xcode-pid", "123", "--debug"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandAgentDemo {
		t.Fatalf("command = %q, want %q", cfg.Command, commandAgentDemo)
	}
	if !cfg.JSONOutput || cfg.Timeout.String() != "45s" || cfg.XcodePID != "123" || !cfg.Debug {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestParseCLIAgentDemoDefaultTimeout(t *testing.T) {
	cfg, _, err := parseCLI([]string{"agent", "demo"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Timeout != defaultAgentDemoRequestTimeout {
		t.Fatalf("timeout = %s, want %s", cfg.Timeout, defaultAgentDemoRequestTimeout)
	}
}

func TestParseCLIHelpAgentDemo(t *testing.T) {
	_, usage, err := parseCLI([]string{"help", "agent", "demo"})
	if err != errUsageRequested {
		t.Fatalf("err = %v, want errUsageRequested", err)
	}
	if !strings.Contains(usage, "agent demo") {
		t.Fatalf("usage missing agent demo help: %q", usage)
	}
}

func TestRootUsageIncludesAgentDemo(t *testing.T) {
	usage := rootUsage()
	for _, want := range []string{"xcodecli agent demo", "xcodecli tools list", "xcodecli tool call XcodeListWindows --json '{}'"} {
		if !strings.Contains(usage, want) {
			t.Fatalf("root usage missing %q: %s", want, usage)
		}
	}
}

func TestAgentUsageIncludesDemo(t *testing.T) {
	usage := agentUsage()
	for _, want := range []string{"xcodecli agent demo", "demo         Run a safe read-only onboarding demo"} {
		if !strings.Contains(usage, want) {
			t.Fatalf("agent usage missing %q: %s", want, usage)
		}
	}
}

func TestRunAgentDemoText(t *testing.T) {
	withStubs(t, func() {
		defaultDoctorRunFunc = func(ctx context.Context, opts doctor.Options) doctor.Report {
			return doctor.Report{Checks: []doctor.Check{{Name: "running Xcode processes", Status: doctor.StatusWarn, Detail: "no Xcode.app process detected"}}}
		}

		agentStatusCalls := 0
		defaultAgentStatusFunc = func(ctx context.Context, cfg agent.Config) (agent.Status, error) {
			agentStatusCalls++
			return agent.Status{Label: agent.LaunchAgentLabel, Running: true, SocketReachable: true, BackendSessions: agentStatusCalls}, nil
		}
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			return demoToolFixtures(), nil
		}
		defaultToolCallFunc = func(ctx context.Context, cfg agent.Config, req agent.Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
			if toolName != demoWindowsToolName {
				t.Fatalf("toolName = %q, want %q", toolName, demoWindowsToolName)
			}
			if len(arguments) != 0 {
				t.Fatalf("arguments = %+v, want empty object", arguments)
			}
			return mcp.CallResult{
				Result: map[string]any{
					"structuredContent": map[string]any{"message": "* tabIdentifier: windowtab1, workspacePath: /tmp/Demo.xcodeproj\n"},
					"content":           []map[string]any{{"type": "text", "text": "* tabIdentifier: windowtab1, workspacePath: /tmp/Demo.xcodeproj\n"}},
				},
			}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"agent", "demo"}, strings.NewReader(""), &stdout, &stderr, []string{})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		text := stdout.String()
		for _, want := range []string{
			"Environment",
			"recommendations:",
			"Open Xcode with the target workspace visible",
			"Tool Catalog",
			"Safe Live Demo",
			"Next Commands",
			"XcodeListWindows",
			"XcodeLS",
			"XcodeRead",
			"xcodecli tool inspect XcodeRead --json",
			"* tabIdentifier: windowtab1, workspacePath: /tmp/Demo.xcodeproj",
			"launchagent after tools discovery: running=true socketReachable=true backendSessions=2",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("agent demo output missing %q: %s", want, text)
			}
		}
	})
}

func TestFormatAgentDemoIncludesRecommendations(t *testing.T) {
	report := agentDemoReport{
		Doctor: doctor.Report{Checks: []doctor.Check{
			{Name: "running Xcode processes", Status: doctor.StatusWarn, Detail: "no Xcode.app process detected"},
		}}.JSON(),
		ToolCatalog: demoToolCatalog{},
		WindowsDemo: demoWindowsResult{ToolName: demoWindowsToolName},
	}

	text := formatAgentDemo(report)
	for _, want := range []string{"recommendations:", "Open Xcode with the target workspace visible"} {
		if !strings.Contains(text, want) {
			t.Fatalf("demo output missing %q: %s", want, text)
		}
	}
}

func TestRunAgentDemoJSON(t *testing.T) {
	withStubs(t, func() {
		defaultDoctorRunFunc = func(ctx context.Context, opts doctor.Options) doctor.Report {
			return doctor.Report{Checks: []doctor.Check{{Name: "doctor ok", Status: doctor.StatusOK, Detail: "ok"}}}
		}
		defaultAgentStatusFunc = func(ctx context.Context, cfg agent.Config) (agent.Status, error) {
			return agent.Status{Label: agent.LaunchAgentLabel, Running: true, SocketReachable: true, BackendSessions: 1}, nil
		}
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			return demoToolFixtures(), nil
		}
		defaultToolCallFunc = func(ctx context.Context, cfg agent.Config, req agent.Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
			return mcp.CallResult{Result: map[string]any{"structuredContent": map[string]any{"message": "ok"}}}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"agent", "demo", "--json"}, strings.NewReader(""), &stdout, &stderr, []string{})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}

		var report agentDemoReport
		if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil {
			t.Fatalf("stdout is not JSON report: %v (stdout=%q)", err, stdout.String())
		}
		if !report.Success {
			t.Fatalf("report.Success = false, want true")
		}
		if report.AgentStatus == nil || !report.AgentStatus.Running {
			t.Fatalf("unexpected agent status: %+v", report.AgentStatus)
		}
		if report.ToolCatalog.Count != len(demoToolFixtures()) {
			t.Fatalf("tool count = %d, want %d", report.ToolCatalog.Count, len(demoToolFixtures()))
		}
		if len(report.ToolCatalog.Highlights) != 5 {
			t.Fatalf("len(highlights) = %d, want 5", len(report.ToolCatalog.Highlights))
		}
		if !report.WindowsDemo.Attempted || !report.WindowsDemo.Ok {
			t.Fatalf("unexpected windows demo: %+v", report.WindowsDemo)
		}
		if len(report.NextCommands) != 3 {
			t.Fatalf("len(nextCommands) = %d, want 3", len(report.NextCommands))
		}
		if len(report.Errors) != 0 {
			t.Fatalf("errors = %+v, want empty", report.Errors)
		}
	})
}

func TestRunAgentDemoToolsListFailure(t *testing.T) {
	withStubs(t, func() {
		defaultDoctorRunFunc = func(ctx context.Context, opts doctor.Options) doctor.Report {
			return doctor.Report{Checks: []doctor.Check{{Name: "doctor ok", Status: doctor.StatusOK, Detail: "ok"}}}
		}
		defaultAgentStatusFunc = func(ctx context.Context, cfg agent.Config) (agent.Status, error) {
			return agent.Status{Label: agent.LaunchAgentLabel, Running: true}, nil
		}
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			return nil, errors.New("catalog unavailable")
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"agent", "demo", "--json"}, strings.NewReader(""), &stdout, &stderr, []string{})
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		var report agentDemoReport
		if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil {
			t.Fatalf("stdout is not JSON report: %v", err)
		}
		if report.ToolCatalog.Count != 0 {
			t.Fatalf("tool count = %d, want 0", report.ToolCatalog.Count)
		}
		if report.WindowsDemo.Attempted {
			t.Fatalf("windows demo attempted = true, want false")
		}
		if findDemoError(report.Errors, "tools list") == nil {
			t.Fatalf("expected tools list error in %+v", report.Errors)
		}
	})
}

func TestRunAgentDemoMissingWindowsTool(t *testing.T) {
	withStubs(t, func() {
		defaultDoctorRunFunc = func(ctx context.Context, opts doctor.Options) doctor.Report {
			return doctor.Report{Checks: []doctor.Check{{Name: "doctor ok", Status: doctor.StatusOK, Detail: "ok"}}}
		}
		defaultAgentStatusFunc = func(ctx context.Context, cfg agent.Config) (agent.Status, error) {
			return agent.Status{Label: agent.LaunchAgentLabel, Running: true}, nil
		}
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			return []map[string]any{{"name": "XcodeLS", "description": "List files", "inputSchema": map[string]any{"required": []any{"tabIdentifier", "path"}}}}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"agent", "demo", "--json"}, strings.NewReader(""), &stdout, &stderr, []string{})
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		var report agentDemoReport
		if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil {
			t.Fatalf("stdout is not JSON report: %v", err)
		}
		if report.WindowsDemo.Attempted {
			t.Fatalf("windows demo attempted = true, want false")
		}
		if report.WindowsDemo.Error == nil || !strings.Contains(report.WindowsDemo.Error.Message, "tool not found") {
			t.Fatalf("unexpected windows demo error: %+v", report.WindowsDemo.Error)
		}
	})
}

func TestRunAgentDemoWindowsToolFailure(t *testing.T) {
	withStubs(t, func() {
		defaultDoctorRunFunc = func(ctx context.Context, opts doctor.Options) doctor.Report {
			return doctor.Report{Checks: []doctor.Check{{Name: "doctor ok", Status: doctor.StatusOK, Detail: "ok"}}}
		}
		defaultAgentStatusFunc = func(ctx context.Context, cfg agent.Config) (agent.Status, error) {
			return agent.Status{Label: agent.LaunchAgentLabel, Running: true}, nil
		}
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			return demoToolFixtures(), nil
		}
		defaultToolCallFunc = func(ctx context.Context, cfg agent.Config, req agent.Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
			return mcp.CallResult{}, errors.New("window lookup failed")
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"agent", "demo", "--json"}, strings.NewReader(""), &stdout, &stderr, []string{})
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		var report agentDemoReport
		if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil {
			t.Fatalf("stdout is not JSON report: %v", err)
		}
		if !report.WindowsDemo.Attempted || report.WindowsDemo.Ok {
			t.Fatalf("unexpected windows demo state: %+v", report.WindowsDemo)
		}
		if report.WindowsDemo.Error == nil || report.WindowsDemo.Error.Message != "window lookup failed" {
			t.Fatalf("unexpected windows demo error: %+v", report.WindowsDemo.Error)
		}
	})
}

func demoToolFixtures() []map[string]any {
	return []map[string]any{
		{
			"name":        "XcodeListWindows",
			"description": "List windows",
			"inputSchema": map[string]any{"type": "object", "required": []any{}},
		},
		{
			"name":        "XcodeLS",
			"description": "List files",
			"inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier", "path"}},
		},
		{
			"name":        "XcodeRead",
			"description": "Read files",
			"inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier", "filePath"}},
		},
		{
			"name":        "BuildProject",
			"description": "Build project",
			"inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier"}},
		},
		{
			"name":        "RunAllTests",
			"description": "Run tests",
			"inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier"}},
		},
	}
}
