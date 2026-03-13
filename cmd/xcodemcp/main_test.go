package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oozoofrog/xcodemcp-cli/internal/bridge"
	"github.com/oozoofrog/xcodemcp-cli/internal/mcp"
)

func TestParseCLIDefaultBridge(t *testing.T) {
	cfg, usage, err := parseCLI([]string{"--xcode-pid", "123", "--session-id", "11111111-1111-1111-1111-111111111111", "--debug"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandBridge {
		t.Fatalf("command = %q, want %q", cfg.Command, commandBridge)
	}
	if cfg.XcodePID != "123" || cfg.SessionID != "11111111-1111-1111-1111-111111111111" || !cfg.Debug {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if !strings.Contains(usage, "xcodemcp bridge") {
		t.Fatalf("usage missing bridge help: %q", usage)
	}
}

func TestParseCLIToolsList(t *testing.T) {
	cfg, _, err := parseCLI([]string{"tools", "list", "--json", "--timeout", "45s"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandToolsList {
		t.Fatalf("command = %q, want %q", cfg.Command, commandToolsList)
	}
	if !cfg.JSONOutput || cfg.Timeout != 45*time.Second {
		t.Fatalf("unexpected tools list config: %+v", cfg)
	}
}

func TestParseCLIToolCall(t *testing.T) {
	cfg, _, err := parseCLI([]string{"tool", "call", "build_sim", "--json", `{"scheme":"Demo"}`, "--timeout", "15s"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandToolCall {
		t.Fatalf("command = %q, want %q", cfg.Command, commandToolCall)
	}
	if cfg.ToolName != "build_sim" || cfg.ToolInputJSON != `{"scheme":"Demo"}` || cfg.Timeout != 15*time.Second {
		t.Fatalf("unexpected tool call config: %+v", cfg)
	}
}

func TestParseCLIHelp(t *testing.T) {
	_, usage, err := parseCLI([]string{"help", "tool", "call"})
	if err != errUsageRequested {
		t.Fatalf("err = %v, want errUsageRequested", err)
	}
	if !strings.Contains(usage, "tool call") {
		t.Fatalf("usage missing tool call help: %q", usage)
	}
}

func TestParseCLIUnknownCommand(t *testing.T) {
	_, _, err := parseCLI([]string{"unknown"})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}
}

func TestRunRejectsInvalidBridgeOptions(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder
	code := run(context.Background(), []string{"--xcode-pid", "0"}, strings.NewReader(""), &stdout, &stderr, []string{})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "invalid bridge options") {
		t.Fatalf("stderr = %q, want invalid options message", stderr.String())
	}
}

func TestRunToolsListJSON(t *testing.T) {
	oldCommand := defaultMCPCommand
	defaultMCPCommand = mcp.Command{Path: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess", "--", "list-basic"}}
	defer func() { defaultMCPCommand = oldCommand }()

	var stdout strings.Builder
	var stderr strings.Builder
	code := run(context.Background(), []string{"tools", "list", "--json"}, strings.NewReader(""), &stdout, &stderr, append(os.Environ(), "GO_WANT_MCP_HELPER_PROCESS=1"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	var tools []map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &tools); err != nil {
		t.Fatalf("stdout is not JSON array: %v (stdout=%q)", err, stdout.String())
	}
	if len(tools) != 2 {
		t.Fatalf("len(tools) = %d, want 2", len(tools))
	}
}

func TestRunToolsListGeneratesPersistentSessionID(t *testing.T) {
	oldCommand := defaultMCPCommand
	oldSessionPathFunc := defaultSessionPathFunc
	sessionPath := filepath.Join(t.TempDir(), "session-id")
	defaultMCPCommand = mcp.Command{Path: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess", "--", "list-env"}}
	defaultSessionPathFunc = func() (string, error) { return sessionPath, nil }
	defer func() {
		defaultMCPCommand = oldCommand
		defaultSessionPathFunc = oldSessionPathFunc
	}()

	var stdout strings.Builder
	var stderr strings.Builder
	code := run(context.Background(), []string{"tools", "list", "--json", "--debug"}, strings.NewReader(""), &stdout, &stderr, append(os.Environ(), "GO_WANT_MCP_HELPER_PROCESS=1"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) failed: %v", sessionPath, err)
	}
	sessionID := strings.TrimSpace(string(data))
	if !bridge.IsValidUUID(sessionID) {
		t.Fatalf("persisted session ID is invalid: %q", sessionID)
	}
	if !strings.Contains(stderr.String(), "generated persistent MCP_XCODE_SESSION_ID "+sessionID) {
		t.Fatalf("stderr = %q, want generated session debug log", stderr.String())
	}
}

func TestRunToolsListReusesPersistentSessionID(t *testing.T) {
	oldCommand := defaultMCPCommand
	oldSessionPathFunc := defaultSessionPathFunc
	sessionPath := filepath.Join(t.TempDir(), "session-id")
	wantSessionID := "44444444-4444-4444-8444-444444444444"
	if err := os.WriteFile(sessionPath, []byte(wantSessionID+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	defaultMCPCommand = mcp.Command{Path: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess", "--", "list-env"}}
	defaultSessionPathFunc = func() (string, error) { return sessionPath, nil }
	defer func() {
		defaultMCPCommand = oldCommand
		defaultSessionPathFunc = oldSessionPathFunc
	}()

	var stdout strings.Builder
	var stderr strings.Builder
	env := append(os.Environ(),
		"GO_WANT_MCP_HELPER_PROCESS=1",
		"EXPECT_SESSION_ID="+wantSessionID,
	)
	code := run(context.Background(), []string{"tools", "list", "--json", "--debug"}, strings.NewReader(""), &stdout, &stderr, env)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "using persisted MCP_XCODE_SESSION_ID "+wantSessionID) {
		t.Fatalf("stderr = %q, want persisted session debug log", stderr.String())
	}
}

func TestRunToolCallIsErrorExitsOne(t *testing.T) {
	oldCommand := defaultMCPCommand
	defaultMCPCommand = mcp.Command{Path: os.Args[0], Args: []string{"-test.run=TestMCPHelperProcess", "--", "call-error"}}
	defer func() { defaultMCPCommand = oldCommand }()

	var stdout strings.Builder
	var stderr strings.Builder
	code := run(context.Background(), []string{"tool", "call", "build_sim", "--json", `{"scheme":"Demo"}`}, strings.NewReader(""), &stdout, &stderr, append(os.Environ(), "GO_WANT_MCP_HELPER_PROCESS=1"))
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), `"isError": true`) {
		t.Fatalf("stdout = %q, want tool result JSON", stdout.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty stderr", stderr.String())
	}
}

func TestRunRejectsNonObjectToolJSON(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder
	code := run(context.Background(), []string{"tool", "call", "build_sim", "--json", `[]`}, strings.NewReader(""), &stdout, &stderr, os.Environ())
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "must decode to a JSON object") {
		t.Fatalf("stderr = %q, want JSON object error", stderr.String())
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER_PROCESS") != "1" {
		return
	}

	server := mainHelperServer{t: t, reader: bufio.NewReader(os.Stdin)}
	mode := helperMode(t)
	switch mode {
	case "list-basic":
		server.expectInitialize()
		server.expectInitialized()
		server.expectRequest(2, "tools/list")
		server.write(map[string]any{"jsonrpc": "2.0", "id": 2, "result": map[string]any{"tools": []map[string]any{{"name": "build_sim", "description": "Build for simulator"}, {"name": "launch_app_sim"}}}})
	case "list-env":
		gotSessionID := os.Getenv("MCP_XCODE_SESSION_ID")
		if expected := os.Getenv("EXPECT_SESSION_ID"); expected != "" {
			if gotSessionID != expected {
				t.Fatalf("MCP_XCODE_SESSION_ID = %q, want %q", gotSessionID, expected)
			}
		} else if !bridge.IsValidUUID(gotSessionID) {
			t.Fatalf("MCP_XCODE_SESSION_ID is not a valid UUID: %q", gotSessionID)
		}
		server.expectInitialize()
		server.expectInitialized()
		server.expectRequest(2, "tools/list")
		server.write(map[string]any{"jsonrpc": "2.0", "id": 2, "result": map[string]any{"tools": []map[string]any{{"name": "list_windows"}}}})
	case "call-error":
		server.expectInitialize()
		server.expectInitialized()
		server.expectRequest(2, "tools/call")
		server.write(map[string]any{"jsonrpc": "2.0", "id": 2, "result": map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": "boom"}}}})
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
	os.Exit(0)
}

type mainHelperServer struct {
	t      *testing.T
	reader *bufio.Reader
}

func (s mainHelperServer) expectInitialize() {
	request := s.expectRequest(1, "initialize")
	params := decodeMainObject(s.t, request["params"])
	if params["protocolVersion"] != "2025-06-18" {
		s.t.Fatalf("protocolVersion = %#v, want 2025-06-18", params["protocolVersion"])
	}
	s.write(map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"protocolVersion": "2025-06-18"}})
}

func (s mainHelperServer) expectInitialized() {
	message := s.read()
	if method, _ := message["method"].(string); method != "notifications/initialized" {
		s.t.Fatalf("initialized notification method = %q", method)
	}
}

func (s mainHelperServer) expectRequest(id int, method string) map[string]any {
	message := s.read()
	if got, _ := message["method"].(string); got != method {
		s.t.Fatalf("method = %q, want %q", got, method)
	}
	if got := int(message["id"].(float64)); got != id {
		s.t.Fatalf("id = %d, want %d", got, id)
	}
	return message
}

func (s mainHelperServer) read() map[string]any {
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

func (s mainHelperServer) write(v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		s.t.Fatalf("marshal helper JSON: %v", err)
	}
	fmt.Fprintln(os.Stdout, string(payload))
}

func decodeMainObject(t *testing.T, value any) map[string]any {
	t.Helper()
	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %#v", value)
	}
	return obj
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

var _ = bridge.Command{}
