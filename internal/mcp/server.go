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
	"sync"
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

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

type cancelParams struct {
	RequestID any `json:"requestId"`
}

type inFlightRequest struct {
	cancel    context.CancelFunc
	cancelled bool
}

type stdioServer struct {
	ctx     context.Context
	cfg     ServerConfig
	handler ServerHandler

	writeMu  sync.Mutex
	requests sync.Mutex
	inFlight map[string]*inFlightRequest
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

	serverCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	s := &stdioServer{
		ctx:      serverCtx,
		cfg:      cfg,
		handler:  handler,
		inFlight: map[string]*inFlightRequest{},
	}

	reader := bufio.NewReader(cfg.In)
	for {
		env, raw, eof, err := readServerEnvelope(reader)
		if err != nil {
			s.cancelAllRequests()
			return err
		}
		if eof {
			s.cancelAllRequests()
			return nil
		}
		if cfg.Debug {
			fmt.Fprintf(cfg.ErrOut, "[debug] mcp serve recv <- %s\n", raw)
		}
		if err := s.handleEnvelope(env); err != nil {
			s.cancelAllRequests()
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

func (s *stdioServer) handleEnvelope(env rpcEnvelope) error {
	if env.Method == "" {
		if !hasID(env.ID) {
			return nil
		}
		return s.writeResponse(errorResponse(env.ID, -32600, "Invalid Request"))
	}
	if !hasID(env.ID) {
		s.handleNotification(env)
		return nil
	}

	switch env.Method {
	case "initialize":
		return s.writeResponse(s.initializeResponse(env))
	case "tools/list", "tools/call":
		return s.startAsyncRequest(env)
	default:
		return s.writeResponse(errorResponse(env.ID, -32601, "Method not found"))
	}
}

func (s *stdioServer) handleNotification(env rpcEnvelope) {
	switch env.Method {
	case "notifications/cancelled":
		requestKey, err := canonicalRequestKeyFromCancelParams(env.Params)
		if err != nil {
			if s.cfg.Debug {
				fmt.Fprintf(s.cfg.ErrOut, "[debug] mcp serve ignored malformed cancellation: %v\n", err)
			}
			return
		}
		if !s.cancelRequest(requestKey) && s.cfg.Debug {
			fmt.Fprintf(s.cfg.ErrOut, "[debug] mcp serve cancellation ignored for unknown request: %s\n", requestKey)
		}
	default:
		if s.cfg.Debug {
			fmt.Fprintf(s.cfg.ErrOut, "[debug] mcp serve notification ignored: %s\n", env.Method)
		}
	}
}

func (s *stdioServer) initializeResponse(env rpcEnvelope) map[string]any {
	params, err := decodeInitializeParams(env.Params)
	if err != nil {
		return errorResponse(env.ID, -32602, err.Error())
	}
	if !isSupportedVersion(params.ProtocolVersion) {
		return errorResponseWithData(env.ID, -32602, "Unsupported protocol version", map[string]any{
			"requested": params.ProtocolVersion,
			"supported": supportedVersions(),
		})
	}
	return successResponse(env.ID, map[string]any{
		"protocolVersion": params.ProtocolVersion,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    s.cfg.ServerName,
			"version": s.cfg.ServerVersion,
		},
	})
}

func decodeInitializeParams(raw json.RawMessage) (initializeParams, error) {
	var params initializeParams
	if len(bytes.TrimSpace(raw)) == 0 {
		return initializeParams{}, errors.New("initialize params require protocolVersion")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return initializeParams{}, errors.New("initialize params must be a JSON object")
	}
	if strings.TrimSpace(params.ProtocolVersion) == "" {
		return initializeParams{}, errors.New("initialize params require protocolVersion")
	}
	return params, nil
}

func (s *stdioServer) startAsyncRequest(env rpcEnvelope) error {
	requestKey, err := canonicalRequestKeyFromRaw(env.ID)
	if err != nil {
		return s.writeResponse(errorResponse(env.ID, -32600, err.Error()))
	}
	reqCtx, cancel := context.WithCancel(s.ctx)
	if !s.registerRequest(requestKey, cancel) {
		cancel()
		return s.writeResponse(errorResponse(env.ID, -32600, "request id is already in progress"))
	}
	go s.runAsyncRequest(reqCtx, requestKey, env)
	return nil
}

func (s *stdioServer) runAsyncRequest(ctx context.Context, requestKey string, env rpcEnvelope) {
	response := s.processRequest(ctx, env)
	if !s.finishRequest(requestKey) {
		return
	}
	if response == nil {
		return
	}
	if err := s.writeResponse(response); err != nil && s.cfg.Debug {
		fmt.Fprintf(s.cfg.ErrOut, "[debug] mcp serve write failed: %v\n", err)
	}
}

func (s *stdioServer) processRequest(ctx context.Context, env rpcEnvelope) map[string]any {
	switch env.Method {
	case "tools/list":
		tools, err := s.handler.ListTools(ctx)
		if err != nil {
			return errorResponse(env.ID, -32603, err.Error())
		}
		return successResponse(env.ID, map[string]any{"tools": tools})
	case "tools/call":
		name, arguments, err := decodeToolCallParams(env.Params)
		if err != nil {
			return errorResponse(env.ID, -32602, err.Error())
		}
		result, err := s.handler.CallTool(ctx, name, arguments)
		if err != nil {
			return errorResponse(env.ID, -32603, err.Error())
		}
		payload := map[string]any{}
		for key, value := range result.Result {
			payload[key] = value
		}
		if result.IsError {
			payload["isError"] = true
		}
		return successResponse(env.ID, payload)
	default:
		return errorResponse(env.ID, -32601, "Method not found")
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

func canonicalRequestKeyFromRaw(raw json.RawMessage) (string, error) {
	if !hasID(raw) {
		return "", errors.New("request id must be a JSON string or number")
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("decode request id: %w", err)
	}
	return canonicalRequestKey(value)
}

func canonicalRequestKeyFromCancelParams(raw json.RawMessage) (string, error) {
	var params cancelParams
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", errors.New("notifications/cancelled params require requestId")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", errors.New("notifications/cancelled params must be a JSON object")
	}
	if params.RequestID == nil {
		return "", errors.New("notifications/cancelled params require requestId")
	}
	return canonicalRequestKey(params.RequestID)
}

func canonicalRequestKey(value any) (string, error) {
	switch value.(type) {
	case string, float64:
		data, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("encode request id: %w", err)
		}
		return string(data), nil
	default:
		return "", errors.New("request id must be a JSON string or number")
	}
}

func (s *stdioServer) registerRequest(requestKey string, cancel context.CancelFunc) bool {
	s.requests.Lock()
	defer s.requests.Unlock()
	if _, exists := s.inFlight[requestKey]; exists {
		return false
	}
	s.inFlight[requestKey] = &inFlightRequest{cancel: cancel}
	return true
}

func (s *stdioServer) finishRequest(requestKey string) bool {
	s.requests.Lock()
	defer s.requests.Unlock()
	request, ok := s.inFlight[requestKey]
	if !ok {
		return false
	}
	delete(s.inFlight, requestKey)
	return !request.cancelled
}

func (s *stdioServer) cancelRequest(requestKey string) bool {
	s.requests.Lock()
	request, ok := s.inFlight[requestKey]
	if ok {
		request.cancelled = true
	}
	s.requests.Unlock()
	if !ok {
		return false
	}
	request.cancel()
	return true
}

func (s *stdioServer) cancelAllRequests() {
	s.requests.Lock()
	requests := make([]*inFlightRequest, 0, len(s.inFlight))
	for key, request := range s.inFlight {
		request.cancelled = true
		requests = append(requests, request)
		delete(s.inFlight, key)
	}
	s.requests.Unlock()
	for _, request := range requests {
		request.cancel()
	}
}

func successResponse(id json.RawMessage, result any) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(id),
		"result":  result,
	}
}

func errorResponse(id json.RawMessage, code int, message string) map[string]any {
	return errorResponseWithData(id, code, message, nil)
}

func errorResponseWithData(id json.RawMessage, code int, message string, data any) map[string]any {
	errorPayload := map[string]any{
		"code":    code,
		"message": message,
	}
	if data != nil {
		errorPayload["data"] = data
	}
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(id),
		"error":   errorPayload,
	}
}

func (s *stdioServer) writeResponse(payload map[string]any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return writeServerEnvelope(s.cfg.Out, s.cfg.ErrOut, s.cfg.Debug, payload)
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
