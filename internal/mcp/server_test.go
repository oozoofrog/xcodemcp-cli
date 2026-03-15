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
