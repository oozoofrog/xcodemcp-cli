package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/oozoofrog/xcodemcp-cli/internal/agent"
	"github.com/oozoofrog/xcodemcp-cli/internal/doctor"
	"github.com/oozoofrog/xcodemcp-cli/internal/mcp"
)

func TestParseCLIAgentGuide(t *testing.T) {
	cfg, _, err := parseCLI([]string{"agent", "guide", "build Unicody", "--json", "--timeout", "45s", "--xcode-pid", "123", "--debug"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandAgentGuide {
		t.Fatalf("command = %q, want %q", cfg.Command, commandAgentGuide)
	}
	if cfg.Intent != "build Unicody" || !cfg.JSONOutput || cfg.Timeout.String() != "45s" || cfg.XcodePID != "123" || !cfg.Debug {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestParseCLIHelpAgentGuide(t *testing.T) {
	_, usage, err := parseCLI([]string{"help", "agent", "guide"})
	if err != errUsageRequested {
		t.Fatalf("err = %v, want errUsageRequested", err)
	}
	if !strings.Contains(usage, "agent guide") {
		t.Fatalf("usage missing agent guide help: %q", usage)
	}
}

func TestRootAndAgentUsageIncludeGuide(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage string
		want  []string
	}{
		{"root", rootUsage(), []string{"xcodemcp agent guide", "xcodemcp agent demo"}},
		{"agent", agentUsage(), []string{"guide        Explain the recommended tool workflow for a request", "demo         Run a safe read-only onboarding demo"}},
	} {
		for _, want := range tc.want {
			if !strings.Contains(tc.usage, want) {
				t.Fatalf("%s usage missing %q: %s", tc.name, want, tc.usage)
			}
		}
	}
}

func TestClassifyGuideIntent(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"build Unicody", "build"},
		{"run tests for Unicody", "test"},
		{"read KeyboardState.swift", "read"},
		{"search AdManager", "search"},
		{"update KeyboardState.swift", "edit"},
		{"fix build error", "diagnose"},
	}

	for _, tc := range cases {
		got := classifyGuideIntent(tc.raw)
		if got.WorkflowID != tc.want {
			t.Fatalf("classifyGuideIntent(%q) = %q, want %q", tc.raw, got.WorkflowID, tc.want)
		}
	}
}

func TestRunAgentGuideNoIntentShowsCatalog(t *testing.T) {
	withStubs(t, func() {
		stubGuideEnvironment(t)

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"agent", "guide"}, strings.NewReader(""), &stdout, &stderr, []string{})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}

		text := stdout.String()
		for _, want := range []string{
			"Intent",
			"Environment",
			"Recommended Workflow",
			"Exact Next Commands",
			"Fallbacks",
			"build Unicody",
			"run tests for Unicody",
			"read KeyboardState.swift",
			"search for AdManager",
			"update KeyboardState.swift",
			"diagnose build errors",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("agent guide catalog output missing %q: %s", want, text)
			}
		}
	})
}

func TestRunAgentGuideBuildJSONUsesMatchedWindow(t *testing.T) {
	withStubs(t, func() {
		stubGuideEnvironment(t)

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"agent", "guide", "build Unicody", "--json"}, strings.NewReader(""), &stdout, &stderr, []string{})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}

		var report agentGuideReport
		if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil {
			t.Fatalf("stdout is not JSON report: %v (stdout=%q)", err, stdout.String())
		}
		if report.Intent.WorkflowID != "build" {
			t.Fatalf("workflowId = %q, want build", report.Intent.WorkflowID)
		}
		if report.Environment.Windows.Ok != true || len(report.Environment.Windows.Entries) != 2 {
			t.Fatalf("unexpected windows state: %+v", report.Environment.Windows)
		}
		if len(report.NextCommands) < 2 {
			t.Fatalf("nextCommands too short: %+v", report.NextCommands)
		}
		if !strings.Contains(report.NextCommands[0], `"tabIdentifier":"windowtab2"`) {
			t.Fatalf("first command = %q, want matched windowtab2", report.NextCommands[0])
		}
		if len(report.Errors) != 0 {
			t.Fatalf("unexpected errors: %+v", report.Errors)
		}
	})
}

func TestRunAgentGuideAmbiguousWindowKeepsPlaceholder(t *testing.T) {
	withStubs(t, func() {
		stubGuideDoctorAndStatus()
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			return guideToolFixtures(), nil
		}
		defaultToolCallFunc = func(ctx context.Context, cfg agent.Config, req agent.Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
			if toolName != demoWindowsToolName {
				t.Fatalf("guide unexpectedly called mutating or unrelated tool %q", toolName)
			}
			return mcp.CallResult{
				Result: map[string]any{
					"structuredContent": map[string]any{"message": "* tabIdentifier: windowtabA, workspacePath: /tmp/Unicody.xcodeproj\n* tabIdentifier: windowtabB, workspacePath: /tmp/Unicody.xcworkspace\n"},
				},
			}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"agent", "guide", "build Unicody", "--json"}, strings.NewReader(""), &stdout, &stderr, []string{})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}

		var report agentGuideReport
		if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil {
			t.Fatalf("stdout is not JSON report: %v", err)
		}
		if !strings.Contains(report.Workflow.Reason, "ambiguous") {
			t.Fatalf("workflow reason should mention ambiguity: %q", report.Workflow.Reason)
		}
		if !strings.Contains(report.NextCommands[0], "XcodeListWindows") {
			t.Fatalf("expected first command to re-check windows, got %q", report.NextCommands[0])
		}
		if !strings.Contains(report.NextCommands[1], `\u003ctabIdentifier from XcodeListWindows\u003e`) {
			t.Fatalf("expected placeholder tabIdentifier, got %q", report.NextCommands[1])
		}
	})
}

func TestRunAgentGuideNoWindowMatchKeepsPlaceholder(t *testing.T) {
	withStubs(t, func() {
		stubGuideEnvironment(t)

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"agent", "guide", "build OtherApp", "--json"}, strings.NewReader(""), &stdout, &stderr, []string{})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}

		var report agentGuideReport
		if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil {
			t.Fatalf("stdout is not JSON report: %v", err)
		}
		if !strings.Contains(report.Workflow.Reason, "placeholder") {
			t.Fatalf("workflow reason should mention placeholder guidance: %q", report.Workflow.Reason)
		}
		if !strings.Contains(report.NextCommands[0], "XcodeListWindows") {
			t.Fatalf("expected first command to be XcodeListWindows, got %q", report.NextCommands[0])
		}
	})
}

func TestRunAgentGuideReadOnlySafety(t *testing.T) {
	withStubs(t, func() {
		stubGuideDoctorAndStatus()
		defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
			return guideToolFixtures(), nil
		}
		defaultToolCallFunc = func(ctx context.Context, cfg agent.Config, req agent.Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
			if toolName != demoWindowsToolName {
				t.Fatalf("agent guide should only call %q live, got %q", demoWindowsToolName, toolName)
			}
			return mcp.CallResult{
				Result: map[string]any{
					"structuredContent": map[string]any{"message": "* tabIdentifier: windowtab2, workspacePath: /tmp/Unicody.xcodeproj\n"},
				},
			}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"agent", "guide", "update KeyboardState.swift"}, strings.NewReader(""), &stdout, &stderr, []string{})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		text := stdout.String()
		for _, want := range []string{"XcodeUpdate", "XcodeRefreshCodeIssuesInFile", "XcodeWrite"} {
			if !strings.Contains(text, want) {
				t.Fatalf("expected guide output to mention %q: %s", want, text)
			}
		}
	})
}

func stubGuideEnvironment(t *testing.T) {
	t.Helper()
	stubGuideDoctorAndStatus()
	defaultToolsListFunc = func(ctx context.Context, cfg agent.Config, req agent.Request) ([]map[string]any, error) {
		return guideToolFixtures(), nil
	}
	defaultToolCallFunc = func(ctx context.Context, cfg agent.Config, req agent.Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
		if toolName != demoWindowsToolName {
			t.Fatalf("guide unexpectedly called mutating or unrelated tool %q", toolName)
		}
		return mcp.CallResult{
			Result: map[string]any{
				"structuredContent": map[string]any{"message": "* tabIdentifier: windowtab1, workspacePath: /tmp/Talk.xcworkspace\n* tabIdentifier: windowtab2, workspacePath: /tmp/Unicody.xcodeproj\n"},
				"content":           []map[string]any{{"type": "text", "text": "* tabIdentifier: windowtab1, workspacePath: /tmp/Talk.xcworkspace\n* tabIdentifier: windowtab2, workspacePath: /tmp/Unicody.xcodeproj\n"}},
			},
		}, nil
	}
}

func stubGuideDoctorAndStatus() {
	defaultDoctorRunFunc = func(ctx context.Context, opts doctor.Options) doctor.Report {
		return doctor.Report{Checks: []doctor.Check{{Name: "doctor ok", Status: doctor.StatusOK, Detail: "ok"}}}
	}
	defaultAgentStatusFunc = func(ctx context.Context, cfg agent.Config) (agent.Status, error) {
		return agent.Status{Label: agent.LaunchAgentLabel, Running: true, SocketReachable: true, BackendSessions: 1}, nil
	}
}

func guideToolFixtures() []map[string]any {
	return []map[string]any{
		{"name": "XcodeListWindows", "description": "List windows", "inputSchema": map[string]any{"type": "object", "required": []any{}}},
		{"name": "BuildProject", "description": "Build project", "inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier"}}},
		{"name": "GetBuildLog", "description": "Build log", "inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier"}}},
		{"name": "RunAllTests", "description": "Run all tests", "inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier"}}},
		{"name": "GetTestList", "description": "List tests", "inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier"}}},
		{"name": "RunSomeTests", "description": "Run some tests", "inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier", "tests"}}},
		{"name": "XcodeLS", "description": "List files", "inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier", "path"}}},
		{"name": "XcodeRead", "description": "Read files", "inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier", "filePath"}}},
		{"name": "XcodeGlob", "description": "Search files", "inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier"}}},
		{"name": "XcodeGrep", "description": "Search content", "inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier", "pattern"}}},
		{"name": "XcodeUpdate", "description": "Update file text", "inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier", "filePath", "oldString", "newString"}}},
		{"name": "XcodeWrite", "description": "Write file", "inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier", "filePath", "content"}}},
		{"name": "XcodeRefreshCodeIssuesInFile", "description": "Refresh issues", "inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier", "filePath"}}},
		{"name": "XcodeListNavigatorIssues", "description": "Navigator issues", "inputSchema": map[string]any{"type": "object", "required": []any{"tabIdentifier"}}},
	}
}
