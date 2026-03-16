package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/oozoofrog/xcodecli/internal/bridge"
	"github.com/oozoofrog/xcodecli/internal/pathutil"
)

type externalCommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type mcpConfigServerSpec struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

type mcpConfigWriteResult struct {
	Requested bool   `json:"requested"`
	Executed  bool   `json:"executed"`
	ExitCode  int    `json:"exitCode"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
}

type mcpConfigResult struct {
	Client         string               `json:"client"`
	Mode           string               `json:"mode"`
	Name           string               `json:"name"`
	Scope          string               `json:"scope,omitempty"`
	Server         mcpConfigServerSpec  `json:"server"`
	Command        []string             `json:"command"`
	DisplayCommand string               `json:"displayCommand"`
	Write          mcpConfigWriteResult `json:"write"`
}

type mcpCommandRunner func(ctx context.Context, name string, args []string) (externalCommandResult, error)

type commandInvocation struct {
	Name string
	Args []string
}

var defaultExecutablePathFunc = resolveCurrentExecutablePath
var defaultMCPCommandRunner mcpCommandRunner = runExternalCommand
var defaultArgv0Func = func() string {
	if len(os.Args) == 0 {
		return ""
	}
	return os.Args[0]
}
var defaultGetwdFunc = os.Getwd
var defaultLookPathFunc = exec.LookPath
var defaultOSExecutableFunc = os.Executable
var defaultTempDirFunc = os.TempDir

func runMCPConfig(ctx context.Context, cfg cliConfig, stdout, stderr io.Writer) int {
	explicit := bridge.EnvOptions{XcodePID: cfg.XcodePID, SessionID: cfg.SessionID}
	if err := bridge.ValidateEnvOptions(explicit); err != nil {
		fmt.Fprintf(stderr, "xcodecli: invalid MCP config options: %v\n", err)
		return 1
	}

	executablePath, err := defaultExecutablePathFunc()
	if err != nil {
		fmt.Fprintf(stderr, "xcodecli: %v\n", err)
		return 1
	}

	result, err := buildMCPConfigResult(cfg, executablePath)
	if err != nil {
		fmt.Fprintf(stderr, "xcodecli: %v\n", err)
		return 1
	}

	exitCode := 0
	if cfg.Write {
		result.Write = performMCPConfigWrite(ctx, cfg, result)
		if !result.Write.Executed || result.Write.ExitCode != 0 {
			exitCode = 1
		}
	}

	if cfg.JSONOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprint(stdout, formatMCPConfigResult(result))
	}

	return exitCode
}

func buildMCPConfigResult(cfg cliConfig, executablePath string) (mcpConfigResult, error) {
	serverArgs := []string{"serve"}
	if cfg.MCPMode == "bridge" {
		serverArgs = []string{"bridge"}
	}
	server := mcpConfigServerSpec{
		Command: executablePath,
		Args:    serverArgs,
		Env:     explicitMCPConfigEnv(cfg),
	}

	invocation, err := buildMCPConfigInvocation(cfg, server)
	if err != nil {
		return mcpConfigResult{}, err
	}

	command := append([]string{invocation.Name}, invocation.Args...)
	return mcpConfigResult{
		Client:         cfg.MCPClient,
		Mode:           cfg.MCPMode,
		Name:           cfg.ConfigName,
		Scope:          cfg.Scope,
		Server:         server,
		Command:        append([]string{}, command...),
		DisplayCommand: shellQuoteCommand(command),
		Write: mcpConfigWriteResult{
			Requested: cfg.Write,
		},
	}, nil
}

func buildMCPConfigInvocation(cfg cliConfig, server mcpConfigServerSpec) (commandInvocation, error) {
	switch cfg.MCPClient {
	case "codex":
		args := []string{"mcp", "add", cfg.ConfigName}
		args = append(args, envArgs("--env", server.Env)...)
		args = append(args, "--", server.Command)
		args = append(args, server.Args...)
		return commandInvocation{Name: "codex", Args: args}, nil
	case "claude":
		payload, err := buildClaudeJSONPayload(server)
		if err != nil {
			return commandInvocation{}, err
		}
		return commandInvocation{
			Name: "claude",
			Args: []string{"mcp", "add-json", "-s", cfg.Scope, cfg.ConfigName, payload},
		}, nil
	case "gemini":
		args := []string{"mcp", "add", "-s", cfg.Scope}
		args = append(args, envArgs("-e", server.Env)...)
		args = append(args, cfg.ConfigName, server.Command)
		args = append(args, server.Args...)
		return commandInvocation{Name: "gemini", Args: args}, nil
	default:
		return commandInvocation{}, fmt.Errorf("unsupported MCP client: %s", cfg.MCPClient)
	}
}

func buildClaudeRemoveInvocation(cfg cliConfig) commandInvocation {
	return commandInvocation{Name: "claude", Args: []string{"mcp", "remove", "-s", cfg.Scope, cfg.ConfigName}}
}

func performMCPConfigWrite(ctx context.Context, cfg cliConfig, result mcpConfigResult) mcpConfigWriteResult {
	writeResult := result.Write
	writeResult.Requested = true

	invocation, err := buildMCPConfigInvocation(cfg, result.Server)
	if err != nil {
		writeResult.Stderr = err.Error()
		return writeResult
	}

	switch cfg.MCPClient {
	case "claude":
		firstRun, firstErr := runInvocation(ctx, invocation)
		mergeWriteResult(&writeResult, firstRun, firstErr)
		if firstErr == nil && firstRun.ExitCode == 0 {
			return writeResult
		}
		if !claudeAlreadyExists(firstRun, firstErr) {
			return writeResult
		}

		removeRun, removeErr := runInvocation(ctx, buildClaudeRemoveInvocation(cfg))
		mergeWriteResult(&writeResult, removeRun, removeErr)
		if removeErr != nil {
			return writeResult
		}
		if removeRun.ExitCode != 0 && !claudeRemoveNotFound(removeRun) {
			return writeResult
		}

		retryRun, retryErr := runInvocation(ctx, invocation)
		mergeWriteResult(&writeResult, retryRun, retryErr)
		return writeResult
	default:
		runResult, runErr := runInvocation(ctx, invocation)
		mergeWriteResult(&writeResult, runResult, runErr)
		return writeResult
	}
}

func runInvocation(ctx context.Context, invocation commandInvocation) (externalCommandResult, error) {
	return defaultMCPCommandRunner(ctx, invocation.Name, invocation.Args)
}

func mergeWriteResult(target *mcpConfigWriteResult, run externalCommandResult, err error) {
	if target == nil {
		return
	}
	if err != nil {
		target.Stderr = joinOutput(target.Stderr, err.Error())
		return
	}
	target.Executed = true
	target.ExitCode = run.ExitCode
	target.Stdout = joinOutput(target.Stdout, run.Stdout)
	target.Stderr = joinOutput(target.Stderr, run.Stderr)
}

func claudeAlreadyExists(run externalCommandResult, err error) bool {
	if err != nil || run.ExitCode == 0 {
		return false
	}
	combined := strings.ToLower(run.Stdout + "\n" + run.Stderr)
	return strings.Contains(combined, "already exists")
}

func claudeRemoveNotFound(run externalCommandResult) bool {
	if run.ExitCode == 0 {
		return false
	}
	combined := strings.ToLower(run.Stdout + "\n" + run.Stderr)
	return strings.Contains(combined, "no mcp server found")
}

func explicitMCPConfigEnv(cfg cliConfig) map[string]string {
	env := map[string]string{}
	if cfg.XcodePID != "" {
		env["MCP_XCODE_PID"] = cfg.XcodePID
	}
	if cfg.SessionID != "" {
		env["MCP_XCODE_SESSION_ID"] = cfg.SessionID
	}
	return env
}

func buildClaudeJSONPayload(server mcpConfigServerSpec) (string, error) {
	type claudePayload struct {
		Type    string            `json:"type"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env,omitempty"`
	}
	payload := claudePayload{
		Type:    "stdio",
		Command: server.Command,
		Args:    append([]string{}, server.Args...),
	}
	if len(server.Env) > 0 {
		payload.Env = copyStringMap(server.Env)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal Claude MCP JSON payload: %w", err)
	}
	return string(data), nil
}

func copyStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func envArgs(flagName string, env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	args := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		args = append(args, flagName, key+"="+env[key])
	}
	return args
}

func formatMCPConfigResult(result mcpConfigResult) string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "client: %s\n", result.Client)
	fmt.Fprintf(&buf, "mode: %s\n", result.Mode)
	fmt.Fprintf(&buf, "name: %s\n", result.Name)
	if result.Scope != "" {
		fmt.Fprintf(&buf, "scope: %s\n", result.Scope)
	}
	fmt.Fprintf(&buf, "server: %s %s\n", result.Server.Command, strings.Join(result.Server.Args, " "))
	if len(result.Server.Env) == 0 {
		fmt.Fprintln(&buf, "env: none")
	} else {
		fmt.Fprintln(&buf, "env:")
		keys := make([]string, 0, len(result.Server.Env))
		for key := range result.Server.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&buf, "  %s=%s\n", key, result.Server.Env[key])
		}
	}
	fmt.Fprintln(&buf, "command:")
	fmt.Fprintf(&buf, "  %s\n", result.DisplayCommand)
	fmt.Fprintf(&buf, "write requested: %t\n", result.Write.Requested)
	if result.Write.Requested {
		fmt.Fprintf(&buf, "write executed: %t\n", result.Write.Executed)
		fmt.Fprintf(&buf, "write exit code: %d\n", result.Write.ExitCode)
		if result.Write.Stdout != "" {
			fmt.Fprintln(&buf, "write stdout:")
			writeIndentedBlock(&buf, result.Write.Stdout)
		}
		if result.Write.Stderr != "" {
			fmt.Fprintln(&buf, "write stderr:")
			writeIndentedBlock(&buf, result.Write.Stderr)
		}
	}
	return buf.String()
}

func writeIndentedBlock(buf *strings.Builder, raw string) {
	for _, line := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		if line == "" {
			fmt.Fprintln(buf, "  ")
			continue
		}
		fmt.Fprintf(buf, "  %s\n", line)
	}
}

func joinOutput(existing, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	switch {
	case existing == "":
		return next
	case next == "":
		return existing
	default:
		return existing + "\n" + next
	}
}

func shellQuoteCommand(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, part := range argv {
		parts = append(parts, mcpShellQuote(part))
	}
	return strings.Join(parts, " ")
}

func mcpShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z':
			return false
		case r >= 'A' && r <= 'Z':
			return false
		case r >= '0' && r <= '9':
			return false
		case r == '-', r == '_', r == '.', r == '/', r == ':', r == '=':
			return false
		default:
			return true
		}
	}) == -1 {
		return value
	}
	return shellQuote(value)
}

func resolveCurrentExecutablePath() (string, error) {
	if path, ok, err := resolveConfiguredExecutablePath(); err != nil {
		return "", err
	} else if ok {
		return path, nil
	}

	path, err := defaultOSExecutableFunc()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	return validateConfiguredExecutablePath(filepath.Clean(path))
}

func resolveConfiguredExecutablePath() (string, bool, error) {
	raw := strings.TrimSpace(defaultArgv0Func())
	if raw == "" {
		return "", false, nil
	}
	if filepath.IsAbs(raw) {
		path := filepath.Clean(raw)
		validated, err := validateConfiguredExecutablePath(path)
		return validated, true, err
	}
	if strings.Contains(raw, string(os.PathSeparator)) {
		cwd, err := defaultGetwdFunc()
		if err != nil {
			return "", false, fmt.Errorf("resolve current working directory: %w", err)
		}
		path := filepath.Clean(filepath.Join(cwd, raw))
		validated, err := validateConfiguredExecutablePath(path)
		return validated, true, err
	}
	lookedUp, err := defaultLookPathFunc(raw)
	if err != nil {
		return "", false, nil
	}
	path := filepath.Clean(lookedUp)
	validated, err := validateConfiguredExecutablePath(path)
	return validated, true, err
}

func validateConfiguredExecutablePath(path string) (string, error) {
	if pathutil.IsTemporaryGoBuildExecutable(path, defaultTempDirFunc) {
		return "", fmt.Errorf("current executable path appears to be a temporary Go build output (%s); rerun `mcp config` using an installed or directly built xcodecli binary", path)
	}
	return path, nil
}

func runExternalCommand(ctx context.Context, name string, args []string) (externalCommandResult, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return externalCommandResult{}, fmt.Errorf("%s CLI not found on PATH", name)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	result := externalCommandResult{
		ExitCode: 0,
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, fmt.Errorf("run %s: %w", name, err)
}
