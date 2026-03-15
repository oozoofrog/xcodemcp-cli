package main

import (
	"context"
	"fmt"
	"io"

	"github.com/oozoofrog/xcodecli/internal/agent"
	"github.com/oozoofrog/xcodecli/internal/bridge"
	"github.com/oozoofrog/xcodecli/internal/mcp"
)

type mcpServeFunc func(ctx context.Context, cfg mcp.ServerConfig, handler mcp.ServerHandler) error

func runServe(ctx context.Context, cfg cliConfig, env []string, stdin io.Reader, stdout, stderr io.Writer, agentCfg agent.Config) int {
	resolved, err := resolveEffectiveOptions(env, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "xcodecli: %v\n", err)
		return 1
	}
	if cfg.Debug {
		logResolvedSession(stderr, resolved)
	}
	effective := resolved.EnvOptions
	if err := bridge.ValidateEnvOptions(effective); err != nil {
		fmt.Fprintf(stderr, "xcodecli: invalid MCP options: %v\n", err)
		return 1
	}

	handler := mcp.ServerHandler{
		ListTools: func(reqCtx context.Context) ([]map[string]any, error) {
			return defaultToolsListFunc(reqCtx, agentCfg, agent.BuildRequest(env, effective, 0, cfg.Debug))
		},
		CallTool: func(reqCtx context.Context, name string, arguments map[string]any) (mcp.CallResult, error) {
			return defaultToolCallFunc(reqCtx, agentCfg, agent.BuildRequest(env, effective, 0, cfg.Debug), name, arguments)
		},
	}
	if err := defaultMCPServeFunc(ctx, mcp.ServerConfig{
		In:            stdin,
		Out:           stdout,
		ErrOut:        stderr,
		Debug:         cfg.Debug,
		ServerName:    "xcodecli",
		ServerVersion: currentVersion(),
	}, handler); err != nil {
		fmt.Fprintf(stderr, "xcodecli: %v\n", err)
		return 1
	}
	return 0
}

func runServeMCP(ctx context.Context, cfg mcp.ServerConfig, handler mcp.ServerHandler) error {
	return mcp.ServeStdio(ctx, cfg, handler)
}
