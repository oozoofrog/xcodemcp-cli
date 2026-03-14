package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const requestProtocolVersion = "2025-06-18"

type Command struct {
	Path string
	Args []string
}

type Config struct {
	Command Command
	Env     []string
	Debug   bool
	ErrOut  io.Writer
}

type CallResult struct {
	Result  map[string]any
	IsError bool
}

type session struct {
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       *bufio.Reader
	stderr       *stderrBuffer
	errOut       io.Writer
	debug        bool
	nextID       int64
	writeMu      sync.Mutex
	stderrDoneCh chan struct{}
}

type stderrBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
}

type toolsListResult struct {
	Tools      []map[string]any `json:"tools"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

func ListTools(ctx context.Context, cfg Config) ([]map[string]any, error) {
	client, err := NewClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()

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
		return res.tools, res.err
	case <-ctx.Done():
		_ = client.Abort()
		return nil, ctx.Err()
	}
}

func CallTool(ctx context.Context, cfg Config, name string, arguments map[string]any) (CallResult, error) {
	client, err := NewClient(ctx, cfg)
	if err != nil {
		return CallResult{}, err
	}
	defer func() { _ = client.Close() }()

	type result struct {
		callResult CallResult
		err        error
	}
	resultCh := make(chan result, 1)
	go func() {
		callResult, callErr := client.CallTool(name, arguments)
		resultCh <- result{callResult: callResult, err: callErr}
	}()

	select {
	case res := <-resultCh:
		return res.callResult, res.err
	case <-ctx.Done():
		_ = client.Abort()
		return CallResult{}, ctx.Err()
	}
}

func startSession(lifetimeCtx, initCtx context.Context, cfg Config) (*session, error) {
	if cfg.Command.Path == "" {
		return nil, errors.New("missing command path")
	}
	if lifetimeCtx == nil {
		lifetimeCtx = context.Background()
	}
	if initCtx == nil {
		initCtx = context.Background()
	}

	path, err := exec.LookPath(cfg.Command.Path)
	if err != nil {
		return nil, fmt.Errorf("resolve command %q: %w", cfg.Command.Path, err)
	}

	errOut := cfg.ErrOut
	if errOut == nil {
		errOut = io.Discard
	}

	cmd := exec.CommandContext(lifetimeCtx, path, cfg.Command.Args...)
	cmd.Env = cfg.Env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	if cfg.Debug {
		fmt.Fprintf(errOut, "[debug] starting %s %s\n", path, strings.Join(cfg.Command.Args, " "))
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start child process: %w", err)
	}

	s := &session{
		cmd:          cmd,
		stdin:        stdin,
		stdout:       bufio.NewReader(stdoutPipe),
		stderr:       &stderrBuffer{},
		errOut:       errOut,
		debug:        cfg.Debug,
		nextID:       1,
		stderrDoneCh: make(chan struct{}),
	}
	go s.captureStderr(stderrPipe)

	initErrCh := make(chan error, 1)
	go func() {
		initErrCh <- s.initialize()
	}()

	select {
	case err := <-initErrCh:
		if err != nil {
			_ = s.abort()
			return nil, err
		}
	case <-initCtx.Done():
		_ = s.abort()
		<-initErrCh
		return nil, initCtx.Err()
	}

	return s, nil
}

func (s *session) initialize() error {
	var result initializeResult
	if err := s.request("initialize", map[string]any{
		"protocolVersion": requestProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "xcodecli",
			"version": "dev",
		},
	}, &result); err != nil {
		return fmt.Errorf("initialize MCP session: %w", err)
	}

	if !isSupportedVersion(result.ProtocolVersion) {
		return fmt.Errorf("initialize MCP session: unsupported protocol version %q", result.ProtocolVersion)
	}

	if err := s.notify("notifications/initialized", map[string]any{}); err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}

	return nil
}

func (s *session) request(method string, params any, out any) error {
	id := s.nextID
	s.nextID++

	if s.debug {
		fmt.Fprintf(s.errOut, "[debug] mcp request -> %s (id=%d)\n", method, id)
	}

	if err := s.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}); err != nil {
		return err
	}

	for {
		env, raw, err := s.readEnvelope()
		if err != nil {
			return fmt.Errorf("wait for %s response: %w", method, err)
		}
		if s.debug {
			fmt.Fprintf(s.errOut, "[debug] mcp recv <- %s\n", raw)
		}
		if env.Method != "" {
			if hasID(env.ID) {
				_ = s.writeJSON(map[string]any{
					"jsonrpc": "2.0",
					"id":      json.RawMessage(env.ID),
					"error": map[string]any{
						"code":    -32601,
						"message": "Method not found",
					},
				})
				return fmt.Errorf("server request %q is not supported", env.Method)
			}
			if s.debug {
				fmt.Fprintf(s.errOut, "[debug] server notification ignored: %s\n", env.Method)
			}
			continue
		}

		responseID, err := parseID(env.ID)
		if err != nil {
			return err
		}
		if responseID != id {
			return fmt.Errorf("received unexpected response id %d while waiting for %d", responseID, id)
		}
		if env.Error != nil {
			return fmt.Errorf("JSON-RPC error %d: %s", env.Error.Code, env.Error.Message)
		}
		if out == nil {
			return nil
		}
		if len(env.Result) == 0 {
			return errors.New("response did not include a result")
		}
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
		return nil
	}
}

func (s *session) notify(method string, params any) error {
	if s.debug {
		fmt.Fprintf(s.errOut, "[debug] mcp notification -> %s\n", method)
	}
	return s.writeJSON(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

func (s *session) writeJSON(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal JSON-RPC message: %w", err)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.stdin.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("write JSON-RPC message: %w", s.wrapError(err))
	}
	return nil
}

func (s *session) readEnvelope() (rpcEnvelope, string, error) {
	for {
		line, err := s.stdout.ReadBytes('\n')
		if err != nil {
			if len(bytes.TrimSpace(line)) > 0 {
				var env rpcEnvelope
				text := strings.TrimSpace(string(line))
				if unmarshalErr := json.Unmarshal(bytes.TrimSpace(line), &env); unmarshalErr != nil {
					return rpcEnvelope{}, text, fmt.Errorf("decode JSON-RPC message: %w", s.wrapError(unmarshalErr))
				}
				return env, text, nil
			}
			if errors.Is(err, io.EOF) {
				return rpcEnvelope{}, "", fmt.Errorf("child process closed stdout: %w", s.wrapError(err))
			}
			return rpcEnvelope{}, "", s.wrapError(err)
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var env rpcEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			return rpcEnvelope{}, string(line), fmt.Errorf("decode JSON-RPC message: %w", s.wrapError(err))
		}
		return env, string(line), nil
	}
}

func (s *session) captureStderr(r io.Reader) {
	defer close(s.stderrDoneCh)
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		s.stderr.append(line + "\n")
		if s.debug {
			fmt.Fprintf(s.errOut, "[debug] child stderr: %s\n", line)
		}
	}
	if err := scanner.Err(); err != nil {
		s.stderr.append(err.Error() + "\n")
		if s.debug {
			fmt.Fprintf(s.errOut, "[debug] child stderr read error: %v\n", err)
		}
	}
}

func (s *session) close() error {
	return s.terminate(false)
}

func (s *session) abort() error {
	return s.terminate(true)
}

func (s *session) terminate(force bool) error {
	if s.stdin != nil {
		_ = s.stdin.Close()
		s.stdin = nil
	}

	if force && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- s.cmd.Wait()
	}()

	var waitErr error
	select {
	case waitErr = <-waitCh:
	case <-time.After(2 * time.Second):
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		waitErr = <-waitCh
	}
	<-s.stderrDoneCh
	if force {
		return nil
	}
	if waitErr == nil {
		return nil
	}
	if errors.Is(waitErr, context.Canceled) || errors.Is(waitErr, context.DeadlineExceeded) {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return s.wrapError(exitErr)
	}
	return s.wrapError(waitErr)
}

func (s *session) wrapError(err error) error {
	stderr := strings.TrimSpace(s.stderr.String())
	if stderr == "" {
		return err
	}
	return fmt.Errorf("%w (stderr: %s)", err, stderr)
}

func (b *stderrBuffer) append(text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, _ = b.buf.WriteString(text)
}

func (b *stderrBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func parseID(raw json.RawMessage) (int64, error) {
	if !hasID(raw) {
		return 0, errors.New("response did not include an id")
	}
	var id int64
	if err := json.Unmarshal(raw, &id); err != nil {
		return 0, fmt.Errorf("decode response id: %w", err)
	}
	return id, nil
}

func hasID(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

func isSupportedVersion(version string) bool {
	switch version {
	case "2025-06-18", "2025-03-26", "2024-11-05":
		return true
	default:
		return false
	}
}
