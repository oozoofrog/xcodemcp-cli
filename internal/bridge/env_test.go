package bridge

import "testing"

func TestEffectiveOptionsPrefersOverrides(t *testing.T) {
	env := []string{
		"MCP_XCODE_PID=111",
		"MCP_XCODE_SESSION_ID=aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
	}
	effective := EffectiveOptions(env, EnvOptions{
		XcodePID:  "222",
		SessionID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
	})
	if effective.XcodePID != "222" || effective.SessionID != "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" {
		t.Fatalf("unexpected effective options: %+v", effective)
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	env := ApplyEnvOverrides([]string{"FOO=bar", "MCP_XCODE_PID=111"}, EnvOptions{XcodePID: "333", SessionID: "cccccccc-cccc-cccc-cccc-cccccccccccc"})
	m := envSliceToMap(env)
	if m["FOO"] != "bar" || m["MCP_XCODE_PID"] != "333" || m["MCP_XCODE_SESSION_ID"] != "cccccccc-cccc-cccc-cccc-cccccccccccc" {
		t.Fatalf("unexpected env map: %+v", m)
	}
}

func TestValidateEnvOptions(t *testing.T) {
	if err := ValidateEnvOptions(EnvOptions{XcodePID: "12", SessionID: "11111111-1111-1111-1111-111111111111"}); err != nil {
		t.Fatalf("expected valid options, got %v", err)
	}
	if err := ValidateEnvOptions(EnvOptions{XcodePID: "0"}); err == nil {
		t.Fatal("expected invalid PID error")
	}
	if err := ValidateEnvOptions(EnvOptions{SessionID: "invalid"}); err == nil {
		t.Fatal("expected invalid UUID error")
	}
}
