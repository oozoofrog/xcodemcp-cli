package agent

import (
	"bufio"
	"bytes"
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

func TestDefaultIdleTimeoutIs24Hours(t *testing.T) {
	if DefaultIdleTimeout != 24*time.Hour {
		t.Fatalf("DefaultIdleTimeout = %s, want 24h", DefaultIdleTimeout)
	}
}

func TestListToolsReplacesIdleSessionWhenSessionKeyChanges(t *testing.T) {
	tempDir, paths := newShortPaths(t)
	spawnFile := filepath.Join(tempDir, "spawn.log")
	serverCfg := testServerConfig(t, paths, spawnFile, 5*time.Second)
	harness := newServerHarness(t, serverCfg)
	launchd := &fakeLaunchd{harness: harness}
	clientCfg := testClientConfig(paths, spawnFile, 5*time.Second, launchd)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := ListTools(ctx, clientCfg, Request{Timeout: 2 * time.Second, SessionID: "session-a"}); err != nil {
		t.Fatalf("ListTools(session-a) returned error: %v", err)
	}
	if _, err := ListTools(ctx, clientCfg, Request{Timeout: 2 * time.Second, SessionID: "session-b"}); err != nil {
		t.Fatalf("ListTools(session-b) returned error: %v", err)
	}

	status, err := StatusInfo(ctx, clientCfg)
	if err != nil {
		t.Fatalf("StatusInfo returned error: %v", err)
	}
	if status.BackendSessions != 1 {
		t.Fatalf("BackendSessions = %d, want 1 after replacing idle session", status.BackendSessions)
	}
	if count := helperSpawnCount(t, spawnFile); count != 2 {
		t.Fatalf("backend helper spawn count = %d, want 2 after session change", count)
	}

	if _, err := ListTools(ctx, clientCfg, Request{Timeout: 2 * time.Second, SessionID: "session-b"}); err != nil {
		t.Fatalf("ListTools(session-b reuse) returned error: %v", err)
	}
	if count := helperSpawnCount(t, spawnFile); count != 2 {
		t.Fatalf("backend helper spawn count = %d, want 2 after reusing latest session", count)
	}
}

func TestListToolsRetiresPreviousInFlightSessionAfterHandoff(t *testing.T) {
	tempDir, paths := newShortPaths(t)
	spawnFile := filepath.Join(tempDir, "spawn.log")
	serverCfg := testServerConfig(t, paths, spawnFile, 5*time.Second)
	serverCfg.Command = helperCommand("slow-list")
	harness := newServerHarness(t, serverCfg)
	launchd := &fakeLaunchd{harness: harness}
	clientCfg := testClientConfig(paths, spawnFile, 5*time.Second, launchd)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 2)
	go func() {
		_, err := ListTools(ctx, clientCfg, Request{Timeout: 2 * time.Second, SessionID: "session-a"})
		errCh <- err
	}()
	waitForSpawnCount(t, spawnFile, 1, 2*time.Second)
	go func() {
		_, err := ListTools(ctx, clientCfg, Request{Timeout: 2 * time.Second, SessionID: "session-b"})
		errCh <- err
	}()

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("ListTools concurrent call %d returned error: %v", i+1, err)
		}
	}

	status, err := StatusInfo(ctx, clientCfg)
	if err != nil {
		t.Fatalf("StatusInfo returned error: %v", err)
	}
	if status.BackendSessions != 1 {
		t.Fatalf("BackendSessions = %d, want 1 after retiring previous in-flight session", status.BackendSessions)
	}
	if count := helperSpawnCount(t, spawnFile); count != 2 {
		t.Fatalf("backend helper spawn count = %d, want 2 after handoff", count)
	}

	if _, err := ListTools(ctx, clientCfg, Request{Timeout: 2 * time.Second, SessionID: "session-b"}); err != nil {
		t.Fatalf("ListTools(session-b reuse) returned error: %v", err)
	}
	if count := helperSpawnCount(t, spawnFile); count != 2 {
		t.Fatalf("backend helper spawn count = %d, want 2 after reusing handed-off session", count)
	}
}

func TestListToolsRecyclesLaunchAgentWhenRegisteredBinaryChanges(t *testing.T) {
	tempDir, paths := newShortPaths(t)
	spawnFile := filepath.Join(tempDir, "spawn.log")
	serverCfg := testServerConfig(t, paths, spawnFile, 5*time.Second)
	harness := newServerHarness(t, serverCfg)
	launchd := &fakeLaunchd{harness: harness, bootstrapped: true}
	clientCfg := testClientConfig(paths, spawnFile, 5*time.Second, launchd)
	clientCfg.ExecutablePath = func() (string, error) { return "/tmp/xcodecli-new", nil }

	if err := os.WriteFile(paths.PlistPath, []byte(renderLaunchAgentPlist(paths, LaunchAgentLabel, "/tmp/xcodecli-old")), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", paths.PlistPath, err)
	}
	if err := harness.start(); err != nil {
		t.Fatalf("harness.start() returned error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := ListTools(ctx, clientCfg, Request{Timeout: 2 * time.Second}); err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}

	registeredBinary, err := readLaunchAgentBinaryPath(paths.PlistPath)
	if err != nil {
		t.Fatalf("readLaunchAgentBinaryPath returned error: %v", err)
	}
	if registeredBinary != "/tmp/xcodecli-new" {
		t.Fatalf("registered binary = %q, want /tmp/xcodecli-new", registeredBinary)
	}
	if launchd.bootoutCalls != 1 {
		t.Fatalf("bootoutCalls = %d, want 1", launchd.bootoutCalls)
	}
	if launchd.bootstrapCalls != 1 {
		t.Fatalf("bootstrapCalls = %d, want 1", launchd.bootstrapCalls)
	}
	if count := helperSpawnCount(t, spawnFile); count != 1 {
		t.Fatalf("backend helper spawn count = %d, want 1 after stale-agent recycle", count)
	}
}

func TestListToolsRecyclesLaunchAgentWhenBinaryIdentityChangesAtSamePath(t *testing.T) {
	tempDir, paths := newShortPaths(t)
	spawnFile := filepath.Join(tempDir, "spawn.log")
	binaryPath := filepath.Join(tempDir, "xcodecli-bin")
	if err := os.WriteFile(binaryPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", binaryPath, err)
	}

	serverCfg := testServerConfig(t, paths, spawnFile, 5*time.Second)
	serverCfg.ExecutablePath = func() (string, error) { return binaryPath, nil }
	harness := newServerHarness(t, serverCfg)
	launchd := &fakeLaunchd{harness: harness, bootstrapped: true}
	clientCfg := testClientConfig(paths, spawnFile, 5*time.Second, launchd)
	clientCfg.ExecutablePath = func() (string, error) { return binaryPath, nil }

	if err := os.WriteFile(paths.PlistPath, []byte(renderLaunchAgentPlist(paths, LaunchAgentLabel, binaryPath)), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", paths.PlistPath, err)
	}
	if err := harness.start(); err != nil {
		t.Fatalf("harness.start() returned error: %v", err)
	}

	if err := os.WriteFile(binaryPath, []byte("new-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", binaryPath, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := ListTools(ctx, clientCfg, Request{Timeout: 2 * time.Second}); err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}

	currentIdentity, err := binaryIdentityForExecutable(binaryPath)
	if err != nil {
		t.Fatalf("binaryIdentityForExecutable returned error: %v", err)
	}
	persistedIdentity, err := readBinaryIdentity(binaryIdentityPath(paths))
	if err != nil {
		t.Fatalf("readBinaryIdentity returned error: %v", err)
	}
	if persistedIdentity != currentIdentity {
		t.Fatalf("persisted binary identity = %q, want %q", persistedIdentity, currentIdentity)
	}
	if launchd.bootoutCalls != 1 {
		t.Fatalf("bootoutCalls = %d, want 1", launchd.bootoutCalls)
	}
	if launchd.bootstrapCalls != 1 {
		t.Fatalf("bootstrapCalls = %d, want 1", launchd.bootstrapCalls)
	}
	if count := helperSpawnCount(t, spawnFile); count != 1 {
		t.Fatalf("backend helper spawn count = %d, want 1 after same-path recycle", count)
	}
}

func TestListToolsDoesNotBlockOnRetiredIdleSessionAbort(t *testing.T) {
	oldFactory := newSessionClient
	t.Cleanup(func() { newSessionClient = oldFactory })

	abortStarted := make(chan struct{})
	abortRelease := make(chan struct{})
	abortFinished := make(chan struct{})

	newSessionClient = func(ctx context.Context, cfg mcp.Config) (sessionClient, error) {
		sessionID := envValue(cfg.Env, "MCP_XCODE_SESSION_ID")
		client := &fakeSessionClient{
			listToolsFn: func() ([]map[string]any, error) {
				return []map[string]any{{"name": sessionID}}, nil
			},
		}
		if sessionID == "session-a" {
			client.abortFn = func() error {
				close(abortStarted)
				<-abortRelease
				close(abortFinished)
				return nil
			}
		}
		return client, nil
	}

	s := &server{
		cfg:      Config{ErrOut: io.Discard},
		sessions: make(map[sessionKey]*pooledSession),
	}
	if _, err := s.listTools(context.Background(), rpcRequest{SessionID: "session-a", TimeoutMS: 1000}); err != nil {
		t.Fatalf("listTools(session-a) returned error: %v", err)
	}

	done := make(chan error, 1)
	started := time.Now()
	go func() {
		_, err := s.listTools(context.Background(), rpcRequest{SessionID: "session-b", TimeoutMS: 1000})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("listTools(session-b) returned error: %v", err)
		}
		if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
			t.Fatalf("listTools(session-b) took %s, want handoff without waiting for abort", elapsed)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("listTools(session-b) blocked on retired session abort")
	}

	select {
	case <-abortStarted:
	case <-time.After(time.Second):
		t.Fatal("retired session abort did not start")
	}

	close(abortRelease)
	select {
	case <-abortFinished:
	case <-time.After(time.Second):
		t.Fatal("retired session abort did not finish")
	}
}

func TestFinishSessionDoesNotBlockOnRetiredInFlightAbort(t *testing.T) {
	abortStarted := make(chan struct{})
	abortRelease := make(chan struct{})
	abortFinished := make(chan struct{})

	key := sessionKey{SessionID: "session-a"}
	pooled := &pooledSession{
		key:            key,
		client:         &fakeSessionClient{},
		inFlight:       1,
		retireWhenIdle: true,
	}
	pooled.client = &fakeSessionClient{
		abortFn: func() error {
			close(abortStarted)
			<-abortRelease
			close(abortFinished)
			return nil
		},
	}

	s := &server{
		cfg:      Config{ErrOut: io.Discard},
		sessions: map[sessionKey]*pooledSession{key: pooled},
	}

	started := time.Now()
	done := make(chan struct{})
	go func() {
		s.finishSession(pooled)
		close(done)
	}()

	select {
	case <-done:
		if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
			t.Fatalf("finishSession took %s, want return without waiting for abort", elapsed)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("finishSession blocked on retired session abort")
	}

	s.mu.Lock()
	_, exists := s.sessions[key]
	s.mu.Unlock()
	if exists {
		t.Fatal("retired in-flight session remained in the pool")
	}

	select {
	case <-abortStarted:
	case <-time.After(time.Second):
		t.Fatal("retired in-flight abort did not start")
	}

	close(abortRelease)
	select {
	case <-abortFinished:
	case <-time.After(time.Second):
		t.Fatal("retired in-flight abort did not finish")
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
	if err == nil || !strings.Contains(err.Error(), "starting the LaunchAgent or initializing the mcpbridge session") || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected caller timeout error, got %v", err)
	}
	if strings.Contains(err.Error(), "after 5s") {
		t.Fatalf("expected caller timeout message to use remaining budget, got %v", err)
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
	if err == nil || (!strings.Contains(err.Error(), "request timed out after 200ms") && !strings.Contains(err.Error(), "context deadline exceeded")) {
		t.Fatalf("expected backend initialization timeout, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("backend initialization timeout took too long: %s", elapsed)
	}
}

func TestCallToolTimeoutIncludesRequestTimeoutMessage(t *testing.T) {
	tempDir, paths := newShortPaths(t)
	spawnFile := filepath.Join(tempDir, "spawn.log")
	serverCfg := testServerConfig(t, paths, spawnFile, 5*time.Second)
	serverCfg.Command = helperCommand("timeout-call")
	harness := newServerHarness(t, serverCfg)
	launchd := &fakeLaunchd{harness: harness}
	clientCfg := testClientConfig(paths, spawnFile, 5*time.Second, launchd)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := CallTool(ctx, clientCfg, Request{Timeout: 500 * time.Millisecond}, "BuildProject", map[string]any{"tabIdentifier": "demo"})
	if err == nil || (!strings.Contains(err.Error(), "while calling BuildProject") && !strings.Contains(err.Error(), "context deadline exceeded")) {
		t.Fatalf("expected request timeout message, got %v", err)
	}
	if strings.Contains(err.Error(), "while calling BuildProject") {
		if strings.Contains(err.Error(), "after 500ms") {
			t.Fatalf("expected request timeout message to use remaining budget after cold start, got %v", err)
		}
		if !strings.Contains(err.Error(), "not the mcpbridge session idle timeout") {
			t.Fatalf("expected idle-timeout clarification, got %v", err)
		}
		if strings.Contains(err.Error(), "connecting to the LaunchAgent after startup") {
			t.Fatalf("expected tool timeout labeling to be preserved after cold start, got %v", err)
		}
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("call timeout took too long: %s", elapsed)
	}
}

func TestDoRPCCancelWithoutDeadlineUnblocksRead(t *testing.T) {
	_, paths := newShortPaths(t)
	cfg := Config{
		Paths: paths,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			client, server := net.Pipe()
			t.Cleanup(func() { _ = server.Close() })
			return client, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := doRPC(ctx, cfg, rpcRequest{Method: "tools/list"})
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected doRPC to return an error after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("doRPC did not unblock after cancellation")
	}
}

func TestDoWithAutostartReturnsServerResponseErrorVerbatim(t *testing.T) {
	_, paths := newShortPaths(t)
	cfg := Config{
		Paths:       paths,
		Label:       LaunchAgentLabel,
		IdleTimeout: time.Second,
		ErrOut:      io.Discard,
		ExecutablePath: func() (string, error) {
			return "/tmp/xcodecli-test", nil
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			client, server := net.Pipe()
			go func() {
				defer server.Close()
				_, _ = bufio.NewReader(server).ReadBytes('\n')
				_, _ = server.Write([]byte(`{"error":"backend boom"}` + "\n"))
			}()
			return client, nil
		},
	}

	_, err := doWithAutostart(context.Background(), cfg, rpcRequest{Method: "tools/list", TimeoutMS: 1000})
	if err == nil || err.Error() != "backend boom" {
		t.Fatalf("expected server response error, got %v", err)
	}
}

func TestServerHelpersCoverClientLifecycleAndStatus(t *testing.T) {
	abortCalled := false
	closeCalled := false
	pooled := &pooledSession{
		client: &fakeSessionClient{
			abortFn: func() error {
				abortCalled = true
				return nil
			},
			closeFn: func() error {
				closeCalled = true
				return nil
			},
		},
	}
	s := &server{
		cfg: Config{IdleTimeout: 5 * time.Second, ErrOut: io.Discard},
		sessions: map[sessionKey]*pooledSession{
			{SessionID: "a"}: pooled,
			{SessionID: "b"}: {},
		},
	}

	s.discardClient(pooled)
	if !abortCalled || pooled.client != nil {
		t.Fatalf("discardClient did not abort and clear client: abortCalled=%t client=%v", abortCalled, pooled.client)
	}

	pooled.client = &fakeSessionClient{
		closeFn: func() error {
			closeCalled = true
			return nil
		},
	}
	s.closeSession(pooled)
	if !closeCalled || pooled.client != nil {
		t.Fatalf("closeSession did not close and clear client: closeCalled=%t client=%v", closeCalled, pooled.client)
	}

	status := s.runtimeStatus()
	if status.BackendSessions != 0 {
		t.Fatalf("BackendSessions = %d, want 0", status.BackendSessions)
	}
}

func TestRequestContextAndSetEnvValueHelpers(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()

	ctx, cancel := requestContext(parent, rpcRequest{})
	cancelParent()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("requestContext should inherit parent cancellation")
	}
	cancel()

	timeoutCtx, cancelTimeout := requestContext(context.Background(), rpcRequest{TimeoutMS: 50})
	defer cancelTimeout()
	if _, ok := timeoutCtx.Deadline(); !ok {
		t.Fatal("requestContext with timeout should have deadline")
	}

	env := []string{"A=1"}
	replaced := setEnvValue(env, "A", "2")
	if len(replaced) != 1 || replaced[0] != "A=2" {
		t.Fatalf("setEnvValue replace = %v, want [A=2]", replaced)
	}
	appended := setEnvValue(env, "B", "3")
	if len(appended) != 2 || appended[1] != "B=3" {
		t.Fatalf("setEnvValue append = %v, want appended B=3", appended)
	}
}

func TestTimeoutBudgetMillisUsesRemainingDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	budget := timeoutBudgetMillis(ctx, int64((500 * time.Millisecond).Milliseconds()))
	if budget <= 0 || budget >= 500 {
		t.Fatalf("budget = %dms, want >0 and <500", budget)
	}
}

func TestSessionKeyAndErrorHelpers(t *testing.T) {
	req := rpcRequest{XcodePID: "123", SessionID: "abc", DeveloperDir: "/Applications/Xcode.app/Contents/Developer"}
	key := sessionKeyForRequest(req)
	if key.XcodePID != "123" || key.SessionID != "abc" || key.DeveloperDir != "/Applications/Xcode.app/Contents/Developer" {
		t.Fatalf("unexpected sessionKey: %+v", key)
	}

	baseErr := errors.New("socket missing")
	unavailable := unavailableError{stage: "connect", err: baseErr}
	if unavailable.Error() != "socket missing" {
		t.Fatalf("Error() = %q, want socket missing", unavailable.Error())
	}
	if !errors.Is(unavailable, baseErr) {
		t.Fatal("Unwrap should expose underlying error")
	}

	if got, err := resolvedExecutablePath(); err != nil || got == "" {
		t.Fatalf("resolvedExecutablePath() = %q, err=%v; want non-empty path", got, err)
	}
}

func TestHandleConnCancelsInFlightRequestOnDisconnect(t *testing.T) {
	oldFactory := newSessionClient
	t.Cleanup(func() { newSessionClient = oldFactory })

	abortCalled := make(chan struct{})
	releaseCall := make(chan struct{})
	newSessionClient = func(ctx context.Context, cfg mcp.Config) (sessionClient, error) {
		return &fakeSessionClient{
			callToolFn: func(name string, arguments map[string]any) (mcp.CallResult, error) {
				<-releaseCall
				return mcp.CallResult{}, context.Canceled
			},
			abortFn: func() error {
				close(abortCalled)
				close(releaseCall)
				return nil
			},
		}, nil
	}

	s := &server{
		cfg: Config{
			IdleTimeout: time.Hour,
			ErrOut:      io.Discard,
		},
		sessions: make(map[sessionKey]*pooledSession),
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		s.handleConn(serverConn)
		close(done)
	}()

	payload, err := json.Marshal(rpcRequest{
		Method:    "tools/call",
		ToolName:  "BuildProject",
		Arguments: map[string]any{"tabIdentifier": "demo"},
	})
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}
	if _, err := clientConn.Write(append(payload, '\n')); err != nil {
		t.Fatalf("clientConn.Write failed: %v", err)
	}
	_ = clientConn.Close()

	select {
	case <-abortCalled:
	case <-time.After(time.Second):
		t.Fatal("expected disconnect to abort in-flight request")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handleConn did not return after disconnect")
	}
}

func TestListToolsAutostartUsesRemainingTimeoutBudget(t *testing.T) {
	_, paths := newShortPaths(t)
	started := time.Now()
	readyAfter := 150 * time.Millisecond
	reqTimeout := 300 * time.Millisecond

	var mu sync.Mutex
	seenTimeoutMS := int64(0)

	cfg := Config{
		Paths:       paths,
		Label:       LaunchAgentLabel,
		IdleTimeout: 5 * time.Second,
		ErrOut:      io.Discard,
		Launchd:     bootstrapLaunchd{},
		ExecutablePath: func() (string, error) {
			return "/tmp/xcodecli-test", nil
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if time.Since(started) < readyAfter {
				return nil, fmt.Errorf("connect to agent socket %s: no such file or directory", address)
			}

			client, server := net.Pipe()
			go func() {
				defer server.Close()
				line, err := bufio.NewReader(server).ReadBytes('\n')
				if err != nil {
					return
				}

				var req rpcRequest
				if err := json.Unmarshal(bytes.TrimSpace(line), &req); err != nil {
					t.Errorf("decode request: %v", err)
					return
				}

				switch req.Method {
				case "ping":
					_, _ = server.Write([]byte(`{"status":{"pid":1,"idleTimeoutMs":86400000,"backendSessions":1}}` + "\n"))
				case "tools/list":
					mu.Lock()
					seenTimeoutMS = req.TimeoutMS
					mu.Unlock()
					_, _ = server.Write([]byte(`{"tools":[]}` + "\n"))
				default:
					t.Errorf("unexpected method %q", req.Method)
				}
			}()
			return client, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), reqTimeout)
	defer cancel()

	if _, err := ListTools(ctx, cfg, Request{Timeout: reqTimeout}); err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if seenTimeoutMS <= 0 {
		t.Fatalf("captured timeout = %d, want > 0", seenTimeoutMS)
	}
	if seenTimeoutMS >= reqTimeout.Milliseconds() {
		t.Fatalf("captured timeout = %dms, want less than original %dms after autostart", seenTimeoutMS, reqTimeout.Milliseconds())
	}
}

func TestWaitForReadyHonorsShortCallerDeadline(t *testing.T) {
	started := time.Now()
	cfg := Config{
		Label:       LaunchAgentLabel,
		Paths:       Paths{SocketPath: "/tmp/unused-agent.sock"},
		DialContext: readyAfterDialer(started, time.Hour),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	begin := time.Now()
	err := waitForReady(ctx, cfg)
	if err == nil {
		t.Fatal("expected waitForReady to time out")
	}
	if elapsed := time.Since(begin); elapsed > time.Second {
		t.Fatalf("waitForReady short deadline took too long: %s", elapsed)
	}
}

func TestWaitForReadyCanExceedFiveSecondsWhenCallerDeadlineAllows(t *testing.T) {
	started := time.Now()
	cfg := Config{
		Label:       LaunchAgentLabel,
		Paths:       Paths{SocketPath: "/tmp/unused-agent.sock"},
		DialContext: readyAfterDialer(started, 5200*time.Millisecond),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()

	begin := time.Now()
	if err := waitForReady(ctx, cfg); err != nil {
		t.Fatalf("waitForReady returned error: %v", err)
	}
	if elapsed := time.Since(begin); elapsed < 5*time.Second {
		t.Fatalf("waitForReady returned too early: %s", elapsed)
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

func readyAfterDialer(start time.Time, readyAfter time.Duration) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if time.Since(start) < readyAfter {
			return nil, fmt.Errorf("agent socket not ready yet")
		}

		client, server := net.Pipe()
		go func() {
			defer server.Close()
			_, _ = bufio.NewReader(server).ReadBytes('\n')
			_, _ = server.Write([]byte(`{"status":{"pid":1,"idleTimeoutMs":86400000,"backendSessions":1}}` + "\n"))
		}()
		return client, nil
	}
}

type fakeLaunchd struct {
	mu             sync.Mutex
	bootstrapped   bool
	bootstrapCalls int
	kickstartCalls int
	bootoutCalls   int
	harness        *serverHarness
}

type fakeSessionClient struct {
	listToolsFn func() ([]map[string]any, error)
	callToolFn  func(string, map[string]any) (mcp.CallResult, error)
	closeFn     func() error
	abortFn     func() error
}

func (f *fakeSessionClient) ListTools() ([]map[string]any, error) {
	if f.listToolsFn != nil {
		return f.listToolsFn()
	}
	return nil, nil
}

func (f *fakeSessionClient) CallTool(name string, arguments map[string]any) (mcp.CallResult, error) {
	if f.callToolFn != nil {
		return f.callToolFn(name, arguments)
	}
	return mcp.CallResult{}, nil
}

func (f *fakeSessionClient) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

func (f *fakeSessionClient) Abort() error {
	if f.abortFn != nil {
		return f.abortFn()
	}
	return nil
}

type blockingLaunchd struct{}

type bootstrapLaunchd struct{}

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

func (bootstrapLaunchd) Print(ctx context.Context, target string) (string, error) {
	return "", fmt.Errorf("service %s not loaded", target)
}

func (bootstrapLaunchd) Bootstrap(ctx context.Context, domainTarget, plistPath string) error {
	return nil
}

func (bootstrapLaunchd) Kickstart(ctx context.Context, serviceTarget string) error {
	return nil
}

func (bootstrapLaunchd) Bootout(ctx context.Context, target string) error {
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
	f.bootstrapCalls++
	f.mu.Unlock()
	return f.harness.start()
}

func (f *fakeLaunchd) Kickstart(ctx context.Context, serviceTarget string) error {
	f.mu.Lock()
	bootstrapped := f.bootstrapped
	f.kickstartCalls++
	f.mu.Unlock()
	if !bootstrapped {
		return fmt.Errorf("service %s not loaded", serviceTarget)
	}
	return f.harness.start()
}

func (f *fakeLaunchd) Bootout(ctx context.Context, target string) error {
	f.mu.Lock()
	f.bootstrapped = false
	f.bootoutCalls++
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

func waitForSpawnCount(t *testing.T, path string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			trimmed := strings.TrimSpace(string(data))
			count := 0
			if trimmed != "" {
				count = len(strings.Split(trimmed, "\n"))
			}
			if count >= want {
				return
			}
		} else if !os.IsNotExist(err) {
			t.Fatalf("ReadFile(%q) failed: %v", path, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for spawn count %d at %s", want, path)
		}
		time.Sleep(20 * time.Millisecond)
	}
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
		line = bytes.TrimSpace(line)
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
			switch helperMode(t) {
			case "slow-list":
				time.Sleep(300 * time.Millisecond)
			}
			writeHelperResponse(t, map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"tools": []map[string]any{{"name": "list_windows", "description": "List Xcode windows"}}}})
		case "tools/call":
			params, _ := req["params"].(map[string]any)
			switch helperMode(t) {
			case "timeout-call":
				time.Sleep(2 * time.Second)
				continue
			}
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
