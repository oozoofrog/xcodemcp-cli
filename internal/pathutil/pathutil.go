package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

// IsTemporaryGoBuildExecutable reports whether path looks like a temporary
// executable produced by "go run" or "go test" under the system temp directory.
// tempDirFunc should return the system's temporary directory (typically os.TempDir).
func IsTemporaryGoBuildExecutable(path string, tempDirFunc func() string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	normalizedPath := NormalizePrivatePrefix(filepath.Clean(path))
	normalizedTempDir := NormalizePrivatePrefix(filepath.Clean(tempDirFunc()))
	if !PathWithinBase(normalizedPath, normalizedTempDir) {
		return false
	}
	if filepath.Base(normalizedPath) != "xcodecli" {
		return false
	}
	if filepath.Base(filepath.Dir(normalizedPath)) != "exe" {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(normalizedPath), "/") {
		if strings.HasPrefix(part, "go-build") {
			return true
		}
	}
	return false
}

// NormalizePrivatePrefix strips the /private prefix that macOS adds to
// some temporary directory paths (e.g. /private/tmp → /tmp).
func NormalizePrivatePrefix(path string) string {
	const privatePrefix = "/private"
	if strings.HasPrefix(path, privatePrefix+"/") {
		return strings.TrimPrefix(path, privatePrefix)
	}
	return path
}

// PathWithinBase reports whether path is equal to or a descendant of base.
func PathWithinBase(path, base string) bool {
	if path == base {
		return true
	}
	if base == "" {
		return false
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
