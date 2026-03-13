package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

type commandName string

const (
	commandBridge commandName = "bridge"
	commandDoctor commandName = "doctor"
)

type cliConfig struct {
	Command   commandName
	XcodePID  string
	SessionID string
	Debug     bool
}

var errUsageRequested = errors.New("usage requested")

func parseCLI(args []string) (cliConfig, string, error) {
	if len(args) == 0 {
		cfg, err := parseBridgeFlags("xcodemcp", args)
		return cfg, rootUsage(), err
	}

	switch args[0] {
	case "help", "-h", "--help":
		if len(args) == 1 {
			return cliConfig{}, rootUsage(), errUsageRequested
		}
		switch args[1] {
		case string(commandBridge):
			return cliConfig{}, bridgeUsage(), errUsageRequested
		case string(commandDoctor):
			return cliConfig{}, doctorUsage(), errUsageRequested
		default:
			return cliConfig{}, rootUsage(), fmt.Errorf("unknown help topic: %s", args[1])
		}
	case string(commandBridge):
		cfg, err := parseBridgeFlags("xcodemcp bridge", args[1:])
		cfg.Command = commandBridge
		return cfg, bridgeUsage(), err
	case string(commandDoctor):
		cfg, err := parseDoctorFlags("xcodemcp doctor", args[1:])
		cfg.Command = commandDoctor
		return cfg, doctorUsage(), err
	default:
		if strings.HasPrefix(args[0], "-") {
			cfg, err := parseBridgeFlags("xcodemcp", args)
			cfg.Command = commandBridge
			return cfg, bridgeUsage(), err
		}
		return cliConfig{}, rootUsage(), fmt.Errorf("unknown command: %s", args[0])
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

COMMANDS:
  bridge    Run raw STDIO passthrough to xcrun mcpbridge (default)
  doctor    Run environment diagnostics

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
