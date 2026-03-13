package bridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

type Command struct {
	Path string
	Args []string
}

type Config struct {
	Command      Command
	Env          []string
	In           io.Reader
	Out          io.Writer
	ErrOut       io.Writer
	Debug        bool
	SignalSource <-chan os.Signal
	OnStart      func(pid int)
}

type Result struct {
	ExitCode int
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.Command.Path == "" {
		return Result{}, errors.New("missing command path")
	}

	path, err := resolveCommandPath(cfg.Command.Path)
	if err != nil {
		return Result{}, err
	}

	stdin := cfg.In
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	stdout := cfg.Out
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := cfg.ErrOut
	if stderr == nil {
		stderr = io.Discard
	}

	cmd := exec.CommandContext(ctx, path, cfg.Command.Args...)
	cmd.Env = cfg.Env

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return Result{}, fmt.Errorf("create stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, fmt.Errorf("create stderr pipe: %w", err)
	}

	if cfg.Debug {
		fmt.Fprintf(stderr, "[debug] starting %s %s\n", path, strings.Join(cfg.Command.Args, " "))
	}

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start child process: %w", err)
	}
	if cfg.OnStart != nil {
		cfg.OnStart(cmd.Process.Pid)
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stdinPipe, stdin)
		_ = stdinPipe.Close()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stdout, stdoutPipe)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stderr, stderrPipe)
	}()

	done := make(chan struct{})
	cleanupSignals := startSignalForwarding(cmd, cfg, stderr, done)

	waitErr := cmd.Wait()
	close(done)
	cleanupSignals()
	wg.Wait()

	if cfg.Debug {
		fmt.Fprintf(stderr, "[debug] child exited\n")
	}

	if waitErr == nil {
		return Result{ExitCode: 0}, nil
	}

	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return Result{ExitCode: exitErr.ExitCode()}, nil
	}

	return Result{}, fmt.Errorf("wait for child process: %w", waitErr)
}

func startSignalForwarding(cmd *exec.Cmd, cfg Config, stderr io.Writer, done <-chan struct{}) func() {
	var sigCh <-chan os.Signal
	var stop func()
	if cfg.SignalSource != nil {
		sigCh = cfg.SignalSource
		stop = func() {}
	} else {
		localCh := make(chan os.Signal, 4)
		signal.Notify(localCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		sigCh = localCh
		stop = func() {
			signal.Stop(localCh)
			close(localCh)
		}
	}

	go func() {
		for {
			select {
			case <-done:
				return
			case sig, ok := <-sigCh:
				if !ok {
					return
				}
				if cfg.Debug {
					fmt.Fprintf(stderr, "[debug] forwarding signal %s to child\n", sig)
				}
				if cmd.Process != nil {
					_ = cmd.Process.Signal(sig)
				}
			}
		}
	}()

	return stop
}

func resolveCommandPath(path string) (string, error) {
	if strings.Contains(path, "/") {
		return path, nil
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("resolve command %q: %w", path, err)
	}
	return resolved, nil
}
