package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/oozoofrog/xcodecli/internal/agent"
	"github.com/oozoofrog/xcodecli/internal/bridge"
	"github.com/oozoofrog/xcodecli/internal/doctor"
	"github.com/oozoofrog/xcodecli/internal/mcp"
)

var defaultBridgeCommand = bridge.Command{Path: "xcrun", Args: []string{"mcpbridge"}}
var defaultMCPCommand = mcp.Command{Path: "xcrun", Args: []string{"mcpbridge"}}
var defaultSessionPathFunc = bridge.DefaultSessionFilePath
var defaultMCPServeFunc = runServeMCP
var defaultAgentConfigFunc = func(command mcp.Command, env []string, errOut io.Writer) (agent.Config, error) {
	return agent.DefaultConfig(command, env, errOut)
}
var defaultToolsListFunc = agent.ListTools
var defaultToolCallFunc = agent.CallTool
var defaultAgentStatusFunc = agent.StatusInfo
var defaultAgentStopFunc = agent.Stop
var defaultAgentUninstallFunc = agent.Uninstall
var defaultAgentRunFunc = agent.RunServer
var defaultDoctorRunFunc = func(ctx context.Context, opts doctor.Options) doctor.Report {
	return doctor.NewInspector().Run(ctx, opts)
}

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
		fmt.Fprintf(stderr, "xcodecli: %v\n", err)
		if usage != "" {
			fmt.Fprint(stderr, usage)
		}
		return 1
	}

	if cfg.Command == commandVersion {
		fmt.Fprintln(stdout, versionLine())
		return 0
	}

	if runtime.GOOS != "darwin" {
		fmt.Fprintln(stderr, "xcodecli: only macOS (darwin) is supported")
		return 1
	}

	agentCfg, err := defaultAgentConfigFunc(defaultMCPCommand, env, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "xcodecli: %v\n", err)
		return 1
	}
	if cfg.IdleTimeout > 0 {
		agentCfg.IdleTimeout = cfg.IdleTimeout
	}

	switch cfg.Command {
	case commandUpdate:
		return runUpdate(ctx, stdout, stderr)
	case commandDoctor:
		resolved, err := resolveEffectiveOptions(env, cfg)
		if err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		agentStatus, agentStatusErr := defaultAgentStatusFunc(ctx, agentCfg)
		report := defaultDoctorRunFunc(ctx, doctor.Options{
			BaseEnv:        env,
			XcodePID:       resolved.XcodePID,
			SessionID:      resolved.SessionID,
			SessionSource:  resolved.SessionSource,
			SessionPath:    resolved.SessionPath,
			AgentStatus:    &agentStatus,
			AgentStatusErr: agentStatusErr,
		})
		if cfg.JSONOutput {
			if err := writeJSON(stdout, report.JSON()); err != nil {
				fmt.Fprintf(stderr, "xcodecli: %v\n", err)
				return 1
			}
		} else {
			fmt.Fprint(stdout, report.String())
		}
		if report.Success() {
			return 0
		}
		return 1
	case commandMCPConfig:
		return runMCPConfig(ctx, cfg, stdout, stderr)
	case commandBridge:
		resolved, err := resolveEffectiveOptions(env, cfg)
		if err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		if cfg.Debug {
			logResolvedSession(stderr, resolved)
		}
		effective := resolved.EnvOptions
		if err := bridge.ValidateEnvOptions(effective); err != nil {
			fmt.Fprintf(stderr, "xcodecli: invalid bridge options: %v\n", err)
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
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		return result.ExitCode
	case commandServe:
		return runServe(ctx, cfg, env, stdin, stdout, stderr, agentCfg)
	case commandToolsList:
		effective, err := resolveAndValidateOptions(env, cfg, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		requestCtx, cancel := requestTimeoutContext(ctx, cfg.Timeout)
		defer cancel()
		request := agentRequest(env, effective, cfg)
		tools, err := defaultToolsListFunc(requestCtx, agentCfg, request)
		if err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		if cfg.JSONOutput {
			if err := writeJSON(stdout, tools); err != nil {
				fmt.Fprintf(stderr, "xcodecli: %v\n", err)
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
	case commandToolInspect:
		effective, err := resolveAndValidateOptions(env, cfg, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		requestCtx, cancel := requestTimeoutContext(ctx, cfg.Timeout)
		defer cancel()
		request := agentRequest(env, effective, cfg)
		tools, err := defaultToolsListFunc(requestCtx, agentCfg, request)
		if err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		tool, found := findToolByName(tools, cfg.ToolName)
		if !found {
			fmt.Fprintf(stderr, "xcodecli: tool not found: %s\n", cfg.ToolName)
			return 1
		}
		if cfg.JSONOutput {
			if err := writeJSON(stdout, tool); err != nil {
				fmt.Fprintf(stderr, "xcodecli: %v\n", err)
				return 1
			}
			return 0
		}
		if err := writeToolInspect(stdout, tool); err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		return 0
	case commandToolCall:
		effective, err := resolveAndValidateOptions(env, cfg, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		arguments, err := resolveToolArguments(stdin, cfg)
		if err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		requestCtx, cancel := requestTimeoutContext(ctx, cfg.Timeout)
		defer cancel()
		request := agentRequest(env, effective, cfg)
		result, err := defaultToolCallFunc(requestCtx, agentCfg, request, cfg.ToolName, arguments)
		if err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		if err := writeJSON(stdout, result.Result); err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		if result.IsError {
			return 1
		}
		return 0
	case commandAgentGuide:
		return runAgentGuide(ctx, cfg, env, stdout, stderr, agentCfg)
	case commandAgentDemo:
		return runAgentDemo(ctx, cfg, env, stdout, stderr, agentCfg)
	case commandAgentStatus:
		status, err := defaultAgentStatusFunc(ctx, agentCfg)
		if err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		if cfg.JSONOutput {
			if err := writeJSON(stdout, status); err != nil {
				fmt.Fprintf(stderr, "xcodecli: %v\n", err)
				return 1
			}
		} else {
			fmt.Fprint(stdout, formatAgentStatus(status))
		}
		return 0
	case commandAgentStop:
		if err := defaultAgentStopFunc(ctx, agentCfg); err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "stopped LaunchAgent process if it was running")
		return 0
	case commandAgentUninstall:
		if err := defaultAgentUninstallFunc(ctx, agentCfg); err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "removed LaunchAgent plist and local agent runtime files")
		return 0
	case commandAgentRun:
		signalCtx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		defer cancel()
		if err := defaultAgentRunFunc(signalCtx, agentCfg); err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "xcodecli: unsupported command %q\n", cfg.Command)
		return 1
	}
}

func resolveEffectiveOptions(env []string, cfg cliConfig) (bridge.ResolvedOptions, error) {
	sessionPath, err := defaultSessionPathFunc()
	if err != nil {
		return bridge.ResolvedOptions{}, err
	}
	return bridge.ResolveOptions(env, bridge.EnvOptions{
		XcodePID:  cfg.XcodePID,
		SessionID: cfg.SessionID,
	}, sessionPath)
}

func resolveAndValidateOptions(env []string, cfg cliConfig, stderr io.Writer) (bridge.EnvOptions, error) {
	resolved, err := resolveEffectiveOptions(env, cfg)
	if err != nil {
		return bridge.EnvOptions{}, err
	}
	if cfg.Debug {
		logResolvedSession(stderr, resolved)
	}
	effective := resolved.EnvOptions
	if err := bridge.ValidateEnvOptions(effective); err != nil {
		return bridge.EnvOptions{}, fmt.Errorf("invalid MCP options: %v", err)
	}
	return effective, nil
}

func agentRequest(env []string, effective bridge.EnvOptions, cfg cliConfig) agent.Request {
	return agent.BuildRequest(env, effective, cfg.Timeout, cfg.Debug)
}

func requestTimeoutContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout > 0 {
		return context.WithTimeout(parent, timeout)
	}
	return context.WithCancel(parent)
}

func findToolByName(tools []map[string]any, name string) (map[string]any, bool) {
	for _, tool := range tools {
		if toolName, _ := tool["name"].(string); toolName == name {
			return tool, true
		}
	}
	return nil, false
}

func resolveToolArguments(stdin io.Reader, cfg cliConfig) (map[string]any, error) {
	switch {
	case cfg.ToolInputFromStdin:
		payload, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read --json-stdin payload: %w", err)
		}
		return parseJSONObject(string(payload))
	case strings.HasPrefix(cfg.ToolInputJSON, "@"):
		path := strings.TrimPrefix(cfg.ToolInputJSON, "@")
		if path == "" {
			return nil, errors.New("--json @file requires a non-empty path")
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read JSON payload file %s: %w", path, err)
		}
		return parseJSONObject(string(payload))
	default:
		return parseJSONObject(cfg.ToolInputJSON)
	}
}

func writeToolInspect(w io.Writer, tool map[string]any) error {
	name, _ := tool["name"].(string)
	description, _ := tool["description"].(string)
	if _, err := fmt.Fprintf(w, "name: %s\n", name); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "description: %s\n", description); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "inputSchema:"); err != nil {
		return err
	}
	if err := writeJSON(w, tool["inputSchema"]); err != nil {
		return err
	}
	return nil
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

func formatAgentStatus(status agent.Status) string {
	binaryLine := status.RegisteredBinary
	if binaryLine == "" {
		binaryLine = "not installed"
	}
	matchText := "n/a"
	if status.RegisteredBinary != "" && status.CurrentBinary != "" {
		if status.BinaryPathMatches {
			matchText = "yes"
		} else {
			matchText = "no"
		}
	}
	runningText := "no"
	if status.Running {
		runningText = "yes"
	}
	socketText := "no"
	if status.SocketReachable {
		socketText = "yes"
	}
	var buf strings.Builder
	fmt.Fprintf(&buf, "xcodecli agent\n\nlabel: %s\nplist installed: %t\nplist path: %s\nregistered binary: %s\ncurrent binary: %s\nbinary matches: %s\nsocket path: %s\nsocket reachable: %s\nrunning: %s\npid: %d\nmcpbridge session idle timeout: %s\nbackend sessions: %d\n",
		status.Label,
		status.PlistInstalled,
		status.PlistPath,
		binaryLine,
		status.CurrentBinary,
		matchText,
		status.SocketPath,
		socketText,
		runningText,
		status.PID,
		formatTimeoutDuration(status.IdleTimeout),
		status.BackendSessions,
	)
	warnings := status.Warnings
	if len(warnings) == 0 {
		warnings = agentStatusWarnings(status)
	}
	if len(warnings) > 0 {
		fmt.Fprintln(&buf, "warnings:")
		for _, warning := range warnings {
			fmt.Fprintf(&buf, "- %s\n", warning)
		}
	}
	nextSteps := status.NextSteps
	if len(nextSteps) == 0 {
		nextSteps = agentStatusNextSteps(status, warnings)
	}
	if len(nextSteps) > 0 {
		fmt.Fprintln(&buf, "next steps:")
		for _, step := range nextSteps {
			fmt.Fprintf(&buf, "- %s\n", step)
		}
	}
	return buf.String()
}

func agentStatusWarnings(status agent.Status) []string {
	warnings := []string{}
	if strings.TrimSpace(status.RegisteredBinary) != "" && !filepath.IsAbs(status.RegisteredBinary) {
		warnings = append(warnings, "registered LaunchAgent binary path is relative; stale older installs can make launchctl bootstrap fail with Input/output error")
	}
	if strings.TrimSpace(status.RegisteredBinary) != "" {
		if info, err := os.Stat(status.RegisteredBinary); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			warnings = append(warnings, "registered LaunchAgent binary is missing or not executable; the next LaunchAgent bootstrap may fail until the plist is rewritten")
		}
	}
	if strings.TrimSpace(status.RegisteredBinary) != "" && strings.TrimSpace(status.CurrentBinary) != "" && !status.BinaryPathMatches {
		warnings = append(warnings, "registered LaunchAgent binary differs from the current binary; switching binaries recycles the backend session and can surface fresh Xcode authorization prompts")
	}
	return dedupeStrings(warnings)
}

func agentStatusNextSteps(status agent.Status, warnings []string) []string {
	steps := []string{}
	if len(warnings) > 0 {
		steps = append(steps, "if the registration looks stale, run `xcodecli agent uninstall` and then re-register from one stable xcodecli path")
	}
	if status.PlistInstalled && !status.Running && !status.SocketReachable {
		steps = append(steps, "run `xcodecli agent demo` or `xcodecli tools list --json` to bootstrap the LaunchAgent again")
	}
	return steps
}

func parseJSONObject(raw string) (map[string]any, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("JSON payload must be valid JSON: %w", err)
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("JSON payload must decode to a JSON object")
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
