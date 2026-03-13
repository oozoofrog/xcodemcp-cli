package bridge

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type EnvOptions struct {
	XcodePID  string
	SessionID string
}

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func EffectiveOptions(baseEnv []string, overrides EnvOptions) EnvOptions {
	envMap := envSliceToMap(baseEnv)
	effective := EnvOptions{
		XcodePID:  envMap["MCP_XCODE_PID"],
		SessionID: envMap["MCP_XCODE_SESSION_ID"],
	}
	if overrides.XcodePID != "" {
		effective.XcodePID = overrides.XcodePID
	}
	if overrides.SessionID != "" {
		effective.SessionID = overrides.SessionID
	}
	return effective
}

func ApplyEnvOverrides(baseEnv []string, opts EnvOptions) []string {
	envMap := envSliceToMap(baseEnv)
	if opts.XcodePID != "" {
		envMap["MCP_XCODE_PID"] = opts.XcodePID
	}
	if opts.SessionID != "" {
		envMap["MCP_XCODE_SESSION_ID"] = opts.SessionID
	}
	return envMapToSlice(envMap)
}

func ValidateEnvOptions(opts EnvOptions) error {
	if opts.XcodePID != "" {
		if _, err := ParsePID(opts.XcodePID); err != nil {
			return err
		}
	}
	if opts.SessionID != "" && !IsValidUUID(opts.SessionID) {
		return fmt.Errorf("MCP_XCODE_SESSION_ID must be a UUID")
	}
	return nil
}

func ParsePID(raw string) (int, error) {
	pid, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("MCP_XCODE_PID must be a positive integer")
	}
	return pid, nil
}

func IsValidUUID(raw string) bool {
	return uuidPattern.MatchString(strings.TrimSpace(raw))
}

func envSliceToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		m[key] = value
	}
	return m
}

func envMapToSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	return out
}
