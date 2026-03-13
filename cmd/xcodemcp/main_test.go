package main

import (
	"context"
	"strings"
	"testing"
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

func TestParseCLIDoctor(t *testing.T) {
	cfg, _, err := parseCLI([]string{"doctor", "--xcode-pid", "456"})
	if err != nil {
		t.Fatalf("parseCLI returned error: %v", err)
	}
	if cfg.Command != commandDoctor {
		t.Fatalf("command = %q, want %q", cfg.Command, commandDoctor)
	}
	if cfg.XcodePID != "456" {
		t.Fatalf("xcode pid = %q, want 456", cfg.XcodePID)
	}
	if cfg.Debug {
		t.Fatalf("doctor should not set debug")
	}
}

func TestParseCLIHelp(t *testing.T) {
	_, usage, err := parseCLI([]string{"help", "doctor"})
	if err != errUsageRequested {
		t.Fatalf("err = %v, want errUsageRequested", err)
	}
	if !strings.Contains(usage, "xcodemcp doctor") {
		t.Fatalf("usage missing doctor help: %q", usage)
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
