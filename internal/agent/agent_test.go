package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oozoofrog/xcodecli/internal/mcp"
)

func TestListToolsAutoInstallsLaunchAgentAndReusesBackendSession(t *testing.T) {
	tempDir, paths := newShortPaths(t)
	spawnFile := filepath.Join(tempDir, "spawn.log")
	serverCfg := testServerConfig(t, paths, spawnFile, 5*time.Second)
	harness := newServerHarness(t, serverCfg)
	launchd := &fakeLaunchd{harness: harness}
	clientCfg := testClientConfig(paths, spawnFile, 5*time.Second, launchd)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tools1, err := ListTools(ctx, clientCfg, Request{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("ListTools first call returned error: %v", err)
	}
	tools2, err := ListTools(ctx, clientCfg, Request{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("ListTools second call returned error: %v", err)
	}
	if len(tools1) != 1 || len(tools2) != 1 {
		t.Fatalf("unexpected tools lengths: %d %d", len(tools1), len(tools2))
	}

	status, err := StatusInfo(ctx, clientCfg)
	if err != nil {
		t.Fatalf("StatusInfo returned error: %v", err)
	}
	if !status.PlistInstalled || !status.SocketReachable || !status.Running {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.BackendSessions != 1 {
		t.Fatalf("BackendSessions = %d, want 1", status.BackendSessions)
	}
	if !status.BinaryPathMatches {
		t.Fatalf("expected binary path to match: %+v", status)
	}

	count := helperSpawnCount(t, spawnFile)
	if count != 1 {
		t.Fatalf("backend helper spawn count = %d, want 1", count)
	}
}

func TestLaunchAgentStopsAfterIdleTimeout(t *testing.T) {
	tempDir, paths := newShortPaths(t)
	spawnFile := filepath.Join(tempDir, "spawn.log")
	serverCfg := testServerConfig(t, paths, spawnFile, 250*time.Millisecond)
	harness := newServerHarness(t, serverCfg)
	launchd := &fakeLaunchd{harness: harness}
	clientCfg := testClientConfig(paths, spawnFile, 250*time.Millisecond, launchd)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := ListTools(ctx, clientCfg, Request{Timeout: 2 * time.Second}); err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}
	time.Sleep(700 * time.Millisecond)
	status, err := StatusInfo(context.Background(), clientCfg)
	if err != nil {
		t.Fatalf("StatusInfo returned error: %v", err)
	}
	if status.SocketReachable {
		t.Fatalf("agent did not stop after idle timeout: %+v", status)
	}
}

func TestListToolsAutostartHonorsCallerTimeout(t *testing.T) {
	_, paths := newShortPaths(t)
	clientCfg := Config{
		Paths:          paths,
		Label:          LaunchAgentLabel,
		IdleTimeout:    5 * time.Second,
		ErrOut:         io.Discard,
		Launchd:        blockingLaunchd{},
		ExecutablePath: func() (string, error) { return "/tmp/xcodecli-test", nil },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := ListTools(ctx, clientCfg, Request{Timeout: 5 * time.Second})
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected caller timeout error, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("autostart timeout took too long: %s", elapsed)
	}
}

func TestListToolsBackendInitializationHonorsRequestTimeout(t *testing.T) {
	tempDir, paths := newShortPaths(t)
	spawnFile := filepath.Join(tempDir, "spawn.log")
	serverCfg := testServerConfig(t, paths, spawnFile, 5*time.Second)
	serverCfg.Command = helperCommand("timeout-init")
	harness := newServerHarness(t, serverCfg)
	launchd := &fakeLaunchd{harness: harness}
	clientCfg := testClientConfig(paths, spawnFile, 5*time.Second, launchd)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := time.Now()
	_, err := ListTools(ctx, clientCfg, Request{Timeout: 200 * time.Millisecond})
	if err == nil || (!strings.Contains(err.Error(), "timed out") && !strings.Contains(err.Error(), "context deadline exceeded")) {
		t.Fatalf("expected backend initialization timeout, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("backend initialization timeout took too long: %s", elapsed)
	}
}

func TestUninstallRemovesLaunchAgentArtifacts(t *testing.T) {
	tempDir, paths := newShortPaths(t)
	spawnFile := filepath.Join(tempDir, "spawn.log")
	serverCfg := testServerConfig(t, paths, spawnFile, 5*time.Second)
	harness := newServerHarness(t, serverCfg)
	launchd := &fakeLaunchd{harness: harness}
	clientCfg := testClientConfig(paths, spawnFile, 5*time.Second, launchd)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := ListTools(ctx, clientCfg, Request{Timeout: 2 * time.Second}); err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}
	if err := Uninstall(ctx, clientCfg); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}
	for _, path := range []string{paths.PlistPath, paths.SocketPath, paths.PIDPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err=%v", path, err)
		}
	}
}

func testClientConfig(paths Paths, spawnFile string, idleTimeout time.Duration, launchd Launchd) Config {
	return Config{
		Paths:       paths,
		Label:       LaunchAgentLabel,
		IdleTimeout: idleTimeout,
		Command:     helperCommand("basic"),
		BaseEnv: append(os.Environ(),
			"GO_WANT_AGENT_HELPER_PROCESS=1",
			"AGENT_HELPER_SPAWN_FILE="+spawnFile,
		),
		ErrOut:         io.Discard,
		Launchd:        launchd,
		ExecutablePath: func() (string, error) { return "/tmp/xcodecli-test", nil },
	}
}

func testServerConfig(t *testing.T, paths Paths, spawnFile string, idleTimeout time.Duration) Config {
	t.Helper()
	cfg, err := normalizeConfig(Config{
		Paths:       paths,
		Label:       LaunchAgentLabel,
		IdleTimeout: idleTimeout,
		Command:     helperCommand("basic"),
		BaseEnv: append(os.Environ(),
			"GO_WANT_AGENT_HELPER_PROCESS=1",
			"AGENT_HELPER_SPAWN_FILE="+spawnFile,
		),
		ErrOut:         io.Discard,
		ExecutablePath: func() (string, error) { return "/tmp/xcodecli-test", nil },
	})
	if err != nil {
		t.Fatalf("normalizeConfig returned error: %v", err)
	}
	return cfg
}

func newShortPaths(t *testing.T) (string, Paths) {
	t.Helper()
	tempDir, err := os.MkdirTemp("/tmp", "xcda-")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	return tempDir, Paths{
		SupportDir: tempDir,
		SocketPath: filepath.Join(tempDir, "s.sock"),
		PIDPath:    filepath.Join(tempDir, "a.pid"),
		LogPath:    filepath.Join(tempDir, "a.log"),
		PlistPath:  filepath.Join(tempDir, "a.plist"),
	}
}

type fakeLaunchd struct {
	mu           sync.Mutex
	bootstrapped bool
	harness      *serverHarness
}

type blockingLaunchd struct{}

func (blockingLaunchd) Print(ctx context.Context, target string) (string, error) {
	return "", fmt.Errorf("service %s not loaded", target)
}

func (blockingLaunchd) Bootstrap(ctx context.Context, domainTarget, plistPath string) error {
	<-ctx.Done()
	return ctx.Err()
}

func (blockingLaunchd) Kickstart(ctx context.Context, serviceTarget string) error {
	<-ctx.Done()
	return ctx.Err()
}

func (blockingLaunchd) Bootout(ctx context.Context, target string) error {
	return nil
}

func (f *fakeLaunchd) Print(ctx context.Context, target string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.bootstrapped {
		return "", fmt.Errorf("service %s not loaded", target)
	}
	return target, nil
}

func (f *fakeLaunchd) Bootstrap(ctx context.Context, domainTarget, plistPath string) error {
	f.mu.Lock()
	f.bootstrapped = true
	f.mu.Unlock()
	return f.harness.start()
}

func (f *fakeLaunchd) Kickstart(ctx context.Context, serviceTarget string) error {
	f.mu.Lock()
	bootstrapped := f.bootstrapped
	f.mu.Unlock()
	if !bootstrapped {
		return fmt.Errorf("service %s not loaded", serviceTarget)
	}
	return f.harness.start()
}

func (f *fakeLaunchd) Bootout(ctx context.Context, target string) error {
	f.mu.Lock()
	f.bootstrapped = false
	f.mu.Unlock()
	return f.harness.stop()
}

type serverHarness struct {
	t       *testing.T
	cfg     Config
	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
	errCh   chan error
}

func newServerHarness(t *testing.T, cfg Config) *serverHarness {
	return &serverHarness{t: t, cfg: cfg}
}

func (h *serverHarness) start() error {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.running = true
	h.errCh = make(chan error, 1)
	h.mu.Unlock()

	go func() {
		err := RunServer(ctx, h.cfg)
		h.errCh <- err
		h.mu.Lock()
		h.running = false
		h.cancel = nil
		h.mu.Unlock()
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		conn, err := net.DialTimeout("unix", h.cfg.Paths.SocketPath, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for test agent server: %w", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (h *serverHarness) stop() error {
	h.mu.Lock()
	cancel := h.cancel
	errCh := h.errCh
	running := h.running
	h.mu.Unlock()
	if !running || cancel == nil {
		return nil
	}
	cancel()
	select {
	case err := <-errCh:
		return err
	case <-time.After(3 * time.Second):
		return errors.New("timed out waiting for test agent server to stop")
	}
}

func helperCommand(mode string) mcp.Command {
	return mcp.Command{Path: os.Args[0], Args: []string{"-test.run=TestAgentHelperProcess", "--", mode}}
}

func helperMode(t *testing.T) string {
	t.Helper()
	idx := -1
	for i, arg := range os.Args {
		if arg == "--" {
			idx = i
			break
		}
	}
	if idx == -1 || idx+1 >= len(os.Args) {
		t.Fatal("missing helper mode")
	}
	return os.Args[idx+1]
}

func helperSpawnCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) failed: %v", path, err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

func TestAgentHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_AGENT_HELPER_PROCESS") != "1" {
		return
	}
	if spawnFile := os.Getenv("AGENT_HELPER_SPAWN_FILE"); spawnFile != "" {
		file, err := os.OpenFile(spawnFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatalf("OpenFile spawn log failed: %v", err)
		}
		fmt.Fprintln(file, "spawn")
		file.Close()
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			os.Exit(0)
		}
		line = bytesTrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var req map[string]any
		if err := json.Unmarshal(line, &req); err != nil {
			t.Fatalf("decode helper request: %v", err)
		}
		method, _ := req["method"].(string)
		id := req["id"]
		switch method {
		case "initialize":
			switch helperMode(t) {
			case "timeout-init":
				time.Sleep(2 * time.Second)
				continue
			default:
				writeHelperResponse(t, map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"protocolVersion": "2025-06-18"}})
			}
		case "notifications/initialized":
			continue
		case "tools/list":
			writeHelperResponse(t, map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"tools": []map[string]any{{"name": "list_windows", "description": "List Xcode windows"}}}})
		case "tools/call":
			params, _ := req["params"].(map[string]any)
			writeHelperResponse(t, map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"content": []map[string]any{{"type": "text", "text": "ok"}}, "echoName": params["name"]}})
		default:
			t.Fatalf("unexpected helper method %q", method)
		}
	}
}

func writeHelperResponse(t *testing.T, payload any) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal helper response: %v", err)
	}
	fmt.Fprintln(os.Stdout, string(data))
}
