package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type ServerConfig struct {
	In            io.Reader
	Out           io.Writer
	ErrOut        io.Writer
	Debug         bool
	ServerName    string
	ServerVersion string
}

type ServerHandler struct {
	ListTools func(ctx context.Context) ([]map[string]any, error)
	CallTool  func(ctx context.Context, name string, arguments map[string]any) (CallResult, error)
}

func ServeStdio(ctx context.Context, cfg ServerConfig, handler ServerHandler) error {
	if cfg.In == nil {
		return errors.New("missing server stdin")
	}
	if cfg.Out == nil {
		return errors.New("missing server stdout")
	}
	if handler.ListTools == nil {
		return errors.New("missing tools/list handler")
	}
	if handler.CallTool == nil {
		return errors.New("missing tools/call handler")
	}
	if cfg.ErrOut == nil {
		cfg.ErrOut = io.Discard
	}
	if strings.TrimSpace(cfg.ServerName) == "" {
		cfg.ServerName = "xcodecli"
	}
	if strings.TrimSpace(cfg.ServerVersion) == "" {
		cfg.ServerVersion = "dev"
	}

	reader := bufio.NewReader(cfg.In)
	for {
		env, raw, eof, err := readServerEnvelope(reader)
		if err != nil {
			return err
		}
		if eof {
			return nil
		}
		if cfg.Debug {
			fmt.Fprintf(cfg.ErrOut, "[debug] mcp serve recv <- %s\n", raw)
		}
		resp, ok := handleServerEnvelope(ctx, cfg, handler, env)
		if !ok {
			continue
		}
		if err := writeServerEnvelope(cfg.Out, cfg.ErrOut, cfg.Debug, resp); err != nil {
			return err
		}
	}
}

func readServerEnvelope(reader *bufio.Reader) (rpcEnvelope, string, bool, error) {
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				text := strings.TrimSpace(string(line))
				if text == "" {
					return rpcEnvelope{}, "", true, nil
				}
				var env rpcEnvelope
				if unmarshalErr := json.Unmarshal(bytes.TrimSpace(line), &env); unmarshalErr != nil {
					return rpcEnvelope{}, text, false, fmt.Errorf("decode MCP request: %w", unmarshalErr)
				}
				return env, text, false, nil
			}
			return rpcEnvelope{}, "", false, fmt.Errorf("read MCP request: %w", err)
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var env rpcEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			return rpcEnvelope{}, string(line), false, fmt.Errorf("decode MCP request: %w", err)
		}
		return env, string(line), false, nil
	}
}

func handleServerEnvelope(ctx context.Context, cfg ServerConfig, handler ServerHandler, env rpcEnvelope) (map[string]any, bool) {
	if env.Method == "" {
		if !hasID(env.ID) {
			return nil, false
		}
		return errorResponse(env.ID, -32600, "Invalid Request"), true
	}
	if !hasID(env.ID) {
		if cfg.Debug {
			fmt.Fprintf(cfg.ErrOut, "[debug] mcp serve notification ignored: %s\n", env.Method)
		}
		return nil, false
	}

	switch env.Method {
	case "initialize":
		return successResponse(env.ID, map[string]any{
			"protocolVersion": requestProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    cfg.ServerName,
				"version": cfg.ServerVersion,
			},
		}), true
	case "tools/list":
		tools, err := handler.ListTools(ctx)
		if err != nil {
			return errorResponse(env.ID, -32603, err.Error()), true
		}
		return successResponse(env.ID, map[string]any{"tools": tools}), true
	case "tools/call":
		name, arguments, err := decodeToolCallParams(env.Params)
		if err != nil {
			return errorResponse(env.ID, -32602, err.Error()), true
		}
		result, err := handler.CallTool(ctx, name, arguments)
		if err != nil {
			return errorResponse(env.ID, -32603, err.Error()), true
		}
		payload := map[string]any{}
		for key, value := range result.Result {
			payload[key] = value
		}
		if result.IsError {
			payload["isError"] = true
		}
		return successResponse(env.ID, payload), true
	default:
		return errorResponse(env.ID, -32601, "Method not found"), true
	}
}

func decodeToolCallParams(raw json.RawMessage) (string, map[string]any, error) {
	params := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return "", nil, errors.New("tools/call params must be a JSON object")
		}
	}
	name, _ := params["name"].(string)
	if strings.TrimSpace(name) == "" {
		return "", nil, errors.New("tools/call params require a non-empty name")
	}
	arguments := map[string]any{}
	if value, ok := params["arguments"]; ok && value != nil {
		obj, ok := value.(map[string]any)
		if !ok {
			return "", nil, errors.New("tools/call params.arguments must be a JSON object")
		}
		arguments = obj
	}
	return name, arguments, nil
}

func successResponse(id json.RawMessage, result any) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(id),
		"result":  result,
	}
}

func errorResponse(id json.RawMessage, code int, message string) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(id),
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
}

func writeServerEnvelope(out io.Writer, errOut io.Writer, debug bool, payload map[string]any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal MCP response: %w", err)
	}
	if debug {
		fmt.Fprintf(errOut, "[debug] mcp serve send -> %s\n", string(data))
	}
	if _, err := out.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write MCP response: %w", err)
	}
	return nil
}
