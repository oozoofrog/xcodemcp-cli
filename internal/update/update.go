package update

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/oozoofrog/xcodecli/internal/pathutil"
)

const (
	sourceRepoURL   = "https://github.com/oozoofrog/xcodecli.git"
	sourceTarball   = "https://github.com/oozoofrog/xcodecli/archive/refs/tags/%s.tar.gz"
	homebrewFormula = "oozoofrog/tap/xcodecli"
)

type Config struct {
	CurrentVersion string
	ExecutablePath string
}

type Result struct {
	Mode            string
	CurrentVersion  string
	TargetVersion   string
	AlreadyUpToDate bool
}

type Command struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}

type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type CommandRunner func(ctx context.Context, command Command) (CommandResult, error)

var defaultCommandRunner CommandRunner = runCommand
var defaultOSExecutableFunc = os.Executable
var defaultEvalSymlinksFunc = filepath.EvalSymlinks
var defaultTempDirFunc = os.TempDir
var defaultMkdirTempFunc = os.MkdirTemp
var defaultCreateTempFunc = os.CreateTemp
var defaultReadDirFunc = os.ReadDir
var defaultRemoveAllFunc = os.RemoveAll
var defaultRenameFunc = os.Rename
var defaultStatFunc = os.Stat
var defaultEnvironFunc = os.Environ
var errCommandNotFound = errors.New("command not found on PATH")

func Run(ctx context.Context, cfg Config) (Result, error) {
	currentVersion := strings.TrimSpace(cfg.CurrentVersion)
	if currentVersion == "" {
		return Result{}, errors.New("current version must not be empty")
	}

	executablePath := strings.TrimSpace(cfg.ExecutablePath)
	if executablePath == "" {
		resolved, err := resolveExecutablePath()
		if err != nil {
			return Result{}, err
		}
		executablePath = resolved
	}

	if pathutil.IsTemporaryGoBuildExecutable(executablePath, defaultTempDirFunc) {
		return Result{}, fmt.Errorf("current executable path appears to be a temporary Go build output (%s); rerun `xcodecli update` using an installed or directly built xcodecli binary", executablePath)
	}
	if err := validateStableExecutablePath(executablePath); err != nil {
		return Result{}, err
	}

	resolvedExecutablePath := cleanResolvedPath(executablePath)
	isHomebrew, prefix, err := detectHomebrewInstallation(ctx, resolvedExecutablePath)
	if err != nil {
		return Result{}, err
	}
	if isHomebrew {
		return runHomebrewUpdate(ctx, currentVersion, prefix)
	}
	return runDirectUpdate(ctx, currentVersion, resolvedExecutablePath)
}

func resolveExecutablePath() (string, error) {
	path, err := defaultOSExecutableFunc()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	return cleanResolvedPath(path), nil
}

func validateStableExecutablePath(path string) error {
	cleaned := cleanResolvedPath(path)
	lower := strings.ToLower(cleaned)
	if !filepath.IsAbs(cleaned) {
		return fmt.Errorf("current executable path must be absolute for `xcodecli update` (%s); rerun update from an installed stable path", cleaned)
	}
	if strings.Contains(lower, string(filepath.Separator)+".build"+string(filepath.Separator)) {
		return fmt.Errorf("current executable path looks like a Swift build output (%s); rerun `xcodecli update` from an installed stable path", cleaned)
	}
	if strings.HasPrefix(lower, "/tmp/") || strings.HasPrefix(lower, "/private/tmp/") {
		return fmt.Errorf("current executable path is in a temporary directory (%s); rerun `xcodecli update` from an installed stable path", cleaned)
	}
	if strings.HasPrefix(lower, "/volumes/") {
		return fmt.Errorf("current executable path is on an external volume (%s); rerun `xcodecli update` from an installed internal stable path", cleaned)
	}
	return nil
}

func detectHomebrewInstallation(ctx context.Context, executablePath string) (bool, string, error) {
	prefix, err := homebrewPrefix(ctx)
	if err != nil {
		if errors.Is(err, errCommandNotFound) {
			if looksLikeHomebrewPath(executablePath) {
				return false, "", fmt.Errorf("brew CLI not found on PATH but current xcodecli binary appears to be Homebrew-managed (%s): %w", executablePath, err)
			}
			return false, "", nil
		}
		if looksLikeHomebrewPath(executablePath) {
			return false, "", err
		}
		return false, "", nil
	}
	if prefix == "" {
		if looksLikeHomebrewPath(executablePath) {
			return false, "", fmt.Errorf("brew did not report a prefix for %s but current xcodecli binary appears to be Homebrew-managed (%s)", homebrewFormula, executablePath)
		}
		return false, "", nil
	}
	if pathutil.PathWithinBase(executablePath, prefix) {
		return true, prefix, nil
	}
	return false, "", nil
}

func homebrewPrefix(ctx context.Context) (string, error) {
	result, err := defaultCommandRunner(ctx, Command{Name: "brew", Args: []string{"--prefix", homebrewFormula}})
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		stderr := strings.TrimSpace(result.Stderr)
		stdout := strings.TrimSpace(result.Stdout)
		detail := stderr
		if detail == "" {
			detail = stdout
		}
		if detail == "" {
			detail = fmt.Sprintf("brew --prefix %s exited with code %d", homebrewFormula, result.ExitCode)
		}
		return "", fmt.Errorf("inspect Homebrew installation: %s", detail)
	}
	return cleanResolvedPath(strings.TrimSpace(result.Stdout)), nil
}

func runHomebrewUpdate(ctx context.Context, currentVersion, initialPrefix string) (Result, error) {
	result, err := defaultCommandRunner(ctx, Command{Name: "brew", Args: []string{"upgrade", homebrewFormula}})
	if err != nil {
		return Result{}, fmt.Errorf("run brew upgrade: %w", err)
	}
	if result.ExitCode != 0 {
		return Result{}, commandFailure("run brew upgrade", result)
	}

	prefix := initialPrefix
	if refreshed, err := homebrewPrefix(ctx); err == nil && refreshed != "" {
		prefix = refreshed
	}
	binaryPath := filepath.Join(prefix, "bin", "xcodecli")
	targetVersion, err := inspectBinaryVersion(ctx, binaryPath)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Mode:            "homebrew",
		CurrentVersion:  currentVersion,
		TargetVersion:   targetVersion,
		AlreadyUpToDate: versionsEqual(currentVersion, targetVersion),
	}, nil
}

func runDirectUpdate(ctx context.Context, currentVersion, executablePath string) (Result, error) {
	targetVersion, err := latestReleaseTag(ctx)
	if err != nil {
		return Result{}, err
	}
	if versionsEqual(currentVersion, targetVersion) {
		return Result{
			Mode:            "direct",
			CurrentVersion:  currentVersion,
			TargetVersion:   targetVersion,
			AlreadyUpToDate: true,
		}, nil
	}

	replacementPath, cleanupReplacement, err := prepareReplacementPath(executablePath)
	if err != nil {
		return Result{}, err
	}
	replacementReady := false
	defer func() {
		if !replacementReady {
			cleanupReplacement()
		}
	}()

	workDir, err := defaultMkdirTempFunc(defaultTempDirFunc(), "xcodecli-update-")
	if err != nil {
		return Result{}, fmt.Errorf("create update work directory: %w", err)
	}
	defer func() {
		_ = defaultRemoveAllFunc(workDir)
	}()

	tarballPath := filepath.Join(workDir, targetVersion+".tar.gz")
	tarballURL := fmt.Sprintf(sourceTarball, targetVersion)
	curlResult, err := defaultCommandRunner(ctx, Command{Name: "curl", Args: []string{"-fsSL", tarballURL, "-o", tarballPath}})
	if err != nil {
		return Result{}, fmt.Errorf("download release tarball: %w", err)
	}
	if curlResult.ExitCode != 0 {
		return Result{}, commandFailure("download release tarball", curlResult)
	}

	tarResult, err := defaultCommandRunner(ctx, Command{Name: "tar", Args: []string{"-xzf", tarballPath, "-C", workDir}})
	if err != nil {
		return Result{}, fmt.Errorf("extract release tarball: %w", err)
	}
	if tarResult.ExitCode != 0 {
		return Result{}, commandFailure("extract release tarball", tarResult)
	}

	buildRoot, err := findExtractedSourceDir(workDir)
	if err != nil {
		return Result{}, err
	}
	buildScript := filepath.Join(buildRoot, "scripts", "build.sh")
	buildResult, err := defaultCommandRunner(ctx, Command{
		Name: buildScript,
		Args: []string{replacementPath},
		Dir:  buildRoot,
		Env:  []string{"VERSION=" + targetVersion, "BUILD_CHANNEL=release"},
	})
	if err != nil {
		return Result{}, fmt.Errorf("build updated xcodecli binary: %w", err)
	}
	if buildResult.ExitCode != 0 {
		return Result{}, commandFailure("build updated xcodecli binary", buildResult)
	}
	if _, err := defaultStatFunc(replacementPath); err != nil {
		return Result{}, fmt.Errorf("verify built binary at %s: %w", replacementPath, err)
	}

	builtVersion, err := inspectBinaryVersion(ctx, replacementPath)
	if err != nil {
		return Result{}, err
	}
	if !versionsEqual(builtVersion, targetVersion) {
		return Result{}, fmt.Errorf("built binary version mismatch: got %s, want %s", builtVersion, targetVersion)
	}

	if err := defaultRenameFunc(replacementPath, executablePath); err != nil {
		return Result{}, fmt.Errorf("replace current executable %s: %w", executablePath, err)
	}
	replacementReady = true
	return Result{
		Mode:           "direct",
		CurrentVersion: currentVersion,
		TargetVersion:  targetVersion,
	}, nil
}

func latestReleaseTag(ctx context.Context) (string, error) {
	result, err := defaultCommandRunner(ctx, Command{Name: "git", Args: []string{"ls-remote", "--refs", "--tags", sourceRepoURL}})
	if err != nil {
		return "", fmt.Errorf("query release tags: %w", err)
	}
	if result.ExitCode != 0 {
		return "", commandFailure("query release tags", result)
	}
	tag, err := parseLatestTag(result.Stdout)
	if err != nil {
		return "", err
	}
	return tag, nil
}

func parseLatestTag(raw string) (string, error) {
	var versions []semver
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		ref := fields[len(fields)-1]
		if !strings.HasPrefix(ref, "refs/tags/") {
			continue
		}
		tag := strings.TrimPrefix(ref, "refs/tags/")
		if strings.HasSuffix(tag, "^{}") {
			continue
		}
		parsed, ok := parseSemver(tag)
		if !ok {
			continue
		}
		versions = append(versions, parsed)
	}
	if len(versions) == 0 {
		return "", errors.New("no semantic version tags were found in the release list")
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[j].Less(versions[i])
	})
	return versions[0].Raw, nil
}

func inspectBinaryVersion(ctx context.Context, binaryPath string) (string, error) {
	result, err := defaultCommandRunner(ctx, Command{Name: binaryPath, Args: []string{"version"}})
	if err != nil {
		return "", fmt.Errorf("inspect installed version at %s: %w", binaryPath, err)
	}
	if result.ExitCode != 0 {
		return "", commandFailure("inspect installed version", result)
	}
	fields := strings.Fields(strings.TrimSpace(result.Stdout))
	if len(fields) < 2 || fields[0] != "xcodecli" {
		return "", fmt.Errorf("unexpected version output from %s: %q", binaryPath, result.Stdout)
	}
	return fields[1], nil
}

func prepareReplacementPath(executablePath string) (string, func(), error) {
	tmpFile, err := defaultCreateTempFunc(filepath.Dir(executablePath), "xcodecli-update-*")
	if err != nil {
		return "", nil, fmt.Errorf("create replacement file next to %s: %w", executablePath, err)
	}
	replacementPath := tmpFile.Name()
	if err := tmpFile.Close(); err != nil {
		_ = defaultRemoveAllFunc(replacementPath)
		return "", nil, fmt.Errorf("close replacement file %s: %w", replacementPath, err)
	}
	if err := defaultRemoveAllFunc(replacementPath); err != nil {
		return "", nil, fmt.Errorf("prepare replacement file %s: %w", replacementPath, err)
	}
	cleanup := func() {
		_ = defaultRemoveAllFunc(replacementPath)
	}
	return replacementPath, cleanup, nil
}

func findExtractedSourceDir(workDir string) (string, error) {
	entries, err := defaultReadDirFunc(workDir)
	if err != nil {
		return "", fmt.Errorf("list extracted update files in %s: %w", workDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(workDir, entry.Name()), nil
		}
	}
	return "", fmt.Errorf("no extracted source directory found in %s", workDir)
}

func commandFailure(action string, result CommandResult) error {
	detail := strings.TrimSpace(result.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(result.Stdout)
	}
	if detail == "" {
		detail = fmt.Sprintf("exit code %d", result.ExitCode)
	}
	return fmt.Errorf("%s: %s", action, detail)
}

func versionsEqual(left, right string) bool {
	return strings.TrimSpace(left) == strings.TrimSpace(right)
}

func cleanResolvedPath(path string) string {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" {
		return ""
	}
	resolved, err := defaultEvalSymlinksFunc(cleaned)
	if err == nil {
		return filepath.Clean(resolved)
	}
	return cleaned
}

func looksLikeHomebrewPath(path string) bool {
	normalized := filepath.ToSlash(pathutil.NormalizePrivatePrefix(cleanResolvedPath(path)))
	return strings.Contains(normalized, "/Cellar/xcodecli/") || strings.Contains(normalized, "/cellar/xcodecli/")
}

type semver struct {
	Raw   string
	Major int
	Minor int
	Patch int
}

func parseSemver(raw string) (semver, bool) {
	if !strings.HasPrefix(raw, "v") {
		return semver{}, false
	}
	parts := strings.Split(strings.TrimPrefix(raw, "v"), ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semver{}, false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semver{}, false
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semver{}, false
	}
	return semver{Raw: raw, Major: major, Minor: minor, Patch: patch}, true
}

func (v semver) Less(other semver) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}

func runCommand(ctx context.Context, command Command) (CommandResult, error) {
	path := command.Name
	if !strings.Contains(command.Name, string(os.PathSeparator)) {
		resolved, err := exec.LookPath(command.Name)
		if err != nil {
			return CommandResult{}, fmt.Errorf("%w: %s", errCommandNotFound, command.Name)
		}
		path = resolved
	}
	cmd := exec.CommandContext(ctx, path, command.Args...)
	if command.Dir != "" {
		cmd.Dir = command.Dir
	}
	if len(command.Env) > 0 {
		cmd.Env = append(defaultEnvironFunc(), command.Env...)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := CommandResult{
		ExitCode: 0,
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, fmt.Errorf("run %s: %w", command.Name, err)
}
