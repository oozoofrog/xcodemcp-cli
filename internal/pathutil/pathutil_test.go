package pathutil

import (
	"testing"
)

func TestIsTemporaryGoBuildExecutable(t *testing.T) {
	tempDir := func() string { return "/tmp" }

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"go-build path", "/tmp/go-build123/b001/exe/xcodecli", true},
		{"non-temp path", "/usr/local/bin/xcodecli", false},
		{"empty string", "", false},
		{"private prefix normalization", "/private/tmp/go-build456/b001/exe/xcodecli", true},
		{"temp but not go-build", "/tmp/other/exe/xcodecli", false},
		{"go-build but not exe dir", "/tmp/go-build123/b001/xcodecli", false},
		{"go-build but wrong binary name", "/tmp/go-build123/b001/exe/other", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsTemporaryGoBuildExecutable(tc.path, tempDir)
			if got != tc.want {
				t.Fatalf("IsTemporaryGoBuildExecutable(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestNormalizePrivatePrefix(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"strips /private prefix", "/private/tmp/x", "/tmp/x"},
		{"no-op for non-private path", "/tmp/x", "/tmp/x"},
		{"does not strip bare /private", "/private", "/private"},
		{"does not strip /privatefoo", "/privatefoo/bar", "/privatefoo/bar"},
		{"empty string", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizePrivatePrefix(tc.path)
			if got != tc.want {
				t.Fatalf("NormalizePrivatePrefix(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestPathWithinBase(t *testing.T) {
	cases := []struct {
		name string
		path string
		base string
		want bool
	}{
		{"same path", "/tmp", "/tmp", true},
		{"child path", "/tmp/foo/bar", "/tmp", true},
		{"parent path rejected", "/tmp", "/tmp/foo", false},
		{"empty base", "/tmp/foo", "", false},
		{"relative escape attempt", "/tmp/../etc/passwd", "/tmp", false},
		{"unrelated paths", "/var/log", "/tmp", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PathWithinBase(tc.path, tc.base)
			if got != tc.want {
				t.Fatalf("PathWithinBase(%q, %q) = %v, want %v", tc.path, tc.base, got, tc.want)
			}
		})
	}
}
