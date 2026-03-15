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
	"time"

	"github.com/oozoofrog/xcodecli/internal/mcp"
)

type unavailableError struct {
	stage string
	err   error
}

func (e unavailableError) Error() string {
	return e.err.Error()
}

func (e unavailableError) Unwrap() error {
	return e.err
}

type serverResponseError struct {
	message string
}

func (e serverResponseError) Error() string {
	return e.message
}

func ListTools(ctx context.Context, cfg Config, req Request) ([]map[string]any, error) {
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	resp, err := doWithAutostart(ctx, cfg, rpcRequest{
		Method:       "tools/list",
		XcodePID:     req.XcodePID,
		SessionID:    req.SessionID,
		DeveloperDir: req.DeveloperDir,
		TimeoutMS:    durationMillis(req.Timeout),
		Debug:        req.Debug,
	})
	if err != nil {
		return nil, err
	}
	return resp.Tools, nil
}

func CallTool(ctx context.Context, cfg Config, req Request, toolName string, arguments map[string]any) (mcp.CallResult, error) {
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return mcp.CallResult{}, err
	}
	resp, err := doWithAutostart(ctx, cfg, rpcRequest{
		Method:       "tools/call",
		XcodePID:     req.XcodePID,
		SessionID:    req.SessionID,
		DeveloperDir: req.DeveloperDir,
		TimeoutMS:    durationMillis(req.Timeout),
		Debug:        req.Debug,
		ToolName:     toolName,
		Arguments:    arguments,
	})
	if err != nil {
		return mcp.CallResult{}, err
	}
	return mcp.CallResult{Result: resp.Result, IsError: resp.IsError}, nil
}

func StatusInfo(ctx context.Context, cfg Config) (Status, error) {
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return Status{}, err
	}
	status := Status{
		Label:       cfg.Label,
		PlistPath:   cfg.Paths.PlistPath,
		SocketPath:  cfg.Paths.SocketPath,
		IdleTimeout: cfg.IdleTimeout,
	}
	if _, err := os.Stat(cfg.Paths.PlistPath); err == nil {
		status.PlistInstalled = true
		registered, readErr := readLaunchAgentBinaryPath(cfg.Paths.PlistPath)
		if readErr == nil {
			status.RegisteredBinary = registered
		}
	} else if !os.IsNotExist(err) {
		return Status{}, fmt.Errorf("inspect launch agent plist: %w", err)
	}

	currentBinary, err := cfg.ExecutablePath()
	if err == nil {
		status.CurrentBinary = currentBinary
		status.BinaryPathMatches = samePath(currentBinary, status.RegisteredBinary)
	}

	resp, rpcErr := doRPC(ctx, cfg, rpcRequest{Method: "status"})
	if rpcErr == nil && resp.Status != nil {
		status.SocketReachable = true
		status.Running = true
		status.PID = resp.Status.PID
		status.IdleTimeout = time.Duration(resp.Status.IdleTimeoutMS) * time.Millisecond
		status.BackendSessions = resp.Status.BackendSessions
	}
	return status, nil
}

func Stop(ctx context.Context, cfg Config) error {
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return err
	}
	_, rpcErr := doRPC(ctx, cfg, rpcRequest{Method: "stop"})
	if rpcErr != nil && !isUnavailable(rpcErr) {
		return rpcErr
	}
	return nil
}

func Uninstall(ctx context.Context, cfg Config) error {
	cfg, err := normalizeConfig(cfg)
	if err != nil {
		return err
	}
	_ = Stop(ctx, cfg)
	_ = cfg.Launchd.Bootout(ctx, launchAgentServiceTarget(cfg.Label))
	pathsToRemove := []string{cfg.Paths.PlistPath, cfg.Paths.SocketPath, cfg.Paths.PIDPath, cfg.Paths.LogPath}
	for _, path := range pathsToRemove {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	for _, dir := range []string{cfg.Paths.SupportDir} {
		if dir == "" {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove support directory %s: %w", dir, err)
		}
	}
	return nil
}

func doWithAutostart(ctx context.Context, cfg Config, req rpcRequest) (rpcResponse, error) {
	mismatch, registeredBinary, currentBinary := launchAgentBinaryMismatch(cfg)
	if mismatch {
		if req.Debug {
			fmt.Fprintf(cfg.ErrOut, "[debug] registered LaunchAgent binary %s does not match current binary %s; recycling LaunchAgent %s\n", registeredBinary, currentBinary, cfg.Label)
		}
		if err := ensureAgentReady(ctx, cfg); err != nil {
			if ctx.Err() != nil {
				return rpcResponse{}, requestTimeoutError(req.TimeoutMS, "starting the LaunchAgent or initializing the mcpbridge session", ctx.Err())
			}
			return rpcResponse{}, err
		}
		effectiveReq, err := requestWithRemainingTimeout(ctx, req)
		if err != nil {
			return rpcResponse{}, requestTimeoutError(req.TimeoutMS, requestTimeoutAction(req.Method, req.ToolName), err)
		}
		resp, err := doRPC(ctx, cfg, effectiveReq)
		if err != nil {
			var serverErr serverResponseError
			if errors.As(err, &serverErr) {
				return rpcResponse{}, err
			}
			if ctx.Err() != nil {
				var unavailable unavailableError
				if errors.As(err, &unavailable) && unavailable.stage == "connect" {
					return rpcResponse{}, requestTimeoutError(req.TimeoutMS, "connecting to the LaunchAgent after startup", ctx.Err())
				}
				return rpcResponse{}, requestTimeoutError(req.TimeoutMS, requestTimeoutAction(req.Method, req.ToolName), ctx.Err())
			}
		}
		return resp, err
	}

	effectiveReq, err := requestWithRemainingTimeout(ctx, req)
	if err != nil {
		return rpcResponse{}, requestTimeoutError(req.TimeoutMS, requestTimeoutAction(req.Method, req.ToolName), err)
	}
	resp, err := doRPC(ctx, cfg, effectiveReq)
	if err == nil {
		return resp, nil
	}
	if !isUnavailable(err) {
		return rpcResponse{}, err
	}
	if req.Debug {
		fmt.Fprintf(cfg.ErrOut, "[debug] agent socket unavailable, ensuring LaunchAgent %s\n", cfg.Label)
	}
	if err := ensureAgentReady(ctx, cfg); err != nil {
		if ctx.Err() != nil {
			return rpcResponse{}, requestTimeoutError(req.TimeoutMS, "starting the LaunchAgent or initializing the mcpbridge session", ctx.Err())
		}
		return rpcResponse{}, err
	}
	effectiveReq, err = requestWithRemainingTimeout(ctx, req)
	if err != nil {
		return rpcResponse{}, requestTimeoutError(req.TimeoutMS, requestTimeoutAction(req.Method, req.ToolName), err)
	}
	resp, err = doRPC(ctx, cfg, effectiveReq)
	if err != nil {
		var serverErr serverResponseError
		if errors.As(err, &serverErr) {
			return rpcResponse{}, err
		}
		if ctx.Err() != nil {
			var unavailable unavailableError
			if errors.As(err, &unavailable) && unavailable.stage == "connect" {
				return rpcResponse{}, requestTimeoutError(req.TimeoutMS, "connecting to the LaunchAgent after startup", ctx.Err())
			}
			return rpcResponse{}, requestTimeoutError(req.TimeoutMS, requestTimeoutAction(req.Method, req.ToolName), ctx.Err())
		}
	}
	return resp, err
}

func launchAgentBinaryMismatch(cfg Config) (bool, string, string) {
	registeredBinary, err := readLaunchAgentBinaryPath(cfg.Paths.PlistPath)
	if err != nil || strings.TrimSpace(registeredBinary) == "" {
		return false, "", ""
	}
	currentBinary, err := cfg.ExecutablePath()
	if err != nil || strings.TrimSpace(currentBinary) == "" {
		return false, registeredBinary, ""
	}
	return !samePath(currentBinary, registeredBinary), registeredBinary, currentBinary
}

func doRPC(ctx context.Context, cfg Config, req rpcRequest) (rpcResponse, error) {
	conn, err := cfg.DialContext(ctx, "unix", cfg.Paths.SocketPath)
	if err != nil {
		return rpcResponse{}, unavailableError{stage: "connect", err: fmt.Errorf("connect to agent socket %s: %w", cfg.Paths.SocketPath, err)}
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return rpcResponse{}, fmt.Errorf("marshal agent request: %w", err)
	}
	if _, err := conn.Write(append(payload, '\n')); err != nil {
		return rpcResponse{}, fmt.Errorf("write agent request: %w", err)
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return rpcResponse{}, fmt.Errorf("read agent response: %w", err)
	}
	line = bytesTrimSpace(line)
	var resp rpcResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return rpcResponse{}, fmt.Errorf("decode agent response: %w", err)
	}
	if resp.Error != "" {
		return rpcResponse{}, serverResponseError{message: resp.Error}
	}
	return resp, nil
}

func ensureAgentReady(ctx context.Context, cfg Config) error {
	if err := os.MkdirAll(cfg.Paths.SupportDir, 0o700); err != nil {
		return fmt.Errorf("create agent support directory: %w", err)
	}
	executablePath, err := cfg.ExecutablePath()
	if err != nil {
		return err
	}
	changed, _, err := ensureLaunchAgentPlist(cfg.Paths, cfg.Label, executablePath)
	if err != nil {
		return err
	}
	serviceTarget := launchAgentServiceTarget(cfg.Label)
	if changed {
		_ = cfg.Launchd.Bootout(ctx, serviceTarget)
		if err := cfg.Launchd.Bootstrap(ctx, launchAgentDomainTarget(), cfg.Paths.PlistPath); err != nil {
			return fmt.Errorf("bootstrap LaunchAgent %s: %w", cfg.Label, err)
		}
	} else {
		if _, printErr := cfg.Launchd.Print(ctx, serviceTarget); printErr == nil {
			if err := cfg.Launchd.Kickstart(ctx, serviceTarget); err != nil {
				return fmt.Errorf("kickstart LaunchAgent %s: %w", cfg.Label, err)
			}
		} else {
			if err := cfg.Launchd.Bootstrap(ctx, launchAgentDomainTarget(), cfg.Paths.PlistPath); err != nil {
				return fmt.Errorf("bootstrap LaunchAgent %s: %w", cfg.Label, err)
			}
		}
	}
	if err := waitForReady(ctx, cfg); err != nil {
		return err
	}
	return nil
}

func waitForReady(ctx context.Context, cfg Config) error {
	deadline := time.Now().Add(5 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok {
		deadline = ctxDeadline
	}
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for LaunchAgent %s to become ready", cfg.Label)
		}
		pingCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		_, err := doRPC(pingCtx, cfg, rpcRequest{Method: "ping"})
		cancel()
		if err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func durationMillis(value time.Duration) int64 {
	if value <= 0 {
		return 0
	}
	return value.Milliseconds()
}

func isUnavailable(err error) bool {
	var unavailable unavailableError
	if errors.As(err, &unavailable) {
		return true
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	text := err.Error()
	return strings.Contains(text, "no such file or directory") || strings.Contains(text, "connection refused")
}

func bytesTrimSpace(data []byte) []byte {
	for len(data) > 0 && (data[0] == ' ' || data[0] == '\n' || data[0] == '\r' || data[0] == '\t') {
		data = data[1:]
	}
	for len(data) > 0 {
		last := data[len(data)-1]
		if last != ' ' && last != '\n' && last != '\r' && last != '\t' {
			break
		}
		data = data[:len(data)-1]
	}
	return data
}
