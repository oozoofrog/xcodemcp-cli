package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const binaryIdentityFileName = "binary-id"

func binaryIdentityPath(paths Paths) string {
	if strings.TrimSpace(paths.SupportDir) == "" {
		return ""
	}
	return filepath.Join(paths.SupportDir, binaryIdentityFileName)
}

func binaryIdentityForExecutable(path string) (string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" {
		return "", fmt.Errorf("missing executable path")
	}

	file, err := os.Open(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			return "path:" + cleaned, nil
		}
		return "", fmt.Errorf("open executable %s: %w", cleaned, err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash executable %s: %w", cleaned, err)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func readBinaryIdentity(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func writeBinaryIdentity(path, identity string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("missing binary identity path")
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(identity)+"\n"), 0o600); err != nil {
		return fmt.Errorf("write binary identity %s: %w", path, err)
	}
	return nil
}
