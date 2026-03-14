package bridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveOptionsUsesExplicitAndEnvBeforePersistence(t *testing.T) {
	tempDir := t.TempDir()
	sessionPath := filepath.Join(tempDir, "session-id")

	resolved, err := ResolveOptions([]string{"MCP_XCODE_SESSION_ID=11111111-1111-1111-1111-111111111111"}, EnvOptions{
		SessionID: "22222222-2222-2222-2222-222222222222",
	}, sessionPath)
	if err != nil {
		t.Fatalf("ResolveOptions returned error: %v", err)
	}
	if resolved.SessionID != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("SessionID = %q, want explicit override", resolved.SessionID)
	}
	if resolved.SessionSource != SessionSourceExplicit {
		t.Fatalf("SessionSource = %q, want %q", resolved.SessionSource, SessionSourceExplicit)
	}
	if _, err := os.Stat(sessionPath); !os.IsNotExist(err) {
		t.Fatalf("session file should not have been created, stat err=%v", err)
	}

	resolved, err = ResolveOptions([]string{"MCP_XCODE_SESSION_ID=33333333-3333-3333-3333-333333333333"}, EnvOptions{}, sessionPath)
	if err != nil {
		t.Fatalf("ResolveOptions returned error: %v", err)
	}
	if resolved.SessionID != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("SessionID = %q, want env value", resolved.SessionID)
	}
	if resolved.SessionSource != SessionSourceEnv {
		t.Fatalf("SessionSource = %q, want %q", resolved.SessionSource, SessionSourceEnv)
	}
}

func TestResolveOptionsCreatesAndPersistsSessionID(t *testing.T) {
	tempDir := t.TempDir()
	sessionPath := filepath.Join(tempDir, "nested", "session-id")

	resolved, err := ResolveOptions(nil, EnvOptions{}, sessionPath)
	if err != nil {
		t.Fatalf("ResolveOptions returned error: %v", err)
	}
	if resolved.SessionSource != SessionSourceGenerated {
		t.Fatalf("SessionSource = %q, want %q", resolved.SessionSource, SessionSourceGenerated)
	}
	if !IsValidUUID(resolved.SessionID) {
		t.Fatalf("generated session ID is not a UUID: %q", resolved.SessionID)
	}

	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) failed: %v", sessionPath, err)
	}
	if got := string(data); got != resolved.SessionID+"\n" {
		t.Fatalf("persisted session content = %q, want %q", got, resolved.SessionID+"\n")
	}
}

func TestResolveOptionsReusesPersistedSessionID(t *testing.T) {
	tempDir := t.TempDir()
	sessionPath := filepath.Join(tempDir, "session-id")
	want := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	if err := os.WriteFile(sessionPath, []byte(want+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	resolved, err := ResolveOptions(nil, EnvOptions{}, sessionPath)
	if err != nil {
		t.Fatalf("ResolveOptions returned error: %v", err)
	}
	if resolved.SessionID != want {
		t.Fatalf("SessionID = %q, want %q", resolved.SessionID, want)
	}
	if resolved.SessionSource != SessionSourcePersisted {
		t.Fatalf("SessionSource = %q, want %q", resolved.SessionSource, SessionSourcePersisted)
	}
}

func TestResolveOptionsRepairsInvalidPersistedSessionID(t *testing.T) {
	tempDir := t.TempDir()
	sessionPath := filepath.Join(tempDir, "session-id")
	if err := os.WriteFile(sessionPath, []byte("not-a-uuid\n"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	resolved, err := ResolveOptions(nil, EnvOptions{}, sessionPath)
	if err != nil {
		t.Fatalf("ResolveOptions returned error: %v", err)
	}
	if resolved.SessionSource != SessionSourceGenerated {
		t.Fatalf("SessionSource = %q, want %q", resolved.SessionSource, SessionSourceGenerated)
	}
	if !IsValidUUID(resolved.SessionID) {
		t.Fatalf("repaired session ID is not a UUID: %q", resolved.SessionID)
	}
	if resolved.SessionID == "not-a-uuid" {
		t.Fatal("expected invalid persisted session ID to be replaced")
	}
}

func TestNewUUIDReturnsValidUUID(t *testing.T) {
	value, err := NewUUID()
	if err != nil {
		t.Fatalf("NewUUID returned error: %v", err)
	}
	if !IsValidUUID(value) {
		t.Fatalf("NewUUID returned invalid UUID: %q", value)
	}
}
