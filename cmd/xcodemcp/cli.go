package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type commandName string

const (
	commandBridge    commandName = "bridge"
	commandDoctor    commandName = "doctor"
	commandToolsList commandName = "tools-list"
	commandToolCall  commandName = "tool-call"
)

type cliConfig struct {
	Command       commandName
	XcodePID      string
	SessionID     string
	Debug         bool
	JSONOutput    bool
	Timeout       time.Duration
	ToolName      string
	ToolInputJSON string
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
		if len(args) == 2 && args[1] == "call" {
			return cliConfig{}, toolCallUsage(), errUsageRequested
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
	default:
		return cliConfig{}, toolUsage(), fmt.Errorf("unknown tool subcommand: %s", args[0])
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
	toolName, flagArgs, err := extractToolCallArgs(args)
	if err != nil {
		return cliConfig{}, err
	}

	fs := newFlagSet(name)
	cfg := cliConfig{Command: commandToolCall, Timeout: 30 * time.Second, ToolName: toolName}
	help := false
	fs.StringVar(&cfg.XcodePID, "xcode-pid", "", "")
	fs.StringVar(&cfg.SessionID, "session-id", "", "")
	fs.BoolVar(&cfg.Debug, "debug", false, "")
	fs.StringVar(&cfg.ToolInputJSON, "json", "", "")
	fs.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "")
	fs.BoolVar(&help, "h", false, "")
	fs.BoolVar(&help, "help", false, "")
	if err := fs.Parse(flagArgs); err != nil {
		return cliConfig{}, err
	}
	if help {
		return cliConfig{}, errUsageRequested
	}
	if cfg.Timeout <= 0 {
		return cliConfig{}, errors.New("--timeout must be greater than 0")
	}
	if cfg.ToolInputJSON == "" {
		return cliConfig{}, errors.New("tool call requires --json with an object payload")
	}
	if fs.NArg() != 0 {
		return cliConfig{}, fmt.Errorf("unexpected positional arguments: %s", strings.Join(fs.Args(), " "))
	}
	return cfg, nil
}

func extractToolCallArgs(args []string) (string, []string, error) {
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
	return "", nil, errors.New("tool call requires a tool name")
}

func takesValue(flagName string) bool {
	switch flagName {
	case "--xcode-pid", "--session-id", "--json", "--timeout":
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
  xcodemcp doctor [--xcode-pid PID] [--session-id UUID]
  xcodemcp tools list [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]
  xcodemcp tool call <name> --json '{...}' [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

COMMANDS:
  bridge    Run raw STDIO passthrough to xcrun mcpbridge (default)
  doctor    Run environment diagnostics
  tools     Convenience commands for listing tools
  tool      Convenience commands for calling a tool

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
  xcodemcp doctor [--xcode-pid PID] [--session-id UUID]

FLAGS:
  --xcode-pid PID     Diagnose the effective MCP_XCODE_PID value
  --session-id UUID   Diagnose the effective MCP_XCODE_SESSION_ID value
  -h, --help          Show help
`
}

func toolsUsage() string {
	return `USAGE:
  xcodemcp tools list [--json] [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

SUBCOMMANDS:
  list      List MCP tools exposed through xcrun mcpbridge
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
`
}

func toolUsage() string {
	return `USAGE:
  xcodemcp tool call <name> --json '{...}' [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

SUBCOMMANDS:
  call      Call a single MCP tool with JSON object arguments
`
}

func toolCallUsage() string {
	return `USAGE:
  xcodemcp tool call <name> --json '{...}' [--timeout 30s] [--xcode-pid PID] [--session-id UUID] [--debug]

FLAGS:
  --json PAYLOAD       JSON object passed as tools/call arguments (required)
  --timeout DURATION   Fail if the MCP request does not finish in time (default 30s)
  --xcode-pid PID      Override MCP_XCODE_PID
  --session-id UUID    Override MCP_XCODE_SESSION_ID
  --debug              Emit convenience-command debug logs to stderr
  -h, --help           Show help
`
}
