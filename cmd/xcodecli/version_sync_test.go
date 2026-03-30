package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestGoAndSwiftSourceVersionsStayInSync(t *testing.T) {
	versionFile := filepath.Join("..", "..", "Sources", "XcodeCLICore", "Shared", "Version.swift")
	data, err := os.ReadFile(versionFile)
	if err != nil {
		t.Fatalf("read %s: %v", versionFile, err)
	}

	re := regexp.MustCompile(`public static let source = "([^"]+)"`)
	match := re.FindSubmatch(data)
	if len(match) != 2 {
		t.Fatalf("could not extract Swift source version from %s", versionFile)
	}

	swiftSourceVersion := string(match[1])
	if swiftSourceVersion != sourceVersion {
		t.Fatalf("Go sourceVersion = %q, Swift source version = %q", sourceVersion, swiftSourceVersion)
	}
}
