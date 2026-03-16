package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

)

func TestParseLatestTagSelectsNewestSemanticVersion(t *testing.T) {
	raw := strings.Join([]string{
		"abc refs/tags/not-a-version",
		"def refs/tags/v0.5.2",
		"ghi refs/tags/v0.5.10",
		"jkl refs/tags/v0.6.0^{}",
		"mno refs/tags/v0.6.0",
	}, "\n")

	got, err := parseLatestTag(raw)
	if err != nil {
		t.Fatalf("parseLatestTag returned error: %v", err)
	}
	if got != "v0.6.0" {
		t.Fatalf("parseLatestTag() = %q, want %q", got, "v0.6.0")
	}
}

func TestRunRejectsTemporaryGoBuildExecutable(t *testing.T) {
	withStubs(t, func() {
		defaultTempDirFunc = func() string { return "/tmp" }
		_, err := Run(context.Background(), Config{
			CurrentVersion: "v0.5.2",
			ExecutablePath: "/tmp/go-build123/b001/exe/xcodecli",
		})
		if err == nil || !strings.Contains(err.Error(), "temporary Go build output") {
			t.Fatalf("expected temporary executable error, got %v", err)
		}
	})
}

func TestRunHomebrewUpdateReportsAlreadyUpToDate(t *testing.T) {
	withStubs(t, func() {
		prefix := "/opt/homebrew/Cellar/xcodecli/0.5.2"
		binaryPath := filepath.Join(prefix, "bin", "xcodecli")
		defaultCommandRunner = func(ctx context.Context, command Command) (CommandResult, error) {
			switch {
			case command.Name == "brew" && reflect.DeepEqual(command.Args, []string{"--prefix", homebrewFormula}):
				return CommandResult{Stdout: prefix}, nil
			case command.Name == "brew" && reflect.DeepEqual(command.Args, []string{"upgrade", homebrewFormula}):
				return CommandResult{Stdout: "Warning: xcodecli already installed"}, nil
			case command.Name == binaryPath && reflect.DeepEqual(command.Args, []string{"version"}):
				return CommandResult{Stdout: "xcodecli v0.5.2"}, nil
			default:
				return CommandResult{}, fmt.Errorf("unexpected command: %+v", command)
			}
		}

		result, err := Run(context.Background(), Config{CurrentVersion: "v0.5.2", ExecutablePath: binaryPath})
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		if result.Mode != "homebrew" {
			t.Fatalf("Mode = %q, want homebrew", result.Mode)
		}
		if !result.AlreadyUpToDate {
			t.Fatalf("AlreadyUpToDate = false, want true")
		}
		if result.TargetVersion != "v0.5.2" {
			t.Fatalf("TargetVersion = %q, want v0.5.2", result.TargetVersion)
		}
	})
}

func TestRunDirectUpdateBuildsAndReplacesCurrentExecutable(t *testing.T) {
	withStubs(t, func() {
		execDir := t.TempDir()
		execPath := filepath.Join(execDir, "xcodecli")
		if err := os.WriteFile(execPath, []byte("old-binary"), 0o755); err != nil {
			t.Fatalf("write executable: %v", err)
		}

		var replacementPath string
		defaultCommandRunner = func(ctx context.Context, command Command) (CommandResult, error) {
			switch {
			case command.Name == "brew" && reflect.DeepEqual(command.Args, []string{"--prefix", homebrewFormula}):
				return CommandResult{ExitCode: 1, Stderr: "formula not installed"}, nil
			case command.Name == "git" && reflect.DeepEqual(command.Args, []string{"ls-remote", "--refs", "--tags", sourceRepoURL}):
				return CommandResult{Stdout: strings.Join([]string{
					"abc refs/tags/v0.5.2",
					"def refs/tags/v0.5.3",
				}, "\n")}, nil
			case command.Name == "curl":
				if len(command.Args) != 4 || command.Args[0] != "-fsSL" || command.Args[2] != "-o" {
					return CommandResult{}, fmt.Errorf("unexpected curl args: %v", command.Args)
				}
				if err := os.WriteFile(command.Args[3], []byte("tarball"), 0o644); err != nil {
					return CommandResult{}, err
				}
				return CommandResult{}, nil
			case command.Name == "tar":
				if len(command.Args) != 4 || command.Args[0] != "-xzf" || command.Args[2] != "-C" {
					return CommandResult{}, fmt.Errorf("unexpected tar args: %v", command.Args)
				}
				buildRoot := filepath.Join(command.Args[3], "xcodecli-v0.5.3")
				if err := os.MkdirAll(filepath.Join(buildRoot, "scripts"), 0o755); err != nil {
					return CommandResult{}, err
				}
				return CommandResult{}, nil
			case strings.HasSuffix(command.Name, "/scripts/build.sh"):
				replacementPath = command.Args[0]
				if !containsEnv(command.Env, "VERSION=v0.5.3") || !containsEnv(command.Env, "BUILD_CHANNEL=release") {
					return CommandResult{}, fmt.Errorf("unexpected build env: %v", command.Env)
				}
				if err := os.WriteFile(command.Args[0], []byte("new-binary"), 0o755); err != nil {
					return CommandResult{}, err
				}
				return CommandResult{Stdout: "built"}, nil
			case command.Name == replacementPath && reflect.DeepEqual(command.Args, []string{"version"}):
				return CommandResult{Stdout: "xcodecli v0.5.3"}, nil
			default:
				return CommandResult{}, fmt.Errorf("unexpected command: %+v", command)
			}
		}

		result, err := Run(context.Background(), Config{CurrentVersion: "v0.5.2", ExecutablePath: execPath})
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		if result.Mode != "direct" {
			t.Fatalf("Mode = %q, want direct", result.Mode)
		}
		if result.AlreadyUpToDate {
			t.Fatalf("AlreadyUpToDate = true, want false")
		}
		if result.TargetVersion != "v0.5.3" {
			t.Fatalf("TargetVersion = %q, want v0.5.3", result.TargetVersion)
		}
		contents, err := os.ReadFile(execPath)
		if err != nil {
			t.Fatalf("read replaced executable: %v", err)
		}
		if string(contents) != "new-binary" {
			t.Fatalf("executable contents = %q, want replacement binary", string(contents))
		}
	})
}

func TestRunDirectUpdateReturnsAlreadyUpToDate(t *testing.T) {
	withStubs(t, func() {
		execPath := filepath.Join(t.TempDir(), "xcodecli")
		defaultCommandRunner = func(ctx context.Context, command Command) (CommandResult, error) {
			switch {
			case command.Name == "brew" && reflect.DeepEqual(command.Args, []string{"--prefix", homebrewFormula}):
				return CommandResult{ExitCode: 1, Stderr: "formula not installed"}, nil
			case command.Name == "git" && reflect.DeepEqual(command.Args, []string{"ls-remote", "--refs", "--tags", sourceRepoURL}):
				return CommandResult{Stdout: "abc refs/tags/v0.5.2"}, nil
			default:
				return CommandResult{}, fmt.Errorf("unexpected command: %+v", command)
			}
		}

		result, err := Run(context.Background(), Config{CurrentVersion: "v0.5.2", ExecutablePath: execPath})
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		if !result.AlreadyUpToDate {
			t.Fatalf("AlreadyUpToDate = false, want true")
		}
		if result.TargetVersion != "v0.5.2" {
			t.Fatalf("TargetVersion = %q, want v0.5.2", result.TargetVersion)
		}
	})
}

func TestRunHomebrewPathWithoutBrewReturnsHelpfulError(t *testing.T) {
	withStubs(t, func() {
		defaultCommandRunner = func(ctx context.Context, command Command) (CommandResult, error) {
			return CommandResult{}, fmt.Errorf("%w: brew", errCommandNotFound)
		}
		_, err := Run(context.Background(), Config{
			CurrentVersion: "v0.5.2",
			ExecutablePath: "/opt/homebrew/Cellar/xcodecli/0.5.2/bin/xcodecli",
		})
		if err == nil || !strings.Contains(err.Error(), "Homebrew-managed") {
			t.Fatalf("expected Homebrew-managed error, got %v", err)
		}
	})
}

func withStubs(t *testing.T, fn func()) {
	t.Helper()
	oldRunner := defaultCommandRunner
	oldOSExecutable := defaultOSExecutableFunc
	oldEvalSymlinks := defaultEvalSymlinksFunc
	oldTempDir := defaultTempDirFunc
	oldMkdirTemp := defaultMkdirTempFunc
	oldCreateTemp := defaultCreateTempFunc
	oldReadDir := defaultReadDirFunc
	oldRemoveAll := defaultRemoveAllFunc
	oldRename := defaultRenameFunc
	oldStat := defaultStatFunc
	oldEnviron := defaultEnvironFunc
	defaultCommandRunner = func(ctx context.Context, command Command) (CommandResult, error) {
		return CommandResult{}, fmt.Errorf("unexpected command: %+v", command)
	}
	defaultOSExecutableFunc = func() (string, error) { return "/tmp/xcodecli", nil }
	defaultEvalSymlinksFunc = func(path string) (string, error) { return path, nil }
	defaultTempDirFunc = func() string { return os.TempDir() }
	defaultMkdirTempFunc = os.MkdirTemp
	defaultCreateTempFunc = os.CreateTemp
	defaultReadDirFunc = os.ReadDir
	defaultRemoveAllFunc = os.RemoveAll
	defaultRenameFunc = os.Rename
	defaultStatFunc = os.Stat
	defaultEnvironFunc = func() []string { return nil }
	defer func() {
		defaultCommandRunner = oldRunner
		defaultOSExecutableFunc = oldOSExecutable
		defaultEvalSymlinksFunc = oldEvalSymlinks
		defaultTempDirFunc = oldTempDir
		defaultMkdirTempFunc = oldMkdirTemp
		defaultCreateTempFunc = oldCreateTemp
		defaultReadDirFunc = oldReadDir
		defaultRemoveAllFunc = oldRemoveAll
		defaultRenameFunc = oldRename
		defaultStatFunc = oldStat
		defaultEnvironFunc = oldEnviron
	}()
	fn()
}

func containsEnv(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestRunCommandUsesProvidedEnvironment(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "print-env.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$FOO\"\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	result, err := runCommand(context.Background(), Command{Name: script, Env: []string{"FOO=bar"}})
	if err != nil {
		t.Fatalf("runCommand returned error: %v", err)
	}
	if result.Stdout != "bar" {
		t.Fatalf("Stdout = %q, want bar", result.Stdout)
	}
}

func TestPrepareReplacementPathCreatesSiblingFile(t *testing.T) {
	withStubs(t, func() {
		dir := t.TempDir()
		executablePath := filepath.Join(dir, "xcodecli")
		path, cleanup, err := prepareReplacementPath(executablePath)
		if err != nil {
			t.Fatalf("prepareReplacementPath returned error: %v", err)
		}
		defer cleanup()
		if filepath.Dir(path) != dir {
			t.Fatalf("replacement dir = %q, want %q", filepath.Dir(path), dir)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("replacement path should be reserved but absent, stat err = %v", err)
		}
	})
}

func TestCommandFailureUsesStdoutWhenStderrEmpty(t *testing.T) {
	err := commandFailure("download", CommandResult{ExitCode: 2, Stdout: "boom"})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected stdout in error, got %v", err)
	}
}

func TestRunCommandPropagatesExitCode(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho nope >&2\nexit 7\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	result, err := runCommand(context.Background(), Command{Name: script})
	if err != nil {
		t.Fatalf("runCommand returned error: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", result.ExitCode)
	}
	if result.Stderr != "nope" {
		t.Fatalf("Stderr = %q, want nope", result.Stderr)
	}
}

func TestRunCommandReportsMissingCLI(t *testing.T) {
	_, err := runCommand(context.Background(), Command{Name: "definitely-not-a-real-command"})
	if err == nil || !strings.Contains(err.Error(), "command not found on PATH") {
		t.Fatalf("expected missing CLI error, got %v", err)
	}
}

func TestInspectBinaryVersionRejectsUnexpectedOutput(t *testing.T) {
	withStubs(t, func() {
		defaultCommandRunner = func(ctx context.Context, command Command) (CommandResult, error) {
			return CommandResult{Stdout: "weird-output"}, nil
		}
		_, err := inspectBinaryVersion(context.Background(), "/tmp/xcodecli")
		if err == nil || !strings.Contains(err.Error(), "unexpected version output") {
			t.Fatalf("expected unexpected version output error, got %v", err)
		}
	})
}

func TestRunDirectUpdateBuildFailureUsesCapturedOutput(t *testing.T) {
	withStubs(t, func() {
		execPath := filepath.Join(t.TempDir(), "xcodecli")
		defaultCommandRunner = func(ctx context.Context, command Command) (CommandResult, error) {
			switch {
			case command.Name == "brew" && reflect.DeepEqual(command.Args, []string{"--prefix", homebrewFormula}):
				return CommandResult{ExitCode: 1, Stderr: "formula not installed"}, nil
			case command.Name == "git" && reflect.DeepEqual(command.Args, []string{"ls-remote", "--refs", "--tags", sourceRepoURL}):
				return CommandResult{Stdout: "abc refs/tags/v0.5.3"}, nil
			case command.Name == "curl":
				if err := os.WriteFile(command.Args[3], []byte("tarball"), 0o644); err != nil {
					return CommandResult{}, err
				}
				return CommandResult{}, nil
			case command.Name == "tar":
				buildRoot := filepath.Join(command.Args[3], "xcodecli-v0.5.3")
				if err := os.MkdirAll(filepath.Join(buildRoot, "scripts"), 0o755); err != nil {
					return CommandResult{}, err
				}
				return CommandResult{}, nil
			case strings.HasSuffix(command.Name, "/scripts/build.sh"):
				return CommandResult{ExitCode: 1, Stderr: "build exploded"}, nil
			default:
				return CommandResult{}, fmt.Errorf("unexpected command: %+v", command)
			}
		}
		_, err := Run(context.Background(), Config{CurrentVersion: "v0.5.2", ExecutablePath: execPath})
		if err == nil || !strings.Contains(err.Error(), "build exploded") {
			t.Fatalf("expected build failure with stderr, got %v", err)
		}
	})
}

func TestRunCommandDirectPathDoesNotRequireLookup(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hello.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	result, err := runCommand(context.Background(), Command{Name: script})
	if err != nil {
		t.Fatalf("runCommand returned error: %v", err)
	}
	if result.Stdout != "ok" {
		t.Fatalf("Stdout = %q, want ok", result.Stdout)
	}
}

func TestFindExtractedSourceDirReturnsFirstDirectory(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "archive.tar.gz"), []byte("noop"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(workDir, "source"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got, err := findExtractedSourceDir(workDir)
	if err != nil {
		t.Fatalf("findExtractedSourceDir returned error: %v", err)
	}
	if got != filepath.Join(workDir, "source") {
		t.Fatalf("findExtractedSourceDir() = %q, want %q", got, filepath.Join(workDir, "source"))
	}
}

func TestParseLatestTagFailsWhenMissingVersions(t *testing.T) {
	_, err := parseLatestTag("abc refs/tags/not-semver")
	if err == nil || !strings.Contains(err.Error(), "no semantic version tags") {
		t.Fatalf("expected missing semantic version error, got %v", err)
	}
}

func TestRunCommandUsesWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "pwd.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\npwd\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	workDir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(workDir, 0o755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}
	result, err := runCommand(context.Background(), Command{Name: script, Dir: workDir})
	if err != nil {
		t.Fatalf("runCommand returned error: %v", err)
	}
	if strings.TrimSpace(result.Stdout) != workDir {
		t.Fatalf("Stdout = %q, want %q", result.Stdout, workDir)
	}
}

func TestRunCommandTrimmedOutput(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "trim.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf ' hi \\n'\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	result, err := runCommand(context.Background(), Command{Name: script})
	if err != nil {
		t.Fatalf("runCommand returned error: %v", err)
	}
	if result.Stdout != "hi" {
		t.Fatalf("Stdout = %q, want hi", result.Stdout)
	}
}

func TestRunCommandExitErrorKeepsStdout(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "partial.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho partial\nexit 9\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	result, err := runCommand(context.Background(), Command{Name: script})
	if err != nil {
		t.Fatalf("runCommand returned error: %v", err)
	}
	if result.ExitCode != 9 {
		t.Fatalf("ExitCode = %d, want 9", result.ExitCode)
	}
	if result.Stdout != "partial" {
		t.Fatalf("Stdout = %q, want partial", result.Stdout)
	}
}

func TestWithStubsDoesNotLeakGlobalState(t *testing.T) {
	before := defaultCommandRunner
	withStubs(t, func() {
		if before == nil {
			t.Fatal("expected defaultCommandRunner to be set")
		}
	})
	if reflect.ValueOf(before).Pointer() != reflect.ValueOf(defaultCommandRunner).Pointer() {
		t.Fatal("defaultCommandRunner was not restored")
	}
}

func TestRunCommandCapturesStderrOnlyScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "stderr.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho nope >&2\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	result, err := runCommand(context.Background(), Command{Name: script})
	if err != nil {
		t.Fatalf("runCommand returned error: %v", err)
	}
	if result.Stderr != "nope" {
		t.Fatalf("Stderr = %q, want nope", result.Stderr)
	}
}

func TestPrepareReplacementPathCleanupRemovesFile(t *testing.T) {
	withStubs(t, func() {
		dir := t.TempDir()
		path, cleanup, err := prepareReplacementPath(filepath.Join(dir, "xcodecli"))
		if err != nil {
			t.Fatalf("prepareReplacementPath returned error: %v", err)
		}
		if err := os.WriteFile(path, []byte("temp"), 0o644); err != nil {
			t.Fatalf("write temp replacement: %v", err)
		}
		cleanup()
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cleanup should remove replacement file, stat err = %v", err)
		}
	})
}

func BenchmarkParseLatestTag(b *testing.B) {
	raw := strings.Repeat("abc refs/tags/v0.5.2\n", 50)
	for i := 0; i < b.N; i++ {
		_, _ = parseLatestTag(raw)
	}
}

var _ io.Reader
