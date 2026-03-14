package bridge

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SessionSource string

const (
	SessionSourceUnset     SessionSource = "unset"
	SessionSourceExplicit  SessionSource = "explicit"
	SessionSourceEnv       SessionSource = "env"
	SessionSourcePersisted SessionSource = "persisted"
	SessionSourceGenerated SessionSource = "generated"
	SessionSourceMigrated  SessionSource = "migrated"
)

type ResolvedOptions struct {
	EnvOptions
	SessionSource SessionSource
	SessionPath   string
}

func DefaultSessionFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for session storage: %w", err)
	}
	return resolveSessionFilePath(homeDir, "xcodecli"), nil
}

func DefaultLegacySessionFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for legacy session storage: %w", err)
	}
	return resolveSessionFilePath(homeDir, "xcodemcp"), nil
}

func ResolveOptions(baseEnv []string, overrides EnvOptions, sessionPath string) (ResolvedOptions, error) {
	envMap := envSliceToMap(baseEnv)
	effective := EffectiveOptions(baseEnv, overrides)
	resolved := ResolvedOptions{
		EnvOptions: effective,
	}

	switch {
	case overrides.SessionID != "":
		resolved.SessionSource = SessionSourceExplicit
	case envMap["MCP_XCODE_SESSION_ID"] != "":
		resolved.SessionSource = SessionSourceEnv
	default:
		if sessionPath == "" {
			return ResolvedOptions{}, errors.New("missing persistent session path")
		}
		sessionID, source, err := loadOrCreateSessionID(sessionPath)
		if err != nil {
			return ResolvedOptions{}, err
		}
		resolved.SessionID = sessionID
		resolved.SessionSource = source
		resolved.SessionPath = sessionPath
	}

	if resolved.SessionSource == "" {
		resolved.SessionSource = SessionSourceUnset
	}

	return resolved, nil
}

func loadOrCreateSessionID(path string) (string, SessionSource, error) {
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		sessionID := strings.TrimSpace(string(data))
		if IsValidUUID(sessionID) {
			return sessionID, SessionSourcePersisted, nil
		}
	case errors.Is(err, os.ErrNotExist):
		defaultPath, defaultErr := DefaultSessionFilePath()
		if defaultErr == nil && path == defaultPath {
			legacyPath, legacyErr := DefaultLegacySessionFilePath()
			if legacyErr == nil && legacyPath != "" && legacyPath != path {
				if legacySessionID, migrated, migrateErr := migrateLegacySessionID(legacyPath, path); migrateErr != nil {
					return "", SessionSourceUnset, migrateErr
				} else if migrated {
					return legacySessionID, SessionSourceMigrated, nil
				}
			}
		}
	default:
		return "", SessionSourceUnset, fmt.Errorf("read persistent MCP_XCODE_SESSION_ID from %s: %w", path, err)
	}

	sessionID, err := NewUUID()
	if err != nil {
		return "", SessionSourceUnset, fmt.Errorf("generate persistent MCP_XCODE_SESSION_ID: %w", err)
	}
	if err := persistSessionID(path, sessionID); err != nil {
		return "", SessionSourceUnset, err
	}
	return sessionID, SessionSourceGenerated, nil
}

func persistSessionID(path, sessionID string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create session directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(sessionID+"\n"), 0o600); err != nil {
		return fmt.Errorf("write persistent MCP_XCODE_SESSION_ID to %s: %w", path, err)
	}
	return nil
}

func migrateLegacySessionID(legacyPath, newPath string) (string, bool, error) {
	data, err := os.ReadFile(legacyPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read legacy MCP_XCODE_SESSION_ID from %s: %w", legacyPath, err)
	}
	sessionID := strings.TrimSpace(string(data))
	if !IsValidUUID(sessionID) {
		return "", false, nil
	}
	if err := persistSessionID(newPath, sessionID); err != nil {
		return "", false, err
	}
	return sessionID, true, nil
}

func resolveSessionFilePath(homeDir, supportDirName string) string {
	return filepath.Join(homeDir, "Library", "Application Support", supportDirName, "session-id")
}

func NewUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		raw[0:4],
		raw[4:6],
		raw[6:8],
		raw[8:10],
		raw[10:16],
	), nil
}
