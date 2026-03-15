package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
)

func TestServeStdioRespondsToInitializeListAndCall(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	var stderr bytes.Buffer
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeStdio(context.Background(), ServerConfig{
			In:            inReader,
			Out:           outWriter,
			ErrOut:        &stderr,
			ServerName:    "xcodecli",
			ServerVersion: "test",
		}, ServerHandler{
			ListTools: func(ctx context.Context) ([]map[string]any, error) {
				return []map[string]any{{"name": "BuildProject"}}, nil
			},
			CallTool: func(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
				return CallResult{Result: map[string]any{
					"echoName":      name,
					"echoArguments": arguments,
				}}, nil
			},
		})
		_ = outWriter.Close()
	}()

	reader := bufio.NewReader(outReader)
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{"protocolVersion": requestProtocolVersion},
	})
	initResp := readServerResponse(t, reader)
	if got := decodeObjectValue(t, initResp["result"])["protocolVersion"]; got != requestProtocolVersion {
		t.Fatalf("protocolVersion = %#v, want %q", got, requestProtocolVersion)
	}

	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	})
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	listResp := readServerResponse(t, reader)
	tools := decodeObjectValue(t, listResp["result"])["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}

	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "BuildProject",
			"arguments": map[string]any{"scheme": "Demo"},
		},
	})
	callResp := readServerResponse(t, reader)
	result := decodeObjectValue(t, callResp["result"])
	if result["echoName"] != "BuildProject" {
		t.Fatalf("echoName = %#v, want BuildProject", result["echoName"])
	}

	_ = inWriter.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("ServeStdio returned error: %v (stderr=%q)", err, stderr.String())
	}
}

func TestServeStdioNegotiatesSupportedInitializeVersion(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeStdio(context.Background(), ServerConfig{
			In:            inReader,
			Out:           outWriter,
			ServerName:    "xcodecli",
			ServerVersion: "test",
		}, ServerHandler{
			ListTools: func(ctx context.Context) ([]map[string]any, error) { return nil, nil },
			CallTool: func(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
				return CallResult{}, nil
			},
		})
		_ = outWriter.Close()
	}()

	reader := bufio.NewReader(outReader)
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{"protocolVersion": "2025-03-26"},
	})
	resp := readServerResponse(t, reader)
	if got := decodeObjectValue(t, resp["result"])["protocolVersion"]; got != "2025-03-26" {
		t.Fatalf("protocolVersion = %#v, want %q", got, "2025-03-26")
	}

	_ = inWriter.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
}

func TestServeStdioRejectsUnsupportedInitializeVersion(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeStdio(context.Background(), ServerConfig{
			In:  inReader,
			Out: outWriter,
		}, ServerHandler{
			ListTools: func(ctx context.Context) ([]map[string]any, error) { return nil, nil },
			CallTool: func(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
				return CallResult{}, nil
			},
		})
		_ = outWriter.Close()
	}()

	reader := bufio.NewReader(outReader)
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{"protocolVersion": "2099-01-01"},
	})
	resp := readServerResponse(t, reader)
	errObj := decodeObjectValue(t, resp["error"])
	if int(errObj["code"].(float64)) != -32602 {
		t.Fatalf("error code = %#v, want -32602", errObj["code"])
	}
	data := decodeObjectValue(t, errObj["data"])
	if data["requested"] != "2099-01-01" {
		t.Fatalf("requested = %#v, want 2099-01-01", data["requested"])
	}
	supported := data["supported"].([]any)
	if len(supported) != 3 {
		t.Fatalf("len(supported) = %d, want 3", len(supported))
	}

	_ = inWriter.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
}

func TestServeStdioRejectsInitializeWithoutProtocolVersion(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeStdio(context.Background(), ServerConfig{
			In:  inReader,
			Out: outWriter,
		}, ServerHandler{
			ListTools: func(ctx context.Context) ([]map[string]any, error) { return nil, nil },
			CallTool: func(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
				return CallResult{}, nil
			},
		})
		_ = outWriter.Close()
	}()

	reader := bufio.NewReader(outReader)
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	resp := readServerResponse(t, reader)
	errObj := decodeObjectValue(t, resp["error"])
	if int(errObj["code"].(float64)) != -32602 {
		t.Fatalf("error code = %#v, want -32602", errObj["code"])
	}

	_ = inWriter.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
}

func TestServeStdioCancellationSuppressesResponseAndKeepsServerAlive(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	callStarted := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeStdio(context.Background(), ServerConfig{
			In:            inReader,
			Out:           outWriter,
			ServerName:    "xcodecli",
			ServerVersion: "test",
		}, ServerHandler{
			ListTools: func(ctx context.Context) ([]map[string]any, error) {
				return []map[string]any{{"name": "BuildProject"}}, nil
			},
			CallTool: func(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
				close(callStarted)
				<-ctx.Done()
				return CallResult{}, ctx.Err()
			},
		})
		_ = outWriter.Close()
	}()

	reader := bufio.NewReader(outReader)
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      "call-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "BuildProject",
			"arguments": map[string]any{"tabIdentifier": "demo"},
		},
	})
	<-callStarted
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/cancelled",
		"params":  map[string]any{"requestId": "call-1", "reason": "client abort"},
	})
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})

	resp := readServerResponse(t, reader)
	if got := int(resp["id"].(float64)); got != 2 {
		t.Fatalf("response id = %v, want 2", resp["id"])
	}

	_ = inWriter.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}

	line, err := reader.ReadString('\n')
	if err != io.EOF {
		t.Fatalf("expected EOF after cancellation, got line=%q err=%v", line, err)
	}
	if line != "" {
		t.Fatalf("unexpected extra response after cancellation: %q", line)
	}
}

func TestServeStdioCancellationSupportsNumericRequestID(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	callStarted := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeStdio(context.Background(), ServerConfig{
			In:            inReader,
			Out:           outWriter,
			ServerName:    "xcodecli",
			ServerVersion: "test",
		}, ServerHandler{
			ListTools: func(ctx context.Context) ([]map[string]any, error) {
				return []map[string]any{{"name": "BuildProject"}}, nil
			},
			CallTool: func(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
				close(callStarted)
				<-ctx.Done()
				return CallResult{}, ctx.Err()
			},
		})
		_ = outWriter.Close()
	}()

	reader := bufio.NewReader(outReader)
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      7,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "BuildProject",
			"arguments": map[string]any{"tabIdentifier": "demo"},
		},
	})
	<-callStarted
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/cancelled",
		"params":  map[string]any{"requestId": 7, "reason": "client abort"},
	})
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      8,
		"method":  "tools/list",
		"params":  map[string]any{},
	})

	resp := readServerResponse(t, reader)
	if got := int(resp["id"].(float64)); got != 8 {
		t.Fatalf("response id = %v, want 8", resp["id"])
	}

	_ = inWriter.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}

	line, err := reader.ReadString('\n')
	if err != io.EOF {
		t.Fatalf("expected EOF after numeric cancellation, got line=%q err=%v", line, err)
	}
	if line != "" {
		t.Fatalf("unexpected extra response after numeric cancellation: %q", line)
	}
}

func TestServeStdioIgnoresMalformedAndUnknownCancellation(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	callStarted := make(chan struct{})
	releaseCall := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeStdio(context.Background(), ServerConfig{
			In:            inReader,
			Out:           outWriter,
			ServerName:    "xcodecli",
			ServerVersion: "test",
		}, ServerHandler{
			ListTools: func(ctx context.Context) ([]map[string]any, error) {
				return nil, nil
			},
			CallTool: func(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
				close(callStarted)
				select {
				case <-ctx.Done():
					return CallResult{}, ctx.Err()
				case <-releaseCall:
					return CallResult{Result: map[string]any{"ok": true}}, nil
				}
			},
		})
		_ = outWriter.Close()
	}()

	reader := bufio.NewReader(outReader)
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      "call-x",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "BuildProject",
			"arguments": map[string]any{"tabIdentifier": "demo"},
		},
	})
	<-callStarted
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/cancelled",
		"params":  "not-an-object",
	})
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/cancelled",
		"params":  map[string]any{"requestId": "unknown-id"},
	})
	close(releaseCall)

	resp := readServerResponse(t, reader)
	if got := resp["id"].(string); got != "call-x" {
		t.Fatalf("response id = %v, want call-x", resp["id"])
	}
	result := decodeObjectValue(t, resp["result"])
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("unexpected result after ignored cancellation: %+v", result)
	}

	_ = inWriter.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
}

func TestServeStdioHandlesMultipleRequestsAfterCancellation(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	callStarted := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeStdio(context.Background(), ServerConfig{
			In:            inReader,
			Out:           outWriter,
			ServerName:    "xcodecli",
			ServerVersion: "test",
		}, ServerHandler{
			ListTools: func(ctx context.Context) ([]map[string]any, error) {
				return []map[string]any{{"name": "BuildProject"}}, nil
			},
			CallTool: func(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
				close(callStarted)
				<-ctx.Done()
				return CallResult{}, ctx.Err()
			},
		})
		_ = outWriter.Close()
	}()

	reader := bufio.NewReader(outReader)
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      "call-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "BuildProject",
			"arguments": map[string]any{"tabIdentifier": "demo"},
		},
	})
	<-callStarted
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/cancelled",
		"params":  map[string]any{"requestId": "call-1"},
	})
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/list",
		"params":  map[string]any{},
	})

	resp1 := readServerResponse(t, reader)
	resp2 := readServerResponse(t, reader)
	seen := map[int]bool{}
	for _, resp := range []map[string]any{resp1, resp2} {
		seen[int(resp["id"].(float64))] = true
	}
	if !seen[2] || !seen[3] {
		t.Fatalf("expected responses for ids 2 and 3, got %+v %+v", resp1, resp2)
	}

	_ = inWriter.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}

	line, err := reader.ReadString('\n')
	if err != io.EOF {
		t.Fatalf("expected EOF after sequential requests, got line=%q err=%v", line, err)
	}
	if line != "" {
		t.Fatalf("unexpected extra response after sequential requests: %q", line)
	}
}

func TestServeStdioUnknownMethodReturnsMethodNotFound(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	defer inWriter.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ServeStdio(context.Background(), ServerConfig{
			In:  inReader,
			Out: outWriter,
		}, ServerHandler{
			ListTools: func(ctx context.Context) ([]map[string]any, error) { return nil, nil },
			CallTool: func(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
				return CallResult{}, nil
			},
		})
		_ = outWriter.Close()
	}()

	reader := bufio.NewReader(outReader)
	writeServerRequest(t, inWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "bogus",
		"params":  map[string]any{},
	})
	resp := readServerResponse(t, reader)
	errObj := decodeObjectValue(t, resp["error"])
	if int(errObj["code"].(float64)) != -32601 {
		t.Fatalf("error code = %#v, want -32601", errObj["code"])
	}

	_ = inWriter.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
}

func writeServerRequest(t *testing.T, w io.Writer, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if _, err := w.Write(append(data, '\n')); err != nil {
		t.Fatalf("write request: %v", err)
	}
}

func readServerResponse(t *testing.T, reader *bufio.Reader) map[string]any {
	t.Helper()
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(line), &value); err != nil {
		t.Fatalf("decode response: %v (line=%q)", err, line)
	}
	return value
}

func decodeObjectValue(t *testing.T, value any) map[string]any {
	t.Helper()
	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %#v", value)
	}
	return obj
}
