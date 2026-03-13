package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/oozoofrog/xcodemcp-cli/internal/bridge"
	"github.com/oozoofrog/xcodemcp-cli/internal/doctor"
)

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
		fmt.Fprintf(stderr, "xcodemcp: %v\n", err)
		if usage != "" {
			fmt.Fprint(stderr, usage)
		}
		return 1
	}

	if runtime.GOOS != "darwin" {
		fmt.Fprintln(stderr, "xcodemcp: only macOS (darwin) is supported")
		return 1
	}

	effective := bridge.EffectiveOptions(env, bridge.EnvOptions{
		XcodePID:  cfg.XcodePID,
		SessionID: cfg.SessionID,
	})

	switch cfg.Command {
	case commandDoctor:
		report := doctor.NewInspector().Run(ctx, doctor.Options{
			BaseEnv:   env,
			XcodePID:  effective.XcodePID,
			SessionID: effective.SessionID,
		})
		fmt.Fprint(stdout, report.String())
		if report.Success() {
			return 0
		}
		return 1
	case commandBridge:
		if err := bridge.ValidateEnvOptions(effective); err != nil {
			fmt.Fprintf(stderr, "xcodemcp: invalid bridge options: %v\n", err)
			return 1
		}

		result, err := bridge.Run(ctx, bridge.Config{
			Command: bridge.Command{
				Path: "xcrun",
				Args: []string{"mcpbridge"},
			},
			Env:    bridge.ApplyEnvOverrides(env, effective),
			In:     stdin,
			Out:    stdout,
			ErrOut: stderr,
			Debug:  cfg.Debug,
		})
		if err != nil {
			fmt.Fprintf(stderr, "xcodemcp: %v\n", err)
			return 1
		}
		return result.ExitCode
	default:
		fmt.Fprintf(stderr, "xcodemcp: unsupported command %q\n", cfg.Command)
		return 1
	}
}
