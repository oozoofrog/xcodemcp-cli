package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oozoofrog/xcodecli/internal/bridge"
	"github.com/oozoofrog/xcodecli/internal/mcp"
)

type Config struct {
	Paths          Paths
	Label          string
	IdleTimeout    time.Duration
	Command        mcp.Command
	BaseEnv        []string
	ErrOut         io.Writer
	Launchd        Launchd
	DialContext    func(ctx context.Context, network, address string) (net.Conn, error)
	Listen         func(network, address string) (net.Listener, error)
	ExecutablePath func() (string, error)
}

type Request struct {
	XcodePID     string
	SessionID    string
	DeveloperDir string
	Timeout      time.Duration
	Debug        bool
}

type Status struct {
	Label             string        `json:"label"`
	PlistPath         string        `json:"plistPath"`
	PlistInstalled    bool          `json:"plistInstalled"`
	RegisteredBinary  string        `json:"registeredBinary"`
	CurrentBinary     string        `json:"currentBinary"`
	BinaryPathMatches bool          `json:"binaryPathMatches"`
	SocketPath        string        `json:"socketPath"`
	SocketReachable   bool          `json:"socketReachable"`
	Running           bool          `json:"running"`
	PID               int           `json:"pid"`
	IdleTimeout       time.Duration `json:"idleTimeout"`
	BackendSessions   int           `json:"backendSessions"`
}

type runtimeStatus struct {
	PID             int   `json:"pid"`
	IdleTimeoutMS   int64 `json:"idleTimeoutMs"`
	BackendSessions int   `json:"backendSessions"`
}

type rpcRequest struct {
	Method       string         `json:"method"`
	XcodePID     string         `json:"xcodePid,omitempty"`
	SessionID    string         `json:"sessionId,omitempty"`
	DeveloperDir string         `json:"developerDir,omitempty"`
	TimeoutMS    int64          `json:"timeoutMs,omitempty"`
	Debug        bool           `json:"debug,omitempty"`
	ToolName     string         `json:"toolName,omitempty"`
	Arguments    map[string]any `json:"arguments,omitempty"`
}

type rpcResponse struct {
	Error   string           `json:"error,omitempty"`
	Tools   []map[string]any `json:"tools,omitempty"`
	Result  map[string]any   `json:"result,omitempty"`
	IsError bool             `json:"isError,omitempty"`
	Status  *runtimeStatus   `json:"status,omitempty"`
}

func DefaultConfig(command mcp.Command, baseEnv []string, errOut io.Writer) (Config, error) {
	paths, err := DefaultPaths()
	if err != nil {
		return Config{}, err
	}
	if errOut == nil {
		errOut = io.Discard
	}
	return normalizeConfig(Config{
		Paths:          paths,
		Label:          LaunchAgentLabel,
		IdleTimeout:    DefaultIdleTimeout,
		Command:        command,
		BaseEnv:        append([]string{}, baseEnv...),
		ErrOut:         errOut,
		Launchd:        defaultLaunchd(),
		DialContext:    (&net.Dialer{}).DialContext,
		Listen:         net.Listen,
		ExecutablePath: resolvedExecutablePath,
	})
}

func normalizeConfig(cfg Config) (Config, error) {
	if cfg.Label == "" {
		cfg.Label = LaunchAgentLabel
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultIdleTimeout
	}
	if cfg.ErrOut == nil {
		cfg.ErrOut = io.Discard
	}
	if cfg.Launchd == nil {
		cfg.Launchd = defaultLaunchd()
	}
	if cfg.DialContext == nil {
		cfg.DialContext = (&net.Dialer{}).DialContext
	}
	if cfg.Listen == nil {
		cfg.Listen = net.Listen
	}
	if cfg.ExecutablePath == nil {
		cfg.ExecutablePath = resolvedExecutablePath
	}
	if cfg.Paths.SocketPath == "" || cfg.Paths.PlistPath == "" || cfg.Paths.PIDPath == "" || cfg.Paths.LogPath == "" || cfg.Paths.SupportDir == "" {
		paths, err := DefaultPaths()
		if err != nil {
			return Config{}, err
		}
		cfg.Paths = paths
	}
	return cfg, nil
}

func BuildRequest(env []string, effective bridge.EnvOptions, timeout time.Duration, debug bool) Request {
	return Request{
		XcodePID:     effective.XcodePID,
		SessionID:    effective.SessionID,
		DeveloperDir: envValue(env, "DEVELOPER_DIR"),
		Timeout:      timeout,
		Debug:        debug,
	}
}

func resolvedExecutablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	return filepath.Clean(path), nil
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func samePath(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	leftResolved, leftErr := filepath.EvalSymlinks(left)
	if leftErr != nil {
		leftResolved = filepath.Clean(left)
	}
	rightResolved, rightErr := filepath.EvalSymlinks(right)
	if rightErr != nil {
		rightResolved = filepath.Clean(right)
	}
	return leftResolved == rightResolved
}
