package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/oozoofrog/xcodemcp-cli/internal/agent"
)

type commandName string

const (
	commandBridge         commandName = "bridge"
	commandDoctor         commandName = "doctor"
	commandToolsList      commandName = "tools-list"
	commandToolCall       commandName = "tool-call"
	commandToolInspect    commandName = "tool-inspect"
	commandAgentStatus    commandName = "agent-status"
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
	ToolInputJSON      string
	ToolInputFromStdin bool
	LaunchAgent        bool
}

var errUsageRequested = errors.New("usage requested")

func parseCLI(args []string) (cliConfig, string, error) {
	if len(args) == 0 {
		cfg, err := parseBridgeFlags("xcodemcp", args)
		return cfg, rootUsage(), err
	}

	switch args[0] {
	case "help", "-h", "--help":
		return parseHelp(args[1:])
	case string(commandBridge):
		cfg, err := parseBridgeFlags("xcodemcp bridge", args[1:])
		cfg.Command = commandBridge
		return cfg, bridgeUsage(), err
	case string(commandDoctor):
		cfg, err := parseDoctorFlags("xcodemcp doctor", args[1:])
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
			cfg, err := parseBridgeFlags("xcodemcp", args)
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
		cfg, err := parseToolsListFlags("xcodemcp tools list", args[1:])
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
		cfg, err := parseToolCallFlags("xcodemcp tool call", args[1:])
		cfg.Command = commandToolCall
		return cfg, toolCallUsage(), err
	case "inspect":
		cfg, err := parseToolInspectFlags("xcodemcp tool inspect", args[1:])
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
	case "status":
		cfg, err := parseAgentStatusFlags("xcodemcp agent status", args[1:])
		cfg.Command = commandAgentStatus
		return cfg, agentStatusUsage(), err
	case "stop":
		cfg, err := parseAgentSimpleFlags("xcodemcp agent stop", args[1:])
		cfg.Command = commandAgentStop
		return cfg, agentStopUsage(), err
	case "uninstall":
		cfg, err := parseAgentSimpleFlags("xcodemcp agent uninstall", args[1:])
		cfg.Command = commandAgentUninstall
		return cfg, agentUninstallUsage(), err
	case "run":
		cfg, err := parseAgentRunFlags("xcodemcp agent run", args[1:])
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

func containsHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
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
	return `xcodemcp wraps xcrun mcpbridge for local macOS use.

USAGE:
  xcodemcp [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodemcp bridge [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodemcp doctor [--json] [--xcode-pid PID] [--session-id UUID]
  xcodemcp tools list [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodemcp tool inspect <name> [--json] [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodemcp tool call <name> (--json '{...}' | --json @payload.json | --json-stdin) [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodemcp agent status [--json]
  xcodemcp agent stop
  xcodemcp agent uninstall

COMMANDS:
  bridge    Run raw STDIO passthrough to xcrun mcpbridge (default)
  doctor    Run environment diagnostics
  tools     Convenience commands for listing tools
  tool      Convenience commands for inspecting or calling a tool
  agent     Inspect or manage the LaunchAgent used by tools commands

Use "xcodemcp help <command>" for command-specific help.
`
}

func bridgeUsage() string {
	return `USAGE:
  xcodemcp [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodemcp bridge [--xcode-pid PID] [--session-id UUID] [--debug]

FLAGS:
  --xcode-pid PID     Override MCP_XCODE_PID
  --session-id UUID   Override MCP_XCODE_SESSION_ID
  --debug             Emit wrapper debug logs to stderr
  -h, --help          Show help
`
}

func doctorUsage() string {
	return `USAGE:
  xcodemcp doctor [--json] [--xcode-pid PID] [--session-id UUID]

FLAGS:
  --json              Print the diagnostic report as pretty JSON
  --xcode-pid PID     Diagnose the effective MCP_XCODE_PID value
  --session-id UUID   Diagnose the effective MCP_XCODE_SESSION_ID value
  -h, --help          Show help
`
}

func toolsUsage() string {
	return `USAGE:
  xcodemcp tools list [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

SUBCOMMANDS:
  list      List MCP tools exposed through xcrun mcpbridge via the LaunchAgent
`
}

func toolsListUsage() string {
	return `USAGE:
  xcodemcp tools list [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

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
	return `USAGE:
  xcodemcp tool inspect <name> [--json] [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodemcp tool call <name> (--json '{...}' | --json @payload.json | --json-stdin) [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

SUBCOMMANDS:
  inspect   Show tool description and input schema
  call      Call a single MCP tool with JSON object arguments
`
}

func toolInspectUsage() string {
	return `USAGE:
  xcodemcp tool inspect <name> [--json] [--xcode-pid PID] [--session-id UUID] [--debug]

FLAGS:
  --json               Print the raw tool object as pretty JSON
  --xcode-pid PID      Override MCP_XCODE_PID
  --session-id UUID    Override MCP_XCODE_SESSION_ID
  --debug              Emit convenience-command debug logs to stderr
  -h, --help           Show help
`
}

func toolCallUsage() string {
	return `USAGE:
  xcodemcp tool call <name> (--json '{...}' | --json @payload.json | --json-stdin) [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

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
	return `USAGE:
  xcodemcp agent status [--json]
  xcodemcp agent stop
  xcodemcp agent uninstall

SUBCOMMANDS:
  status       Show LaunchAgent installation and runtime state
  stop         Ask the running LaunchAgent process to stop
  uninstall    Remove the LaunchAgent plist and local agent runtime files
`
}

func agentStatusUsage() string {
	return `USAGE:
  xcodemcp agent status [--json]
`
}

func agentStopUsage() string {
	return `USAGE:
  xcodemcp agent stop
`
}

func agentUninstallUsage() string {
	return `USAGE:
  xcodemcp agent uninstall
`
}

func agentRunUsage() string {
	return `USAGE:
  xcodemcp agent run --launch-agent [--idle-timeout 10m] [--debug]

FLAGS:
  --launch-agent       Required internal flag used by the LaunchAgent plist
  --idle-timeout       Shut down after this much idle time (default 10m)
  --debug              Emit agent runtime debug logs to stderr/log file
  -h, --help           Show help
`
}
