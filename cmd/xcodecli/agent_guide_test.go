package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/oozoofrog/xcodecli/internal/agent"
	"github.com/oozoofrog/xcodecli/internal/doctor"
	"github.com/oozoofrog/xcodecli/internal/mcp"
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

func TestParseCLIAgentGuideDefaultTimeout(t *testing.T) {
	cfg, _, err := parseCLI([]string{"agent", "guide", "build Unicody"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Timeout != defaultAgentGuideRequestTimeout {
		t.Fatalf("timeout = %s, want %s", cfg.Timeout, defaultAgentGuideRequestTimeout)
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
		{"root", rootUsage(), []string{"xcodecli agent guide", "xcodecli agent demo"}},
		{"agent", agentUsage(), []string{"guide        Explain the recommended tool workflow for a request", "demo         Run a safe read-only onboarding demo"}},
	} {
		for _, want := range tc.want {
			if !strings.Contains(tc.usage, want) {
				t.Fatalf("%s usage missing %q: %s", tc.name, want, tc.usage)
			}
		}
	}
}

func TestGuideNextCommandsUseUpdatedTimeouts(t *testing.T) {
	if got := formatBuildProjectCommand("tab1"); !strings.Contains(got, "--timeout 30m") {
		t.Fatalf("BuildProject command = %q, want --timeout 30m", got)
	}
	if got := formatRunAllTestsCommand("tab1"); !strings.Contains(got, "--timeout 30m") {
		t.Fatalf("RunAllTests command = %q, want --timeout 30m", got)
	}
	if got := formatRunSomeTestsTemplate("tab1"); !strings.Contains(got, "--timeout 30m") {
		t.Fatalf("RunSomeTests command = %q, want --timeout 30m", got)
	}
	if got := formatXcodeUpdateTemplate("tab1", "Foo.swift"); !strings.Contains(got, "--timeout 120s") {
		t.Fatalf("XcodeUpdate command = %q, want --timeout 120s", got)
	}
	if got := formatRefreshIssuesCommand("tab1", "Foo.swift"); !strings.Contains(got, "--timeout 120s") {
		t.Fatalf("Refresh issues command = %q, want --timeout 120s", got)
	}
	if got := formatXcodeWriteTemplate("tab1", "Foo.swift"); !strings.Contains(got, "--timeout 120s") {
		t.Fatalf("XcodeWrite command = %q, want --timeout 120s", got)
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

func TestFormatAgentGuideIncludesRecommendations(t *testing.T) {
	report := agentGuideReport{
		Intent: guideIntentResult{Raw: "build Unicody", WorkflowID: "build", Confidence: 0.75},
		Environment: guideEnvironment{
			Doctor: doctor.Report{Checks: []doctor.Check{
				{Name: "running Xcode processes", Status: doctor.StatusWarn, Detail: "no Xcode.app process detected"},
			}}.JSON(),
			ToolCatalog: demoToolCatalog{},
		},
		Workflow: guideWorkflowResult{
			ID:     "build",
			Title:  "Build a project",
			Reason: "The request is about building.",
			Steps:  []guideWorkflowStep{{ToolName: "XcodeListWindows", Why: "Find the right window."}},
		},
	}

	text := formatAgentGuide(report, guideWindowMatch{})
	for _, want := range []string{"recommendations:", "Open Xcode with the target workspace visible"} {
		if !strings.Contains(text, want) {
			t.Fatalf("guide output missing %q: %s", want, text)
		}
	}
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

func TestGuideWorkflowBuildersAndCommandHelpers(t *testing.T) {
	intentTest := guideIntentMatch{WorkflowID: "test", Subject: "Unicody"}
	matched := guideWindowMatch{MatchedEntry: &guideWindowEntry{TabIdentifier: "tab-1"}}
	workflowTest, nextTest := buildGuideTestWorkflow(intentTest, "tab-1", matched)
	if workflowTest.ID != "test" {
		t.Fatalf("test workflow id = %q, want test", workflowTest.ID)
	}
	if len(nextTest) != 2 {
		t.Fatalf("len(test next commands) = %d, want 2 without XcodeListWindows", len(nextTest))
	}
	if !strings.Contains(nextTest[0], "RunAllTests") || !strings.Contains(nextTest[1], "GetBuildLog") {
		t.Fatalf("unexpected test commands: %+v", nextTest)
	}
	if !strings.Contains(workflowTest.Fallbacks[0].Commands[0], "GetTestList") {
		t.Fatalf("expected GetTestList fallback, got %+v", workflowTest.Fallbacks)
	}

	intentReadFile := guideIntentMatch{WorkflowID: "read", Subject: "KeyboardState.swift"}
	workflowReadFile, nextReadFile := buildGuideReadWorkflow(intentReadFile, "tab-2", guideWindowMatch{})
	if workflowReadFile.ID != "read" {
		t.Fatalf("read workflow id = %q, want read", workflowReadFile.ID)
	}
	if !strings.Contains(nextReadFile[0], "XcodeListWindows") || !strings.Contains(nextReadFile[1], "XcodeGlob") || !strings.Contains(nextReadFile[2], "XcodeRead") {
		t.Fatalf("unexpected read(file hint) commands: %+v", nextReadFile)
	}

	intentReadBrowse := guideIntentMatch{WorkflowID: "read", Subject: "settings"}
	_, nextReadBrowse := buildGuideReadWorkflow(intentReadBrowse, "tab-3", guideWindowMatch{})
	if !strings.Contains(nextReadBrowse[1], "XcodeLS") {
		t.Fatalf("expected XcodeLS for non-file read hint, got %+v", nextReadBrowse)
	}

	intentSearchText := guideIntentMatch{WorkflowID: "search", Subject: "AdManager"}
	workflowSearchText, nextSearchText := buildGuideSearchWorkflow(intentSearchText, "tab-4", guideWindowMatch{})
	if workflowSearchText.ID != "search" {
		t.Fatalf("search workflow id = %q, want search", workflowSearchText.ID)
	}
	if !strings.Contains(nextSearchText[len(nextSearchText)-1], "XcodeGrep") {
		t.Fatalf("expected XcodeGrep search command, got %+v", nextSearchText)
	}

	intentSearchFile := guideIntentMatch{WorkflowID: "search", Subject: "KeyboardState.swift"}
	_, nextSearchFile := buildGuideSearchWorkflow(intentSearchFile, "tab-5", guideWindowMatch{})
	if !strings.Contains(nextSearchFile[len(nextSearchFile)-1], "XcodeGlob") {
		t.Fatalf("expected XcodeGlob file search command, got %+v", nextSearchFile)
	}

	intentDiagnose := guideIntentMatch{WorkflowID: "diagnose", Subject: "build errors"}
	workflowDiagnose, nextDiagnose := buildGuideDiagnoseWorkflow(intentDiagnose, "tab-6", guideWindowMatch{})
	if workflowDiagnose.ID != "diagnose" {
		t.Fatalf("diagnose workflow id = %q, want diagnose", workflowDiagnose.ID)
	}
	if !strings.Contains(nextDiagnose[0], "XcodeListWindows") || !strings.Contains(nextDiagnose[1], "GetBuildLog") || !strings.Contains(nextDiagnose[2], "XcodeRead") {
		t.Fatalf("unexpected diagnose commands: %+v", nextDiagnose)
	}

	if got := guideSearchPattern("  FooBar  "); got != "FooBar" {
		t.Fatalf("guideSearchPattern trimmed result = %q, want FooBar", got)
	}
	if got := guideSearchPattern(""); got != "<search pattern>" {
		t.Fatalf("guideSearchPattern empty = %q, want placeholder", got)
	}
	if got := formatGetTestListCommand("tab-1"); !strings.Contains(got, "GetTestList") || !strings.Contains(got, `"tabIdentifier":"tab-1"`) {
		t.Fatalf("unexpected GetTestList command: %q", got)
	}
	if got := formatXcodeLSCommand("tab-2", "Sources"); !strings.Contains(got, "XcodeLS") || !strings.Contains(got, `"path":"Sources"`) {
		t.Fatalf("unexpected XcodeLS command: %q", got)
	}
	if got := formatXcodeGrepCommand("tab-3", "Pattern"); !strings.Contains(got, "XcodeGrep") || !strings.Contains(got, `"pattern":"Pattern"`) {
		t.Fatalf("unexpected XcodeGrep command: %q", got)
	}
	if got := formatMaybeWindowsCommand(matched); got != "# already matched tab-1" {
		t.Fatalf("formatMaybeWindowsCommand matched = %q, want comment", got)
	}
	if got := formatMaybeWindowsCommand(guideWindowMatch{}); !strings.Contains(got, "XcodeListWindows") {
		t.Fatalf("formatMaybeWindowsCommand unmatched = %q, want windows command", got)
	}
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
