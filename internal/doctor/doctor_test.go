package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/oozoofrog/xcodecli/internal/agent"
	"github.com/oozoofrog/xcodecli/internal/bridge"
)

func TestReportString(t *testing.T) {
	report := Report{Checks: []Check{
		{Name: "one", Status: StatusOK, Detail: "ok detail"},
		{Name: "two", Status: StatusWarn, Detail: "warn detail"},
		{Name: "three", Status: StatusFail, Detail: "fail detail"},
		{Name: "four", Status: StatusInfo, Detail: "info detail"},
	}}
	text := report.String()
	for _, want := range []string{"[OK] one: ok detail", "[WARN] two: warn detail", "[FAIL] three: fail detail", "[INFO] four: info detail", "Summary: 1 ok, 1 warn, 1 fail, 1 info"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report output missing %q: %s", want, text)
		}
	}
	if report.Success() {
		t.Fatal("report should not be successful when it contains a fail status")
	}
}

func TestReportJSON(t *testing.T) {
	report := Report{Checks: []Check{
		{Name: "one", Status: StatusOK, Detail: "ok detail"},
		{Name: "two", Status: StatusWarn, Detail: "warn detail"},
		{Name: "three", Status: StatusFail, Detail: "fail detail"},
		{Name: "four", Status: StatusInfo, Detail: "info detail"},
	}}
	payload := report.JSON()
	if payload.Success {
		t.Fatalf("expected unsuccessful payload: %+v", payload)
	}
	if payload.Summary != (Summary{OK: 1, Warn: 1, Fail: 1, Info: 1}) {
		t.Fatalf("unexpected summary: %+v", payload.Summary)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if !strings.Contains(string(data), `"checks"`) {
		t.Fatalf("unexpected JSON payload: %s", string(data))
	}
	if !strings.Contains(string(data), `"recommendations"`) {
		t.Fatalf("unexpected JSON payload missing recommendations: %s", string(data))
	}
}

func TestInspectorRunSuccess(t *testing.T) {
	inspector := Inspector{
		LookPath: func(file string) (string, error) {
			if file != "xcrun" {
				t.Fatalf("unexpected lookPath arg: %s", file)
			}
			return "/usr/bin/xcrun", nil
		},
		RunCommand: func(ctx context.Context, req CommandRequest) (CommandResult, error) {
			switch req.Name {
			case "/usr/bin/xcrun":
				if len(req.Args) == 2 && req.Args[1] == "--help" {
					return CommandResult{Stdout: "help text"}, nil
				}
				if len(req.Args) == 1 && req.Args[0] == "mcpbridge" {
					if !containsEnv(req.Env, "MCP_XCODE_PID=101") {
						t.Fatalf("smoke env missing pid override: %+v", req.Env)
					}
					return CommandResult{}, nil
				}
			case "xcode-select":
				return CommandResult{Stdout: "/Applications/Xcode.app/Contents/Developer\n"}, nil
			}
			return CommandResult{}, errors.New("unexpected command")
		},
		ListProcesses: func(ctx context.Context) ([]Process, error) {
			return []Process{{PID: 101, Command: "/Applications/Xcode.app/Contents/MacOS/Xcode"}}, nil
		},
	}

	report := inspector.Run(context.Background(), Options{
		BaseEnv:   []string{"HOME=/tmp"},
		XcodePID:  "101",
		SessionID: "11111111-1111-1111-1111-111111111111",
	})
	if !report.Success() {
		t.Fatalf("expected success report, got: %s", report.String())
	}
	if !strings.Contains(report.String(), "Summary: 6 ok, 1 warn, 0 fail, 1 info") {
		t.Fatalf("unexpected summary: %s", report.String())
	}
}

func TestInspectorIncludesAgentStatusInfo(t *testing.T) {
	inspector := Inspector{
		LookPath: func(file string) (string, error) { return "/usr/bin/xcrun", nil },
		RunCommand: func(ctx context.Context, req CommandRequest) (CommandResult, error) {
			if req.Name == "xcode-select" {
				return CommandResult{Stdout: "/Applications/Xcode.app/Contents/Developer\n"}, nil
			}
			return CommandResult{Stdout: "help"}, nil
		},
		ListProcesses: func(ctx context.Context) ([]Process, error) {
			return []Process{{PID: 101, Command: "/Applications/Xcode.app/Contents/MacOS/Xcode"}}, nil
		},
	}

	report := inspector.Run(context.Background(), Options{
		AgentStatus: &agent.Status{
			PlistInstalled:    true,
			PlistPath:         "/tmp/io.oozoofrog.xcodecli.plist",
			SocketReachable:   true,
			SocketPath:        "/tmp/daemon.sock",
			RegisteredBinary:  "/tmp/xcodecli",
			CurrentBinary:     "/tmp/xcodecli",
			BinaryPathMatches: true,
		},
	})
	text := report.String()
	for _, want := range []string{"LaunchAgent plist", "LaunchAgent socket", "LaunchAgent binary registration"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report output missing %q: %s", want, text)
		}
	}
}

func TestReportRecommendationsIncludeLaunchAgentHelp(t *testing.T) {
	report := Report{Checks: []Check{
		{Name: "LaunchAgent binary registration", Status: StatusWarn, Detail: "registered=../Cellar/xcodecli | current=/opt/homebrew/bin/xcodecli | match=false | registered binary path is relative"},
	}}
	recommendations := report.Recommendations()
	if len(recommendations) == 0 {
		t.Fatal("expected recommendations for stale LaunchAgent registration")
	}
	if recommendations[0].ID != "launchagent-registration" {
		t.Fatalf("unexpected first recommendation: %+v", recommendations[0])
	}
	if !strings.Contains(report.String(), "Recommendations:") {
		t.Fatalf("doctor text should include recommendations section: %s", report.String())
	}
}

func TestInspectorSkipsSmokeWhenOverridesInvalid(t *testing.T) {
	calledSmoke := false
	inspector := Inspector{
		LookPath: func(file string) (string, error) { return "/usr/bin/xcrun", nil },
		RunCommand: func(ctx context.Context, req CommandRequest) (CommandResult, error) {
			if req.Name == "/usr/bin/xcrun" && len(req.Args) == 1 && req.Args[0] == "mcpbridge" {
				calledSmoke = true
			}
			if req.Name == "xcode-select" {
				return CommandResult{Stdout: "/Applications/Xcode.app/Contents/Developer\n"}, nil
			}
			return CommandResult{Stdout: "help"}, nil
		},
		ListProcesses: func(ctx context.Context) ([]Process, error) {
			return []Process{{PID: 101, Command: "/Applications/Xcode.app/Contents/MacOS/Xcode"}}, nil
		},
	}

	report := inspector.Run(context.Background(), Options{XcodePID: "0"})
	if report.Success() {
		t.Fatalf("expected failure report, got: %s", report.String())
	}
	if calledSmoke {
		t.Fatal("smoke test should be skipped when overrides are invalid")
	}
	if !strings.Contains(report.String(), "skipped because explicit overrides failed validation") {
		t.Fatalf("missing smoke skip message: %s", report.String())
	}
}

func TestFormatSessionDetail(t *testing.T) {
	detail := formatSessionDetail(Options{
		SessionID:     "11111111-1111-1111-1111-111111111111",
		SessionSource: bridge.SessionSourcePersisted,
		SessionPath:   "/tmp/session-id",
	})
	if detail != "11111111-1111-1111-1111-111111111111 (persisted at /tmp/session-id)" {
		t.Fatalf("unexpected detail: %q", detail)
	}
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
