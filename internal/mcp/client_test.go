package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestListToolsAggregatesPagination(t *testing.T) {
	tools, err := ListTools(context.Background(), Config{
		Command: helperCommand("list-paged"),
		Env:     helperEnv(),
	})
	if err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}
	if len(tools) != 3 {
		t.Fatalf("len(tools) = %d, want 3", len(tools))
	}
	if got := toolField(tools[0], "name"); got != "build_sim" {
		t.Fatalf("first tool name = %q, want build_sim", got)
	}
	if got := toolField(tools[2], "name"); got != "launch_app_sim" {
		t.Fatalf("last tool name = %q, want launch_app_sim", got)
	}
}

func TestCallToolReturnsResultAndIsError(t *testing.T) {
	result, err := CallTool(context.Background(), Config{
		Command: helperCommand("call-success"),
		Env:     helperEnv(),
	}, "build_sim", map[string]any{"scheme": "Demo"})
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected successful tool result")
	}
	if got := toolField(result.Result, "echoName"); got != "build_sim" {
		t.Fatalf("echoName = %q, want build_sim", got)
	}
}

func TestCallToolRecognizesIsError(t *testing.T) {
	result, err := CallTool(context.Background(), Config{
		Command: helperCommand("call-is-error"),
		Env:     helperEnv(),
	}, "build_sim", map[string]any{"scheme": "Demo"})
	if err != nil {
		t.Fatalf("CallTool returned error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected isError result")
	}
}

func TestListToolsRejectsUnsupportedVersion(t *testing.T) {
	_, err := ListTools(context.Background(), Config{
		Command: helperCommand("invalid-version"),
		Env:     helperEnv(),
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported protocol version") {
		t.Fatalf("expected unsupported version error, got %v", err)
	}
}

func TestListToolsFailsOnServerRequest(t *testing.T) {
	_, err := ListTools(context.Background(), Config{
		Command: helperCommand("server-request"),
		Env:     helperEnv(),
	})
	if err == nil || !strings.Contains(err.Error(), "server request \"ping\" is not supported") {
		t.Fatalf("expected unsupported server request error, got %v", err)
	}
}

func TestListToolsTimesOut(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := ListTools(ctx, Config{
		Command: helperCommand("timeout"),
		Env:     helperEnv(),
	})
	if err == nil || !strings.Contains(err.Error(), "wait for tools/list response") {
		t.Fatalf("expected timeout-related error, got %v", err)
	}
}

func TestListToolsFailsOnMalformedJSON(t *testing.T) {
	_, err := ListTools(context.Background(), Config{
		Command: helperCommand("bad-json"),
		Env:     helperEnv(),
	})
	if err == nil || !strings.Contains(err.Error(), "decode JSON-RPC message") {
		t.Fatalf("expected malformed JSON error, got %v", err)
	}
}

func TestDebugLogsNotifications(t *testing.T) {
	var stderr bytes.Buffer
	_, err := ListTools(context.Background(), Config{
		Command: helperCommand("list-with-notification"),
		Env:     helperEnv(),
		Debug:   true,
		ErrOut:  &stderr,
	})
	if err != nil {
		t.Fatalf("ListTools returned error: %v", err)
	}
	if !strings.Contains(stderr.String(), "server notification ignored") {
		t.Fatalf("stderr missing ignored notification debug log: %q", stderr.String())
	}
}

func helperCommand(mode string) Command {
	return Command{Path: os.Args[0], Args: []string{"-test.run=TestHelperProcess", "--", mode}}
}

func helperEnv() []string {
	return append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
}

func toolField(obj map[string]any, key string) string {
	value, _ := obj[key].(string)
	return value
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	server := newHelperServer(t)
	mode := helperMode(t)

	switch mode {
	case "list-paged":
		server.expectInitialize("2025-06-18")
		server.expectInitialized()
		server.expectToolsList(2, "")
		server.write(map[string]any{"jsonrpc": "2.0", "id": 2, "result": map[string]any{"tools": []map[string]any{{"name": "build_sim", "description": "Build for simulator"}, {"name": "test_sim"}}, "nextCursor": "page-2"}})
		server.expectToolsList(3, "page-2")
		server.write(map[string]any{"jsonrpc": "2.0", "id": 3, "result": map[string]any{"tools": []map[string]any{{"name": "launch_app_sim", "description": "Launch app"}}}})
	case "call-success":
		server.expectInitialize("2025-06-18")
		server.expectInitialized()
		request := server.expectRequest(2, "tools/call")
		params := decodeObject(t, request["params"])
		server.write(map[string]any{"jsonrpc": "2.0", "id": 2, "result": map[string]any{"content": []map[string]any{{"type": "text", "text": "ok"}}, "echoName": params["name"], "echoArguments": params["arguments"]}})
	case "call-is-error":
		server.expectInitialize("2025-06-18")
		server.expectInitialized()
		server.expectRequest(2, "tools/call")
		server.write(map[string]any{"jsonrpc": "2.0", "id": 2, "result": map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": "failed"}}}})
	case "invalid-version":
		server.expectRequest(1, "initialize")
		server.write(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"protocolVersion": "2099-01-01"}})
	case "server-request":
		server.expectInitialize("2025-06-18")
		server.expectInitialized()
		server.expectToolsList(2, "")
		server.write(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "ping", "params": map[string]any{}})
		response := server.read()
		errObj := decodeObject(t, response["error"])
		if code := int(errObj["code"].(float64)); code != -32601 {
			t.Fatalf("method not found code = %d, want -32601", code)
		}
	case "timeout":
		server.expectInitialize("2025-06-18")
		server.expectInitialized()
		server.expectToolsList(2, "")
		time.Sleep(2 * time.Second)
	case "bad-json":
		server.expectInitialize("2025-06-18")
		server.expectInitialized()
		server.expectToolsList(2, "")
		fmt.Fprintln(os.Stdout, "{bad-json")
	case "list-with-notification":
		server.expectInitialize("2025-06-18")
		server.expectInitialized()
		server.expectToolsList(2, "")
		server.write(map[string]any{"jsonrpc": "2.0", "method": "notifications/tools/list_changed", "params": map[string]any{"reason": "refresh"}})
		server.write(map[string]any{"jsonrpc": "2.0", "id": 2, "result": map[string]any{"tools": []map[string]any{{"name": "build_sim"}}}})
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}

	os.Exit(0)
}

type helperServer struct {
	t      *testing.T
	reader *bufio.Reader
}

func newHelperServer(t *testing.T) helperServer {
	return helperServer{t: t, reader: bufio.NewReader(os.Stdin)}
}

func helperMode(t *testing.T) string {
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

func (s helperServer) expectInitialize(version string) {
	request := s.expectRequest(1, "initialize")
	params := decodeObject(s.t, request["params"])
	if params["protocolVersion"] != version {
		s.t.Fatalf("protocolVersion = %#v, want %q", params["protocolVersion"], version)
	}
	s.write(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"protocolVersion": version}})
}

func (s helperServer) expectInitialized() {
	message := s.read()
	if method, _ := message["method"].(string); method != "notifications/initialized" {
		s.t.Fatalf("initialized notification method = %q", method)
	}
}

func (s helperServer) expectToolsList(id int, cursor string) {
	request := s.expectRequest(id, "tools/list")
	params := decodeObject(s.t, request["params"])
	if cursor == "" {
		if _, ok := params["cursor"]; ok {
			s.t.Fatalf("unexpected cursor in first page request: %#v", params)
		}
		return
	}
	if got, _ := params["cursor"].(string); got != cursor {
		s.t.Fatalf("cursor = %q, want %q", got, cursor)
	}
}

func (s helperServer) expectRequest(id int, method string) map[string]any {
	message := s.read()
	if got, _ := message["method"].(string); got != method {
		s.t.Fatalf("method = %q, want %q", got, method)
	}
	if got := int(message["id"].(float64)); got != id {
		s.t.Fatalf("id = %d, want %d", got, id)
	}
	return message
}

func (s helperServer) read() map[string]any {
	line, err := s.reader.ReadString('\n')
	if err != nil {
		s.t.Fatalf("read helper stdin: %v", err)
	}
	line = strings.TrimSpace(line)
	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		s.t.Fatalf("decode helper JSON: %v (line=%q)", err, line)
	}
	return msg
}

func (s helperServer) write(v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		s.t.Fatalf("marshal helper JSON: %v", err)
	}
	fmt.Fprintln(os.Stdout, string(payload))
}

func decodeObject(t *testing.T, value any) map[string]any {
	t.Helper()
	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %#v", value)
	}
	return obj
}
