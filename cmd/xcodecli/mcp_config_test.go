package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseCLIMCPConfigCodex(t *testing.T) {
	cfg, _, err := parseCLI([]string{"mcp", "config", "--client", "codex", "--json"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandMCPConfig {
		t.Fatalf("command = %q, want %q", cfg.Command, commandMCPConfig)
	}
	if cfg.MCPClient != "codex" || cfg.ConfigName != "xcodecli" || !cfg.JSONOutput || cfg.Scope != "" || cfg.MCPMode != "agent" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestParseCLIMCPAliasCodex(t *testing.T) {
	cfg, _, err := parseCLI([]string{"mcp", "codex", "--json"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandMCPConfig {
		t.Fatalf("command = %q, want %q", cfg.Command, commandMCPConfig)
	}
	if cfg.MCPClient != "codex" || !cfg.JSONOutput {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestParseCLIMCPAliasGeminiDefaultsScope(t *testing.T) {
	cfg, _, err := parseCLI([]string{"mcp", "gemini"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.MCPClient != "gemini" || cfg.Scope != "user" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestParseCLIMCPConfigClaudeDefaultsScope(t *testing.T) {
	cfg, _, err := parseCLI([]string{"mcp", "config", "--client", "claude"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Scope != "local" {
		t.Fatalf("scope = %q, want local", cfg.Scope)
	}
}

func TestParseCLIMCPConfigGeminiDefaultsScope(t *testing.T) {
	cfg, _, err := parseCLI([]string{"mcp", "config", "--client", "gemini"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Scope != "user" {
		t.Fatalf("scope = %q, want user", cfg.Scope)
	}
}

func TestParseCLIMCPConfigRejectsMissingClient(t *testing.T) {
	_, _, err := parseCLI([]string{"mcp", "config"})
	if err == nil || !strings.Contains(err.Error(), "requires --client") {
		t.Fatalf("expected missing client error, got %v", err)
	}
}

func TestParseCLIMCPConfigRejectsCodexScope(t *testing.T) {
	_, _, err := parseCLI([]string{"mcp", "config", "--client", "codex", "--scope", "user"})
	if err == nil || !strings.Contains(err.Error(), "not supported for client codex") {
		t.Fatalf("expected codex scope error, got %v", err)
	}
}

func TestParseCLIMCPConfigRejectsInvalidMode(t *testing.T) {
	_, _, err := parseCLI([]string{"mcp", "config", "--client", "codex", "--mode", "weird"})
	if err == nil || !strings.Contains(err.Error(), "--mode must be one of") {
		t.Fatalf("expected invalid mode error, got %v", err)
	}
}

func TestParseCLIHelpMCPConfig(t *testing.T) {
	_, usage, err := parseCLI([]string{"help", "mcp", "config"})
	if err != errUsageRequested {
		t.Fatalf("err = %v, want errUsageRequested", err)
	}
	for _, want := range []string{"mcp config", "--client <claude|codex|gemini>", "--mode <agent|bridge>", "--write"} {
		if !strings.Contains(usage, want) {
			t.Fatalf("usage missing %q: %s", want, usage)
		}
	}
}

func TestParseCLIHelpMCPCodex(t *testing.T) {
	_, usage, err := parseCLI([]string{"help", "mcp", "codex"})
	if err != errUsageRequested {
		t.Fatalf("err = %v, want errUsageRequested", err)
	}
	for _, want := range []string{"mcp codex", "shorthand alias", "--mode <agent|bridge>", "--write"} {
		if !strings.Contains(usage, want) {
			t.Fatalf("usage missing %q: %s", want, usage)
		}
	}
	if strings.Contains(usage, "--scope") {
		t.Fatalf("usage should not mention --scope: %s", usage)
	}
}

func TestParseCLIHelpMCPClaudeIncludesScope(t *testing.T) {
	_, usage, err := parseCLI([]string{"help", "mcp", "claude"})
	if err != errUsageRequested {
		t.Fatalf("err = %v, want errUsageRequested", err)
	}
	if !strings.Contains(usage, "--scope SCOPE") {
		t.Fatalf("usage missing scope: %s", usage)
	}
}

func TestParseCLIHelpMCPGeminiIncludesScope(t *testing.T) {
	_, usage, err := parseCLI([]string{"help", "mcp", "gemini"})
	if err != errUsageRequested {
		t.Fatalf("err = %v, want errUsageRequested", err)
	}
	if !strings.Contains(usage, "--scope SCOPE") {
		t.Fatalf("usage missing scope: %s", usage)
	}
}

func TestRunMCPConfigTextCodex(t *testing.T) {
	withStubs(t, func() {
		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{
			"mcp", "config",
			"--client", "codex",
			"--xcode-pid", "123",
			"--session-id", "11111111-1111-1111-1111-111111111111",
		}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		text := stdout.String()
		for _, want := range []string{
			"client: codex",
			"mode: agent",
			"name: xcodecli",
			"env:",
			"MCP_XCODE_PID=123",
			"MCP_XCODE_SESSION_ID=11111111-1111-1111-1111-111111111111",
			"codex mcp add xcodecli --env MCP_XCODE_PID=123 --env MCP_XCODE_SESSION_ID=11111111-1111-1111-1111-111111111111 -- /tmp/xcodecli-test serve",
			"write requested: false",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("output missing %q: %s", want, text)
			}
		}
		if stderr.String() != "" {
			t.Fatalf("stderr = %q, want empty stderr", stderr.String())
		}
	})
}

func TestRunMCPAliasTextCodex(t *testing.T) {
	withStubs(t, func() {
		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"mcp", "codex"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "client: codex") {
			t.Fatalf("unexpected output: %s", stdout.String())
		}
	})
}

func TestRunMCPConfigJSONWriteCodex(t *testing.T) {
	withStubs(t, func() {
		defaultMCPCommandRunner = func(ctx context.Context, name string, args []string) (externalCommandResult, error) {
			wantArgs := []string{"mcp", "add", "xcodecli", "--env", "MCP_XCODE_SESSION_ID=11111111-1111-1111-1111-111111111111", "--", "/tmp/xcodecli-test", "serve"}
			if name != "codex" || !reflect.DeepEqual(args, wantArgs) {
				t.Fatalf("unexpected command: %s %v", name, args)
			}
			return externalCommandResult{ExitCode: 0, Stdout: "Added global MCP server 'xcodecli'."}, nil
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{
			"mcp", "config",
			"--client", "codex",
			"--session-id", "11111111-1111-1111-1111-111111111111",
			"--write",
			"--json",
		}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		var result mcpConfigResult
		if err := json.Unmarshal([]byte(stdout.String()), &result); err != nil {
			t.Fatalf("stdout is not JSON result: %v", err)
		}
		if result.Client != "codex" || result.Name != "xcodecli" {
			t.Fatalf("unexpected result: %+v", result)
		}
		if result.Mode != "agent" {
			t.Fatalf("mode = %q, want agent", result.Mode)
		}
		if result.Server.Command != "/tmp/xcodecli-test" || !reflect.DeepEqual(result.Server.Args, []string{"serve"}) {
			t.Fatalf("unexpected server spec: %+v", result.Server)
		}
		if result.Write.ExitCode != 0 || !result.Write.Executed || !strings.Contains(result.Write.Stdout, "Added global MCP server") {
			t.Fatalf("unexpected write result: %+v", result.Write)
		}
		if result.Write.Stderr != "" {
			t.Fatalf("unexpected write stderr: %q", result.Write.Stderr)
		}
	})
}

func TestRunMCPConfigBridgeModePreservesBridgeTarget(t *testing.T) {
	withStubs(t, func() {
		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{
			"mcp", "config",
			"--client", "codex",
			"--mode", "bridge",
			"--json",
		}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		var result mcpConfigResult
		if err := json.Unmarshal([]byte(stdout.String()), &result); err != nil {
			t.Fatalf("stdout is not JSON result: %v", err)
		}
		if result.Mode != "bridge" {
			t.Fatalf("mode = %q, want bridge", result.Mode)
		}
		if !reflect.DeepEqual(result.Server.Args, []string{"bridge"}) {
			t.Fatalf("server args = %v, want [bridge]", result.Server.Args)
		}
	})
}

func TestRunMCPConfigWriteClaudeReplacesExistingServer(t *testing.T) {
	withStubs(t, func() {
		calls := []string{}
		defaultMCPCommandRunner = func(ctx context.Context, name string, args []string) (externalCommandResult, error) {
			calls = append(calls, name+" "+strings.Join(args, " "))
			switch len(calls) {
			case 1:
				if name != "claude" || args[0] != "mcp" || args[1] != "add-json" || args[3] != "local" {
					t.Fatalf("unexpected first claude command: %s %v", name, args)
				}
				return externalCommandResult{ExitCode: 1, Stderr: "MCP server xcodecli already exists in local config"}, nil
			case 2:
				want := []string{"mcp", "remove", "-s", "local", "xcodecli"}
				if name != "claude" || !reflect.DeepEqual(args, want) {
					t.Fatalf("unexpected remove command: %s %v", name, args)
				}
				return externalCommandResult{ExitCode: 0, Stdout: "Removed xcodecli"}, nil
			case 3:
				if name != "claude" || args[0] != "mcp" || args[1] != "add-json" {
					t.Fatalf("unexpected retry command: %s %v", name, args)
				}
				return externalCommandResult{ExitCode: 0, Stdout: "Added stdio MCP server xcodecli to local config"}, nil
			default:
				t.Fatalf("unexpected extra command: %s %v", name, args)
				return externalCommandResult{}, nil
			}
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{
			"mcp", "config",
			"--client", "claude",
			"--write",
			"--json",
		}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		if len(calls) != 3 {
			t.Fatalf("len(calls) = %d, want 3", len(calls))
		}
		var result mcpConfigResult
		if err := json.Unmarshal([]byte(stdout.String()), &result); err != nil {
			t.Fatalf("stdout is not JSON result: %v", err)
		}
		if result.Scope != "local" {
			t.Fatalf("scope = %q, want local", result.Scope)
		}
		if !result.Write.Executed || result.Write.ExitCode != 0 {
			t.Fatalf("unexpected write result: %+v", result.Write)
		}
		if !strings.Contains(result.Write.Stderr, "already exists") || !strings.Contains(result.Write.Stdout, "Removed xcodecli") || !strings.Contains(result.Write.Stdout, "Added stdio MCP server") {
			t.Fatalf("unexpected merged outputs: %+v", result.Write)
		}
	})
}

func TestRunMCPConfigWriteClaudeRetriesWhenRemoveSaysNotFound(t *testing.T) {
	withStubs(t, func() {
		callCount := 0
		defaultMCPCommandRunner = func(ctx context.Context, name string, args []string) (externalCommandResult, error) {
			callCount++
			switch callCount {
			case 1:
				return externalCommandResult{ExitCode: 1, Stderr: "MCP server xcodecli already exists in local config"}, nil
			case 2:
				return externalCommandResult{ExitCode: 1, Stderr: "No MCP server found with name xcodecli"}, nil
			case 3:
				return externalCommandResult{ExitCode: 0, Stdout: "Added stdio MCP server xcodecli to local config"}, nil
			default:
				t.Fatalf("unexpected call %d: %s %v", callCount, name, args)
				return externalCommandResult{}, nil
			}
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{
			"mcp", "config",
			"--client", "claude",
			"--write",
			"--json",
		}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		if callCount != 3 {
			t.Fatalf("callCount = %d, want 3", callCount)
		}
	})
}

func TestHelperFunctionsForMCPConfigFormatting(t *testing.T) {
	if !claudeRemoveNotFound(externalCommandResult{ExitCode: 1, Stderr: "No MCP server found"}) {
		t.Fatal("claudeRemoveNotFound = false, want true")
	}

	source := map[string]string{"A": "1"}
	cloned := copyStringMap(source)
	cloned["A"] = "2"
	if source["A"] != "1" {
		t.Fatalf("copyStringMap mutated source: %+v", source)
	}

	text := formatMCPConfigResult(mcpConfigResult{
		Client: "codex",
		Mode:   "agent",
		Name:   "xcodecli",
		Server: mcpConfigServerSpec{
			Command: "/tmp/xcodecli",
			Args:    []string{"serve"},
			Env:     map[string]string{"MCP_XCODE_PID": "123"},
		},
		DisplayCommand: "codex mcp add xcodecli -- /tmp/xcodecli serve",
		Write: mcpConfigWriteResult{
			Requested: true,
			Executed:  true,
			ExitCode:  0,
			Stdout:    "line one\nline two\n",
			Stderr:    "err one\nerr two\n",
		},
	})
	for _, want := range []string{"mode: agent", "write stdout:", "  line one", "  line two", "write stderr:", "  err one", "  err two"} {
		if !strings.Contains(text, want) {
			t.Fatalf("formatted output missing %q: %s", want, text)
		}
	}
}

func TestRunMCPConfigWriteMissingClientBinaryReturnsStructuredJSON(t *testing.T) {
	withStubs(t, func() {
		defaultMCPCommandRunner = func(ctx context.Context, name string, args []string) (externalCommandResult, error) {
			return externalCommandResult{}, errors.New("codex CLI not found on PATH")
		}

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{
			"mcp", "config",
			"--client", "codex",
			"--write",
			"--json",
		}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if stderr.String() != "" {
			t.Fatalf("stderr = %q, want empty stderr", stderr.String())
		}
		var result mcpConfigResult
		if err := json.Unmarshal([]byte(stdout.String()), &result); err != nil {
			t.Fatalf("stdout is not JSON result: %v", err)
		}
		if result.Write.Executed {
			t.Fatalf("write executed = true, want false")
		}
		if !strings.Contains(result.Write.Stderr, "codex CLI not found on PATH") {
			t.Fatalf("unexpected write stderr: %+v", result.Write)
		}
	})
}

func TestRunMCPConfigDoesNotCreatePersistentSessionFile(t *testing.T) {
	withStubs(t, func() {
		oldSessionPathFunc := defaultSessionPathFunc
		sessionPath := filepath.Join(t.TempDir(), "session-id")
		defaultSessionPathFunc = func() (string, error) { return sessionPath, nil }
		defer func() { defaultSessionPathFunc = oldSessionPathFunc }()

		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{"mcp", "config", "--client", "codex"}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
		}
		if _, err := os.Stat(sessionPath); !os.IsNotExist(err) {
			t.Fatalf("mcp config created persistent session file: %v", err)
		}
	})
}

func TestRunMCPConfigRejectsInvalidSessionID(t *testing.T) {
	withStubs(t, func() {
		var stdout strings.Builder
		var stderr strings.Builder
		code := run(context.Background(), []string{
			"mcp", "config",
			"--client", "codex",
			"--session-id", "not-a-uuid",
		}, strings.NewReader(""), &stdout, &stderr, os.Environ())
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "invalid MCP config options") {
			t.Fatalf("stderr = %q, want invalid options message", stderr.String())
		}
	})
}

func TestResolveCurrentExecutablePathUsesLookPathForBareCommand(t *testing.T) {
	withExecutableResolverStubs(t, func() {
		defaultArgv0Func = func() string { return "xcodecli" }
		defaultLookPathFunc = func(file string) (string, error) {
			if file != "xcodecli" {
				t.Fatalf("file = %q, want xcodecli", file)
			}
			return "/opt/homebrew/bin/xcodecli", nil
		}
		defaultOSExecutableFunc = func() (string, error) {
			return "/private/var/folders/tmp/go-build123/b001/exe/xcodecli", nil
		}
		path, err := resolveCurrentExecutablePath()
		if err != nil {
			t.Fatalf("resolveCurrentExecutablePath returned error: %v", err)
		}
		if path != "/opt/homebrew/bin/xcodecli" {
			t.Fatalf("path = %q, want stable lookup path", path)
		}
	})
}

func TestResolveCurrentExecutablePathKeepsAbsolutePathWithoutResolvingSymlink(t *testing.T) {
	withExecutableResolverStubs(t, func() {
		defaultArgv0Func = func() string { return "/opt/homebrew/bin/xcodecli" }
		path, err := resolveCurrentExecutablePath()
		if err != nil {
			t.Fatalf("resolveCurrentExecutablePath returned error: %v", err)
		}
		if path != "/opt/homebrew/bin/xcodecli" {
			t.Fatalf("path = %q, want unchanged absolute path", path)
		}
	})
}

func TestResolveCurrentExecutablePathRejectsTemporaryGoBuildOutput(t *testing.T) {
	withExecutableResolverStubs(t, func() {
		defaultArgv0Func = func() string { return "/private/var/folders/ab/cd/T/go-build123456/b001/exe/xcodecli" }
		defaultTempDirFunc = func() string { return "/var/folders/ab/cd/T" }
		_, err := resolveCurrentExecutablePath()
		if err == nil || !strings.Contains(err.Error(), "temporary Go build output") {
			t.Fatalf("expected temporary go build error, got %v", err)
		}
	})
}

func withExecutableResolverStubs(t *testing.T, fn func()) {
	t.Helper()
	oldArgv0 := defaultArgv0Func
	oldGetwd := defaultGetwdFunc
	oldLookPath := defaultLookPathFunc
	oldOSExecutable := defaultOSExecutableFunc
	oldTempDir := defaultTempDirFunc
	defaultArgv0Func = func() string { return "" }
	defaultGetwdFunc = func() (string, error) { return "/tmp", nil }
	defaultLookPathFunc = func(file string) (string, error) { return "", errors.New("unexpected lookpath") }
	defaultOSExecutableFunc = func() (string, error) { return "/tmp/xcodecli", nil }
	defaultTempDirFunc = func() string { return "/tmp" }
	defer func() {
		defaultArgv0Func = oldArgv0
		defaultGetwdFunc = oldGetwd
		defaultLookPathFunc = oldLookPath
		defaultOSExecutableFunc = oldOSExecutable
		defaultTempDirFunc = oldTempDir
	}()
	fn()
}
