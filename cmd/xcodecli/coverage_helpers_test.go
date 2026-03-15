package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oozoofrog/xcodecli/internal/mcp"
)

func TestCLIUsageAndFlagHelpers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage string
		want  []string
	}{
		{"mcp", mcpUsage(), []string{"xcodecli mcp config", "--mode <agent|bridge>", "xcodecli mcp codex"}},
		{"tools", toolsUsage(), []string{"tools list", "List MCP tools exposed"}},
		{"tool", toolUsage(), []string{"tool inspect", "tool call"}},
	} {
		for _, want := range tc.want {
			if !strings.Contains(tc.usage, want) {
				t.Fatalf("%s usage missing %q: %s", tc.name, want, tc.usage)
			}
		}
	}

	if !agentGuideFlagTakesValue("--timeout") || agentGuideFlagTakesValue("--json") {
		t.Fatal("agentGuideFlagTakesValue returned unexpected result")
	}
	if !toolCallFlagTakesValue("--json") || !toolCallFlagTakesValue("--timeout") || toolCallFlagTakesValue("--debug") {
		t.Fatal("toolCallFlagTakesValue returned unexpected result")
	}
}

func TestParseHelpAdditionalTopics(t *testing.T) {
	for _, args := range [][]string{
		{"help", "mcp"},
		{"help", "tools"},
		{"help", "tool"},
		{"help", "serve"},
	} {
		if _, _, err := parseCLI(args); err != errUsageRequested {
			t.Fatalf("parseCLI(%v) err = %v, want errUsageRequested", args, err)
		}
	}
}

func TestRunServeMCPDelegatesToMCPServer(t *testing.T) {
	err := runServeMCP(context.Background(), mcp.ServerConfig{}, mcp.ServerHandler{})
	if err == nil || !strings.Contains(err.Error(), "missing server stdin") {
		t.Fatalf("expected missing stdin error, got %v", err)
	}
}

func TestFormatTimeoutDurationHelper(t *testing.T) {
	cases := []struct {
		value time.Duration
		want  string
	}{
		{0, "0s"},
		{2 * time.Hour, "2h"},
		{10 * time.Minute, "10m"},
		{45 * time.Second, "45s"},
		{1500 * time.Millisecond, "1.5s"},
	}
	for _, tc := range cases {
		if got := formatTimeoutDuration(tc.value); got != tc.want {
			t.Fatalf("formatTimeoutDuration(%s) = %q, want %q", tc.value, got, tc.want)
		}
	}
}
