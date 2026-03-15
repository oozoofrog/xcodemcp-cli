package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/oozoofrog/xcodecli/internal/bridge"
	"github.com/oozoofrog/xcodecli/internal/mcp"
)

type sessionKey struct {
	XcodePID     string
	SessionID    string
	DeveloperDir string
}

type sessionClient interface {
	ListTools() ([]map[string]any, error)
	CallTool(name string, arguments map[string]any) (mcp.CallResult, error)
	Close() error
	Abort() error
}

var newSessionClient = func(ctx context.Context, cfg mcp.Config) (sessionClient, error) {
	return mcp.NewClient(ctx, cfg)
}

type pooledSession struct {
	key            sessionKey
	client         sessionClient
	mu             sync.Mutex
	inFlight       int
	retireWhenIdle bool
}

type server struct {
	cfg      Config
	listener net.Listener

	mu       sync.Mutex
	sessions map[sessionKey]*pooledSession
	active   int
	idle     *time.Timer
	closed   bool
}

func RunServer(ctx context.Context, cfg Config) error {
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.Paths.SupportDir, 0o700); err != nil {
		return fmt.Errorf("create agent support directory: %w", err)
	}
	if err := os.Remove(cfg.Paths.SocketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}
	listener, err := cfg.Listen("unix", cfg.Paths.SocketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Paths.SocketPath, err)
	}
	if err := os.Chmod(cfg.Paths.SocketPath, 0o600); err != nil {
		listener.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}
	if err := os.WriteFile(cfg.Paths.PIDPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		listener.Close()
		return fmt.Errorf("write agent pid file: %w", err)
	}

	s := &server{
		cfg:      cfg,
		listener: listener,
		sessions: make(map[sessionKey]*pooledSession),
	}
	defer func() {
		s.shutdown()
		_ = os.Remove(cfg.Paths.PIDPath)
		_ = os.Remove(cfg.Paths.SocketPath)
	}()

	s.startIdleTimer()
	go func() {
		<-ctx.Done()
		s.shutdown()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if s.isClosed() {
				return nil
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept agent connection: %w", err)
		}
		s.connectionStarted()
		go s.handleConn(conn)
	}
}

func (s *server) handleConn(conn net.Conn) {
	defer conn.Close()
	defer s.connectionFinished()

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		s.writeResponse(conn, rpcResponse{Error: fmt.Sprintf("read agent request: %v", err)})
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(bytesTrimSpace(line), &req); err != nil {
		s.writeResponse(conn, rpcResponse{Error: fmt.Sprintf("decode agent request: %v", err)})
		return
	}
	resp := s.dispatch(req)
	_ = s.writeResponse(conn, resp)
	if req.Method == "stop" && resp.Error == "" {
		go s.shutdown()
	}
}

func (s *server) dispatch(req rpcRequest) rpcResponse {
	switch req.Method {
	case "ping":
		return rpcResponse{Status: s.runtimeStatus()}
	case "status":
		return rpcResponse{Status: s.runtimeStatus()}
	case "stop":
		return rpcResponse{}
	case "tools/list":
		tools, err := s.listTools(req)
		if err != nil {
			return rpcResponse{Error: err.Error()}
		}
		return rpcResponse{Tools: tools}
	case "tools/call":
		result, err := s.callTool(req)
		if err != nil {
			return rpcResponse{Error: err.Error()}
		}
		return rpcResponse{Result: result.Result, IsError: result.IsError}
	default:
		return rpcResponse{Error: fmt.Sprintf("unsupported agent method %q", req.Method)}
	}
}

func (s *server) listTools(req rpcRequest) ([]map[string]any, error) {
	ctx, cancel := requestContext(req)
	defer cancel()
	pooled, retired := s.prepareSession(sessionKeyForRequest(req))
	s.abortSessionsAsync(retired)
	pooled.mu.Lock()
	defer func() {
		pooled.mu.Unlock()
		s.finishSession(pooled)
	}()

	client, err := s.ensureClient(ctx, pooled, req)
	if err != nil {
		return nil, err
	}

	type result struct {
		tools []map[string]any
		err   error
	}
	resultCh := make(chan result, 1)
	go func() {
		tools, callErr := client.ListTools()
		resultCh <- result{tools: tools, err: callErr}
	}()
	select {
	case res := <-resultCh:
		if res.err != nil {
			s.discardClient(pooled)
			return nil, res.err
		}
		return res.tools, nil
	case <-ctx.Done():
		s.discardClient(pooled)
		return nil, requestTimeoutError(req.TimeoutMS, requestTimeoutAction(req.Method, req.ToolName), ctx.Err())
	}
}

func (s *server) callTool(req rpcRequest) (mcp.CallResult, error) {
	ctx, cancel := requestContext(req)
	defer cancel()
	pooled, retired := s.prepareSession(sessionKeyForRequest(req))
	s.abortSessionsAsync(retired)
	pooled.mu.Lock()
	defer func() {
		pooled.mu.Unlock()
		s.finishSession(pooled)
	}()

	client, err := s.ensureClient(ctx, pooled, req)
	if err != nil {
		return mcp.CallResult{}, err
	}

	type result struct {
		callResult mcp.CallResult
		err        error
	}
	resultCh := make(chan result, 1)
	go func() {
		callResult, callErr := client.CallTool(req.ToolName, req.Arguments)
		resultCh <- result{callResult: callResult, err: callErr}
	}()
	select {
	case res := <-resultCh:
		if res.err != nil {
			s.discardClient(pooled)
			return mcp.CallResult{}, res.err
		}
		return res.callResult, nil
	case <-ctx.Done():
		s.discardClient(pooled)
		return mcp.CallResult{}, requestTimeoutError(req.TimeoutMS, requestTimeoutAction(req.Method, req.ToolName), ctx.Err())
	}
}

func sessionKeyForRequest(req rpcRequest) sessionKey {
	return sessionKey{
		XcodePID:     req.XcodePID,
		SessionID:    req.SessionID,
		DeveloperDir: req.DeveloperDir,
	}
}

func (s *server) prepareSession(key sessionKey) (*pooledSession, []*pooledSession) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pooled := s.sessions[key]
	if pooled == nil {
		pooled = &pooledSession{key: key}
		s.sessions[key] = pooled
	}
	pooled.inFlight++
	pooled.retireWhenIdle = false

	retired := make([]*pooledSession, 0)
	for otherKey, other := range s.sessions {
		if other == pooled {
			continue
		}
		if other.inFlight == 0 {
			delete(s.sessions, otherKey)
			other.retireWhenIdle = false
			retired = append(retired, other)
			continue
		}
		other.retireWhenIdle = true
	}

	return pooled, retired
}

func (s *server) finishSession(pooled *pooledSession) {
	var retire bool

	s.mu.Lock()
	if pooled.inFlight > 0 {
		pooled.inFlight--
	}
	if pooled.inFlight == 0 && pooled.retireWhenIdle {
		if current := s.sessions[pooled.key]; current == pooled {
			delete(s.sessions, pooled.key)
		}
		pooled.retireWhenIdle = false
		retire = true
	}
	s.mu.Unlock()

	if retire {
		s.abortSessionAsync(pooled)
	}
}

func (s *server) ensureClient(ctx context.Context, pooled *pooledSession, req rpcRequest) (sessionClient, error) {
	if pooled.client != nil {
		return pooled.client, nil
	}
	env := bridge.ApplyEnvOverrides(s.cfg.BaseEnv, bridge.EnvOptions{XcodePID: req.XcodePID, SessionID: req.SessionID})
	if req.DeveloperDir != "" {
		env = setEnvValue(env, "DEVELOPER_DIR", req.DeveloperDir)
	}
	client, err := newSessionClient(ctx, mcp.Config{
		Command: s.cfg.Command,
		Env:     env,
		Debug:   false,
		ErrOut:  s.cfg.ErrOut,
	})
	if err != nil {
		if ctx.Err() != nil {
			action := "initializing the mcpbridge session"
			if req.ToolName != "" {
				action = fmt.Sprintf("initializing the mcpbridge session for %s", req.ToolName)
			}
			return nil, requestTimeoutError(req.TimeoutMS, action, ctx.Err())
		}
		return nil, err
	}
	pooled.client = client
	return pooled.client, nil
}

func (s *server) discardClient(pooled *pooledSession) {
	if pooled.client != nil {
		_ = pooled.client.Abort()
		pooled.client = nil
	}
}

func (s *server) abortSessionsAsync(sessions []*pooledSession) {
	for _, pooled := range sessions {
		s.abortSessionAsync(pooled)
	}
}

func (s *server) abortSessionAsync(pooled *pooledSession) {
	if pooled == nil {
		return
	}
	go func() {
		client := detachClient(pooled)
		if client == nil {
			return
		}
		if err := client.Abort(); err != nil {
			if s.cfg.ErrOut != nil {
				fmt.Fprintf(s.cfg.ErrOut, "[debug] abort retired mcpbridge session: %v\n", err)
			}
		}
	}()
}

func detachClient(pooled *pooledSession) sessionClient {
	pooled.mu.Lock()
	defer pooled.mu.Unlock()
	client := pooled.client
	pooled.client = nil
	return client
}

func (s *server) closeSession(pooled *pooledSession) {
	pooled.mu.Lock()
	defer pooled.mu.Unlock()
	if pooled.client != nil {
		_ = pooled.client.Close()
		pooled.client = nil
	}
}

func (s *server) writeResponse(conn net.Conn, resp rpcResponse) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = conn.Write(append(payload, '\n'))
	return err
}

func (s *server) startIdleTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idle != nil {
		s.idle.Stop()
	}
	s.idle = time.AfterFunc(s.cfg.IdleTimeout, func() {
		s.shutdown()
	})
}

func (s *server) stopIdleTimerLocked() {
	if s.idle != nil {
		s.idle.Stop()
	}
}

func (s *server) connectionStarted() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active++
	s.stopIdleTimerLocked()
}

func (s *server) connectionFinished() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active > 0 {
		s.active--
	}
	if s.active == 0 && !s.closed {
		s.stopIdleTimerLocked()
		s.idle = time.AfterFunc(s.cfg.IdleTimeout, func() {
			s.shutdown()
		})
	}
}

func (s *server) runtimeStatus() *runtimeStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	backendSessions := 0
	for _, pooled := range s.sessions {
		if pooled.client != nil {
			backendSessions++
		}
	}
	return &runtimeStatus{
		PID:             os.Getpid(),
		IdleTimeoutMS:   s.cfg.IdleTimeout.Milliseconds(),
		BackendSessions: backendSessions,
	}
}

func (s *server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *server) shutdown() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	listener := s.listener
	s.stopIdleTimerLocked()
	sessions := make([]*pooledSession, 0, len(s.sessions))
	for _, pooled := range s.sessions {
		sessions = append(sessions, pooled)
	}
	s.mu.Unlock()

	if listener != nil {
		_ = listener.Close()
	}
	for _, pooled := range sessions {
		pooled.mu.Lock()
		if pooled.client != nil {
			_ = pooled.client.Close()
			pooled.client = nil
		}
		pooled.mu.Unlock()
	}
}

func requestContext(req rpcRequest) (context.Context, context.CancelFunc) {
	if req.TimeoutMS <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), time.Duration(req.TimeoutMS)*time.Millisecond)
}

func setEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			out = append(out, prefix+value)
			replaced = true
			continue
		}
		out = append(out, entry)
	}
	if !replaced {
		out = append(out, prefix+value)
	}
	return out
}
