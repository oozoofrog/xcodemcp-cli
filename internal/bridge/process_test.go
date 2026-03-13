package bridge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestRunPassthrough(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	result, err := Run(context.Background(), Config{
		Command: helperCommand("echo"),
		Env:     helperEnv(),
		In:      strings.NewReader("hello"),
		Out:     &stdout,
		ErrOut:  &stderr,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if stdout.String() != "OUT:hello" {
		t.Fatalf("stdout = %q, want OUT:hello", stdout.String())
	}
	if stderr.String() != "ERR:hello" {
		t.Fatalf("stderr = %q, want ERR:hello", stderr.String())
	}
}

func TestRunKeepsStreamingAfterParentEOF(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	result, err := Run(context.Background(), Config{
		Command: helperCommand("after-eof"),
		Env:     helperEnv(),
		In:      strings.NewReader("payload"),
		Out:     &stdout,
		ErrOut:  &stderr,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if stdout.String() != "after-out" {
		t.Fatalf("stdout = %q, want after-out", stdout.String())
	}
	if stderr.String() != "after-err" {
		t.Fatalf("stderr = %q, want after-err", stderr.String())
	}
}

func TestRunPropagatesExitCode(t *testing.T) {
	result, err := Run(context.Background(), Config{
		Command: helperCommand("exit", "7"),
		Env:     helperEnv(),
		In:      strings.NewReader(""),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.ExitCode)
	}
}

func TestRunForwardsSignals(t *testing.T) {
	stdout := newWatchBuffer("ready\n")
	var stderr bytes.Buffer
	started := make(chan int, 1)
	sigCh := make(chan os.Signal, 1)

	var (
		result Result
		runErr error
	)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, runErr = Run(context.Background(), Config{
			Command:      helperCommand("signal"),
			Env:          helperEnv(),
			In:           strings.NewReader(""),
			Out:          stdout,
			ErrOut:       &stderr,
			SignalSource: sigCh,
			OnStart: func(pid int) {
				started <- pid
			},
		})
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("child process did not start")
	}

	select {
	case <-stdout.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("child process did not become ready")
	}

	sigCh <- syscall.SIGTERM
	wg.Wait()

	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(stdout.String(), "signal:") {
		t.Fatalf("stdout = %q, want forwarded signal marker", stdout.String())
	}
	if !strings.Contains(stderr.String(), "signal:") {
		t.Fatalf("stderr = %q, want forwarded signal marker", stderr.String())
	}
}

type watchBuffer struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	target string
	ready  chan struct{}
	once   sync.Once
}

func newWatchBuffer(target string) *watchBuffer {
	return &watchBuffer{target: target, ready: make(chan struct{})}
}

func (w *watchBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	if strings.Contains(w.buf.String(), w.target) {
		w.once.Do(func() { close(w.ready) })
	}
	return n, err
}

func (w *watchBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func helperCommand(mode string, extra ...string) Command {
	args := []string{"-test.run=TestHelperProcess", "--", mode}
	args = append(args, extra...)
	return Command{Path: os.Args[0], Args: args}
}

func helperEnv() []string {
	return append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	idx := -1
	for i, arg := range os.Args {
		if arg == "--" {
			idx = i
			break
		}
	}
	if idx == -1 || idx+1 >= len(os.Args) {
		fmt.Fprint(os.Stderr, "missing helper mode")
		os.Exit(2)
	}

	mode := os.Args[idx+1]
	args := os.Args[idx+2:]

	switch mode {
	case "echo":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read stdin: %v", err)
			os.Exit(2)
		}
		fmt.Fprintf(os.Stdout, "OUT:%s", data)
		fmt.Fprintf(os.Stderr, "ERR:%s", data)
		os.Exit(0)
	case "after-eof":
		_, _ = io.Copy(io.Discard, os.Stdin)
		time.Sleep(50 * time.Millisecond)
		fmt.Fprint(os.Stdout, "after-out")
		fmt.Fprint(os.Stderr, "after-err")
		os.Exit(0)
	case "exit":
		code, _ := strconv.Atoi(args[0])
		os.Exit(code)
	case "signal":
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		fmt.Fprint(os.Stdout, "ready\n")
		sig := <-sigCh
		fmt.Fprintf(os.Stdout, "signal:%s\n", sig)
		fmt.Fprintf(os.Stderr, "signal:%s\n", sig)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q", mode)
		os.Exit(2)
	}
}
