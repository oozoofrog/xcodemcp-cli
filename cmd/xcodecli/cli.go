package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/oozoofrog/xcodecli/internal/agent"
)

type commandName string

const (
	commandBridge         commandName = "bridge"
	commandDoctor         commandName = "doctor"
	commandToolsList      commandName = "tools-list"
	commandToolCall       commandName = "tool-call"
	commandToolInspect    commandName = "tool-inspect"
	commandAgentStatus    commandName = "agent-status"
	commandAgentGuide     commandName = "agent-guide"
	commandAgentDemo      commandName = "agent-demo"
	commandAgentStop      commandName = "agent-stop"
	commandAgentUninstall commandName = "agent-uninstall"
	commandAgentRun       commandName = "agent-run"
)

type cliConfig struct {
	Command            commandName
	XcodePID           string
	SessionID          string
	Debug              bool
	JSONOutput         bool
	Timeout            time.Duration
	IdleTimeout        time.Duration
	ToolName           string
	Intent             string
	ToolInputJSON      string
	ToolInputFromStdin bool
	LaunchAgent        bool
}

var errUsageRequested = errors.New("usage requested")

func parseCLI(args []string) (cliConfig, string, error) {
	if len(args) == 0 {
		return cliConfig{}, rootUsage(), errUsageRequested
	}

	switch args[0] {
	case "help", "-h", "--help":
		return parseHelp(args[1:])
	case string(commandBridge):
		cfg, err := parseBridgeFlags("xcodecli bridge", args[1:])
		cfg.Command = commandBridge
		return cfg, bridgeUsage(), err
	case string(commandDoctor):
		cfg, err := parseDoctorFlags("xcodecli doctor", args[1:])
		cfg.Command = commandDoctor
		return cfg, doctorUsage(), err
	case "tools":
		return parseToolsCLI(args[1:])
	case "tool":
		return parseToolCLI(args[1:])
	case "agent":
		return parseAgentCLI(args[1:])
	default:
		if strings.HasPrefix(args[0], "-") {
			cfg, err := parseBridgeFlags("xcodecli", args)
			cfg.Command = commandBridge
			return cfg, bridgeUsage(), err
		}
		return cliConfig{}, rootUsage(), fmt.Errorf("unknown command: %s", args[0])
	}
}

func parseHelp(args []string) (cliConfig, string, error) {
	if len(args) == 0 {
		return cliConfig{}, rootUsage(), errUsageRequested
	}
	switch args[0] {
	case string(commandBridge):
		return cliConfig{}, bridgeUsage(), errUsageRequested
	case string(commandDoctor):
		return cliConfig{}, doctorUsage(), errUsageRequested
	case "tools":
		if len(args) == 1 {
			return cliConfig{}, toolsUsage(), errUsageRequested
		}
		if len(args) == 2 && args[1] == "list" {
			return cliConfig{}, toolsListUsage(), errUsageRequested
		}
	case "tool":
		if len(args) == 1 {
			return cliConfig{}, toolUsage(), errUsageRequested
		}
		if len(args) == 2 {
			switch args[1] {
			case "call":
				return cliConfig{}, toolCallUsage(), errUsageRequested
			case "inspect":
				return cliConfig{}, toolInspectUsage(), errUsageRequested
			}
		}
	case "agent":
		if len(args) == 1 {
			return cliConfig{}, agentUsage(), errUsageRequested
		}
		if len(args) == 2 {
			switch args[1] {
			case "guide":
				return cliConfig{}, agentGuideUsage(), errUsageRequested
			case "demo":
				return cliConfig{}, agentDemoUsage(), errUsageRequested
			case "status":
				return cliConfig{}, agentStatusUsage(), errUsageRequested
			case "stop":
				return cliConfig{}, agentStopUsage(), errUsageRequested
			case "uninstall":
				return cliConfig{}, agentUninstallUsage(), errUsageRequested
			case "run":
				return cliConfig{}, agentRunUsage(), errUsageRequested
			}
		}
	}
	return cliConfig{}, rootUsage(), fmt.Errorf("unknown help topic: %s", strings.Join(args, " "))
}

func parseToolsCLI(args []string) (cliConfig, string, error) {
	if len(args) == 0 {
		return cliConfig{}, toolsUsage(), errUsageRequested
	}
	switch args[0] {
	case "list":
		cfg, err := parseToolsListFlags("xcodecli tools list", args[1:])
		cfg.Command = commandToolsList
		return cfg, toolsListUsage(), err
	default:
		return cliConfig{}, toolsUsage(), fmt.Errorf("unknown tools subcommand: %s", args[0])
	}
}

func parseToolCLI(args []string) (cliConfig, string, error) {
	if len(args) == 0 {
		return cliConfig{}, toolUsage(), errUsageRequested
	}
	switch args[0] {
	case "call":
		cfg, err := parseToolCallFlags("xcodecli tool call", args[1:])
		cfg.Command = commandToolCall
		return cfg, toolCallUsage(), err
	case "inspect":
		cfg, err := parseToolInspectFlags("xcodecli tool inspect", args[1:])
		cfg.Command = commandToolInspect
		return cfg, toolInspectUsage(), err
	default:
		return cliConfig{}, toolUsage(), fmt.Errorf("unknown tool subcommand: %s", args[0])
	}
}

func parseAgentCLI(args []string) (cliConfig, string, error) {
	if len(args) == 0 {
		return cliConfig{}, agentUsage(), errUsageRequested
	}
	switch args[0] {
	case "guide":
		cfg, err := parseAgentGuideFlags("xcodecli agent guide", args[1:])
		cfg.Command = commandAgentGuide
		return cfg, agentGuideUsage(), err
	case "demo":
		cfg, err := parseAgentDemoFlags("xcodecli agent demo", args[1:])
		cfg.Command = commandAgentDemo
		return cfg, agentDemoUsage(), err
	case "status":
		cfg, err := parseAgentStatusFlags("xcodecli agent status", args[1:])
		cfg.Command = commandAgentStatus
		return cfg, agentStatusUsage(), err
	case "stop":
		cfg, err := parseAgentSimpleFlags("xcodecli agent stop", args[1:])
		cfg.Command = commandAgentStop
		return cfg, agentStopUsage(), err
	case "uninstall":
		cfg, err := parseAgentSimpleFlags("xcodecli agent uninstall", args[1:])
		cfg.Command = commandAgentUninstall
		return cfg, agentUninstallUsage(), err
	case "run":
		cfg, err := parseAgentRunFlags("xcodecli agent run", args[1:])
		cfg.Command = commandAgentRun
		return cfg, agentRunUsage(), err
	default:
		return cliConfig{}, agentUsage(), fmt.Errorf("unknown agent subcommand: %s", args[0])
	}
}

func parseBridgeFlags(name string, args []string) (cliConfig, error) {
	fs := newFlagSet(name)
	cfg := cliConfig{Command: commandBridge}
	help := false
	fs.StringVar(&cfg.XcodePID, "xcode-pid", "", "")
	fs.StringVar(&cfg.SessionID, "session-id", "", "")
	fs.BoolVar(&cfg.Debug, "debug", false, "")
	fs.BoolVar(&help, "h", false, "")
	fs.BoolVar(&help, "help", false, "")
	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	if help {
		return cliConfig{}, errUsageRequested
	}
	if fs.NArg() != 0 {
		return cliConfig{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

func parseDoctorFlags(name string, args []string) (cliConfig, error) {
	fs := newFlagSet(name)
	cfg := cliConfig{Command: commandDoctor}
	help := false
	fs.StringVar(&cfg.XcodePID, "xcode-pid", "", "")
	fs.StringVar(&cfg.SessionID, "session-id", "", "")
	fs.BoolVar(&cfg.JSONOutput, "json", false, "")
	fs.BoolVar(&help, "h", false, "")
	fs.BoolVar(&help, "help", false, "")
	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	if help {
		return cliConfig{}, errUsageRequested
	}
	if fs.NArg() != 0 {
		return cliConfig{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

func parseToolsListFlags(name string, args []string) (cliConfig, error) {
	fs := newFlagSet(name)
	cfg := cliConfig{Command: commandToolsList, Timeout: 30 * time.Second}
	help := false
	fs.StringVar(&cfg.XcodePID, "xcode-pid", "", "")
	fs.StringVar(&cfg.SessionID, "session-id", "", "")
	fs.BoolVar(&cfg.Debug, "debug", false, "")
	fs.BoolVar(&cfg.JSONOutput, "json", false, "")
	fs.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "")
	fs.BoolVar(&help, "h", false, "")
	fs.BoolVar(&help, "help", false, "")
	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	if help {
		return cliConfig{}, errUsageRequested
	}
	if cfg.Timeout <= 0 {
		return cliConfig{}, errors.New("--timeout must be greater than 0")
	}
	if fs.NArg() != 0 {
		return cliConfig{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

func parseToolCallFlags(name string, args []string) (cliConfig, error) {
	if containsHelpFlag(args) {
		return cliConfig{}, errUsageRequested
	}
	toolName, flagArgs, err := extractPositionalArg(args, toolCallFlagTakesValue)
	if err != nil {
		return cliConfig{}, err
	}

	fs := newFlagSet(name)
	cfg := cliConfig{Command: commandToolCall, Timeout: 30 * time.Second, ToolName: toolName}
	fs.StringVar(&cfg.XcodePID, "xcode-pid", "", "")
	fs.StringVar(&cfg.SessionID, "session-id", "", "")
	fs.BoolVar(&cfg.Debug, "debug", false, "")
	fs.StringVar(&cfg.ToolInputJSON, "json", "", "")
	fs.BoolVar(&cfg.ToolInputFromStdin, "json-stdin", false, "")
	fs.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "")
	if err := fs.Parse(flagArgs); err != nil {
		return cliConfig{}, err
	}
	if cfg.Timeout <= 0 {
		return cliConfig{}, errors.New("--timeout must be greater than 0")
	}
	if cfg.ToolInputFromStdin && cfg.ToolInputJSON != "" {
		return cliConfig{}, errors.New("tool call accepts exactly one of --json or --json-stdin")
	}
	if !cfg.ToolInputFromStdin && cfg.ToolInputJSON == "" {
		return cliConfig{}, errors.New("tool call requires exactly one of --json or --json-stdin")
	}
	if fs.NArg() != 0 {
		return cliConfig{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

func parseToolInspectFlags(name string, args []string) (cliConfig, error) {
	if containsHelpFlag(args) {
		return cliConfig{}, errUsageRequested
	}
	toolName, flagArgs, err := extractPositionalArg(args, toolInspectFlagTakesValue)
	if err != nil {
		return cliConfig{}, err
	}

	fs := newFlagSet(name)
	cfg := cliConfig{Command: commandToolInspect, Timeout: 30 * time.Second, ToolName: toolName}
	fs.StringVar(&cfg.XcodePID, "xcode-pid", "", "")
	fs.StringVar(&cfg.SessionID, "session-id", "", "")
	fs.BoolVar(&cfg.Debug, "debug", false, "")
	fs.BoolVar(&cfg.JSONOutput, "json", false, "")
	if err := fs.Parse(flagArgs); err != nil {
		return cliConfig{}, err
	}
	if fs.NArg() != 0 {
		return cliConfig{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

func parseAgentStatusFlags(name string, args []string) (cliConfig, error) {
	fs := newFlagSet(name)
	cfg := cliConfig{}
	help := false
	fs.BoolVar(&cfg.JSONOutput, "json", false, "")
	fs.BoolVar(&help, "h", false, "")
	fs.BoolVar(&help, "help", false, "")
	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	if help {
		return cliConfig{}, errUsageRequested
	}
	if fs.NArg() != 0 {
		return cliConfig{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

func parseAgentGuideFlags(name string, args []string) (cliConfig, error) {
	if containsHelpFlag(args) {
		return cliConfig{}, errUsageRequested
	}
	intent, flagArgs, err := extractOptionalPositionalArg(args, agentGuideFlagTakesValue)
	if err != nil {
		return cliConfig{}, err
	}

	fs := newFlagSet(name)
	cfg := cliConfig{Command: commandAgentGuide, Timeout: 30 * time.Second, Intent: intent}
	fs.StringVar(&cfg.XcodePID, "xcode-pid", "", "")
	fs.StringVar(&cfg.SessionID, "session-id", "", "")
	fs.BoolVar(&cfg.Debug, "debug", false, "")
	fs.BoolVar(&cfg.JSONOutput, "json", false, "")
	fs.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "")
	if err := fs.Parse(flagArgs); err != nil {
		return cliConfig{}, err
	}
	if cfg.Timeout <= 0 {
		return cliConfig{}, errors.New("--timeout must be greater than 0")
	}
	if fs.NArg() != 0 {
		return cliConfig{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

func parseAgentDemoFlags(name string, args []string) (cliConfig, error) {
	fs := newFlagSet(name)
	cfg := cliConfig{Command: commandAgentDemo, Timeout: 30 * time.Second}
	help := false
	fs.StringVar(&cfg.XcodePID, "xcode-pid", "", "")
	fs.StringVar(&cfg.SessionID, "session-id", "", "")
	fs.BoolVar(&cfg.Debug, "debug", false, "")
	fs.BoolVar(&cfg.JSONOutput, "json", false, "")
	fs.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "")
	fs.BoolVar(&help, "h", false, "")
	fs.BoolVar(&help, "help", false, "")
	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	if help {
		return cliConfig{}, errUsageRequested
	}
	if cfg.Timeout <= 0 {
		return cliConfig{}, errors.New("--timeout must be greater than 0")
	}
	if fs.NArg() != 0 {
		return cliConfig{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

func parseAgentSimpleFlags(name string, args []string) (cliConfig, error) {
	fs := newFlagSet(name)
	cfg := cliConfig{}
	help := false
	fs.BoolVar(&help, "h", false, "")
	fs.BoolVar(&help, "help", false, "")
	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	if help {
		return cliConfig{}, errUsageRequested
	}
	if fs.NArg() != 0 {
		return cliConfig{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

func parseAgentRunFlags(name string, args []string) (cliConfig, error) {
	fs := newFlagSet(name)
	cfg := cliConfig{IdleTimeout: agent.DefaultIdleTimeout}
	help := false
	fs.BoolVar(&cfg.LaunchAgent, "launch-agent", false, "")
	fs.BoolVar(&cfg.Debug, "debug", false, "")
	fs.DurationVar(&cfg.IdleTimeout, "idle-timeout", agent.DefaultIdleTimeout, "")
	fs.BoolVar(&help, "h", false, "")
	fs.BoolVar(&help, "help", false, "")
	if err := fs.Parse(args); err != nil {
		return cliConfig{}, err
	}
	if help {
		return cliConfig{}, errUsageRequested
	}
	if !cfg.LaunchAgent {
		return cliConfig{}, errors.New("agent run requires --launch-agent")
	}
	if cfg.IdleTimeout <= 0 {
		return cliConfig{}, errors.New("--idle-timeout must be greater than 0")
	}
	if fs.NArg() != 0 {
		return cliConfig{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

func extractPositionalArg(args []string, takesValue func(string) bool) (string, []string, error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if takesValue(arg) {
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("flag needs an argument: %s", arg)
				}
				i++
			}
			continue
		}
		flagArgs := append([]string{}, args[:i]...)
		flagArgs = append(flagArgs, args[i+1:]...)
		return arg, flagArgs, nil
	}
	return "", nil, errors.New("missing required name")
}

func extractOptionalPositionalArg(args []string, takesValue func(string) bool) (string, []string, error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if takesValue(arg) {
				if i+1 >= len(args) {
					return "", nil, fmt.Errorf("flag needs an argument: %s", arg)
				}
				i++
			}
			continue
		}
		flagArgs := append([]string{}, args[:i]...)
		flagArgs = append(flagArgs, args[i+1:]...)
		return arg, flagArgs, nil
	}
	return "", append([]string{}, args...), nil
}

func containsHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func agentGuideFlagTakesValue(flagName string) bool {
	switch flagName {
	case "--xcode-pid", "--session-id", "--timeout":
		return true
	default:
		return false
	}
}

func toolCallFlagTakesValue(flagName string) bool {
	switch flagName {
	case "--xcode-pid", "--session-id", "--json", "--timeout":
		return true
	default:
		return false
	}
}

func toolInspectFlagTakesValue(flagName string) bool {
	switch flagName {
	case "--xcode-pid", "--session-id":
		return true
	default:
		return false
	}
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func rootUsage() string {
	return `xcodecli wraps xcrun mcpbridge for local macOS use.

START HERE:
  For humans:
    1. xcodecli agent guide "build Unicody"
    2. xcodecli agent demo
    3. xcodecli doctor --json
    4. xcodecli tools list
    5. xcodecli tool call XcodeListWindows --json '{}'

  For agents:
    - Start with a workflow tutor via "xcodecli agent guide <intent> --json".
    - Run a safe live onboarding demo with "xcodecli agent demo --json".
    - Discover command shapes with "xcodecli help <command>".
    - Discover runtime health with "xcodecli doctor --json".
    - Discover LaunchAgent state with "xcodecli agent status --json".
    - Discover available tools with "xcodecli tools list --json".
    - Discover per-tool schema with "xcodecli tool inspect <name> --json".

RUNTIME MODEL:
  - "bridge" is raw passthrough to xcrun mcpbridge.
  - "tools" and "tool" use a per-user LaunchAgent and local Unix socket RPC.
  - The first tools request may install/bootstrap the LaunchAgent automatically.
  - Xcode should be running, with at least one workspace/project window open.

USAGE:
  xcodecli [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodecli bridge [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodecli doctor [--json] [--xcode-pid PID] [--session-id UUID]
  xcodecli tools list [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodecli tool inspect <name> [--json] [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodecli tool call <name> (--json '{...}' | --json @payload.json | --json-stdin) [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodecli agent guide [<intent>] [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodecli agent demo [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodecli agent status [--json]
  xcodecli agent stop
  xcodecli agent uninstall

COMMANDS:
  bridge    Run raw STDIO passthrough to xcrun mcpbridge
  doctor    Run environment diagnostics
  tools     Convenience commands for listing tools
  tool      Convenience commands for inspecting or calling a tool
  agent     Inspect or manage the LaunchAgent used by tools commands

Use "xcodecli help <command>" for command-specific help.
`
}

func bridgeUsage() string {
	return `bridge sends stdin/stdout/stderr directly to xcrun mcpbridge.
Use this when you already have an MCP-aware client and need raw transport.

USAGE:
  xcodecli [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodecli bridge [--xcode-pid PID] [--session-id UUID] [--debug]

FLAGS:
  --xcode-pid PID     Override MCP_XCODE_PID
  --session-id UUID   Override MCP_XCODE_SESSION_ID
  --debug             Emit wrapper debug logs to stderr
  -h, --help          Show help
`
}

func doctorUsage() string {
	return `doctor reports environment readiness for both humans and agents.
Prefer --json when another tool or agent needs to parse the result.

USAGE:
  xcodecli doctor [--json] [--xcode-pid PID] [--session-id UUID]

FLAGS:
  --json              Print the diagnostic report as pretty JSON
  --xcode-pid PID     Diagnose the effective MCP_XCODE_PID value
  --session-id UUID   Diagnose the effective MCP_XCODE_SESSION_ID value
  -h, --help          Show help
`
}

func toolsUsage() string {
	return `Use tools list to discover the names exposed by Xcode through MCP.
Inspect a tool before calling it if you need its schema or description.

USAGE:
  xcodecli tools list [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

SUBCOMMANDS:
  list      List MCP tools exposed through xcrun mcpbridge via the LaunchAgent
`
}

func toolsListUsage() string {
	return `tools list discovers the current MCP tool catalog from Xcode.
This is the primary entrypoint for both humans and agents to learn what is available.

USAGE:
  xcodecli tools list [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

FLAGS:
  --json               Print the flattened tools array as pretty JSON
  --timeout DURATION   Fail if the MCP request does not finish in time (default 30s)
  --xcode-pid PID      Override MCP_XCODE_PID
  --session-id UUID    Override MCP_XCODE_SESSION_ID
  --debug              Emit convenience-command debug logs to stderr
  -h, --help           Show help

NOTES:
  The first tools request may automatically install and bootstrap a per-user LaunchAgent.
`
}

func toolUsage() string {
	return `Use tool inspect to learn what a tool expects, then tool call to execute it.
Agents should usually inspect before calling unless they already cached the schema.

USAGE:
  xcodecli tool inspect <name> [--json] [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodecli tool call <name> (--json '{...}' | --json @payload.json | --json-stdin) [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

SUBCOMMANDS:
  inspect   Show tool description and input schema
  call      Call a single MCP tool with JSON object arguments
`
}

func toolInspectUsage() string {
	return `tool inspect shows one tool's description and input schema.
Use --json for machine-readable metadata or plain text for quick inspection.

USAGE:
  xcodecli tool inspect <name> [--json] [--xcode-pid PID] [--session-id UUID] [--debug]

FLAGS:
  --json               Print the raw tool object as pretty JSON
  --xcode-pid PID      Override MCP_XCODE_PID
  --session-id UUID    Override MCP_XCODE_SESSION_ID
  --debug              Emit convenience-command debug logs to stderr
  -h, --help           Show help
`
}

func toolCallUsage() string {
	return `tool call executes one MCP tool with a JSON object payload.
For large payloads prefer --json @file or --json-stdin instead of a long inline string.

USAGE:
  xcodecli tool call <name> (--json '{...}' | --json @payload.json | --json-stdin) [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

FLAGS:
  --json PAYLOAD       JSON object passed as tools/call arguments, or @path to load a JSON file
  --json-stdin         Read the JSON object payload from stdin
  --timeout DURATION   Fail if the MCP request does not finish in time (default 30s)
  --xcode-pid PID      Override MCP_XCODE_PID
  --session-id UUID    Override MCP_XCODE_SESSION_ID
  --debug              Emit convenience-command debug logs to stderr
  -h, --help           Show help

NOTES:
  The first tools request may automatically install and bootstrap a per-user LaunchAgent.
`
}

func agentUsage() string {
	return `The agent subcommands inspect or manage the LaunchAgent used by tools commands.
Use guide to learn the right workflow for a request, demo for a safe read-only onboarding flow, status for diagnostics, stop to end the running process, and uninstall to remove local LaunchAgent state.

USAGE:
  xcodecli agent guide [<intent>] [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodecli agent demo [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodecli agent status [--json]
  xcodecli agent stop
  xcodecli agent uninstall

SUBCOMMANDS:
  guide        Explain the recommended tool workflow for a request
  demo         Run a safe read-only onboarding demo
  status       Show LaunchAgent installation and runtime state
  stop         Ask the running LaunchAgent process to stop
  uninstall    Remove the LaunchAgent plist and local agent runtime files
`
}

func agentGuideUsage() string {
	return `agent guide explains the recommended xcodecli workflow for a request without executing mutating tools.
It gathers lightweight live context, matches your intent to a workflow family, and prints exact next commands.

USAGE:
  xcodecli agent guide [<intent>] [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

FLAGS:
  --json               Print the full guide report as pretty JSON
  --timeout DURATION   Fail live discovery steps if they do not finish in time (default 30s)
  --xcode-pid PID      Override MCP_XCODE_PID
  --session-id UUID    Override MCP_XCODE_SESSION_ID
  --debug              Emit convenience-command debug logs to stderr
  -h, --help           Show help

NOTES:
  This command is read-only. It may run doctor, tools list, agent status, and XcodeListWindows for context.
`
}

func agentDemoUsage() string {
	return `agent demo runs a safe read-only onboarding flow for first-time humans and agents.
It reuses doctor output, discovers the live MCP tool catalog, and safely calls XcodeListWindows.

USAGE:
  xcodecli agent demo [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

FLAGS:
  --json               Print the full demo report as pretty JSON
  --timeout DURATION   Fail MCP discovery/call steps if they do not finish in time (default 30s)
  --xcode-pid PID      Override MCP_XCODE_PID
  --session-id UUID    Override MCP_XCODE_SESSION_ID
  --debug              Emit convenience-command debug logs to stderr
  -h, --help           Show help

NOTES:
  This command is non-mutating. It only runs doctor, tools list, agent status, and XcodeListWindows.
`
}

func agentStatusUsage() string {
	return `agent status reports LaunchAgent installation, socket reachability, and backend session state.
Prefer --json when another agent or script needs to consume the result.

USAGE:
  xcodecli agent status [--json]
`
}

func agentStopUsage() string {
	return `agent stop asks the running LaunchAgent process to exit if it is currently alive.

USAGE:
  xcodecli agent stop
`
}

func agentUninstallUsage() string {
	return `agent uninstall removes the LaunchAgent plist and local runtime files.
Use this if the LaunchAgent is stale or you want to reset local state.

USAGE:
  xcodecli agent uninstall
`
}

func agentRunUsage() string {
	return `agent run is an internal entrypoint used by the LaunchAgent plist.
Most users and agents should not call it directly.

USAGE:
  xcodecli agent run --launch-agent [--idle-timeout 10m] [--debug]

FLAGS:
  --launch-agent       Required internal flag used by the LaunchAgent plist
  --idle-timeout       Shut down after this much idle time (default 10m)
  --debug              Emit agent runtime debug logs to stderr/log file
  -h, --help           Show help
`
}
