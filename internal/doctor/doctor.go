package doctor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/oozoofrog/xcodemcp-cli/internal/bridge"
)

type Status string

const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
	StatusInfo Status = "info"
)

type Check struct {
	Name   string
	Status Status
	Detail string
}

type Report struct {
	Checks []Check
}

type Options struct {
	BaseEnv   []string
	XcodePID  string
	SessionID string
}

type Process struct {
	PID     int
	Command string
}

type CommandRequest struct {
	Name  string
	Args  []string
	Env   []string
	Stdin []byte
}

type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Inspector struct {
	LookPath      func(file string) (string, error)
	RunCommand    func(ctx context.Context, req CommandRequest) (CommandResult, error)
	ListProcesses func(ctx context.Context) ([]Process, error)
}

func NewInspector() Inspector {
	return Inspector{
		LookPath:      exec.LookPath,
		RunCommand:    defaultRunCommand,
		ListProcesses: defaultListProcesses,
	}
}

func (r Report) Success() bool {
	for _, check := range r.Checks {
		if check.Status == StatusFail {
			return false
		}
	}
	return true
}

func (r Report) String() string {
	var okCount, warnCount, failCount, infoCount int
	var b strings.Builder
	b.WriteString("xcodemcp doctor\n\n")
	for _, check := range r.Checks {
		switch check.Status {
		case StatusOK:
			okCount++
		case StatusWarn:
			warnCount++
		case StatusFail:
			failCount++
		case StatusInfo:
			infoCount++
		}
		fmt.Fprintf(&b, "%s %s: %s\n", statusIcon(check.Status), check.Name, check.Detail)
	}
	fmt.Fprintf(&b, "\nSummary: %d ok, %d warn, %d fail, %d info\n", okCount, warnCount, failCount, infoCount)
	return b.String()
}

func (i Inspector) Run(ctx context.Context, opts Options) Report {
	checks := make([]Check, 0, 7)

	xcrunPath, err := i.LookPath("xcrun")
	xcrunAvailable := err == nil
	if err != nil {
		checks = append(checks, Check{Name: "xcrun lookup", Status: StatusFail, Detail: err.Error()})
	} else {
		checks = append(checks, Check{Name: "xcrun lookup", Status: StatusOK, Detail: xcrunPath})
	}

	if xcrunAvailable {
		result, runErr := i.RunCommand(ctx, CommandRequest{Name: xcrunPath, Args: []string{"mcpbridge", "--help"}, Env: opts.BaseEnv})
		if runErr != nil {
			checks = append(checks, Check{Name: "xcrun mcpbridge --help", Status: StatusFail, Detail: formatCommandFailure(result, runErr)})
		} else {
			checks = append(checks, Check{Name: "xcrun mcpbridge --help", Status: StatusOK, Detail: fmt.Sprintf("exit 0 (%d bytes stdout)", len(result.Stdout))})
		}
	} else {
		checks = append(checks, Check{Name: "xcrun mcpbridge --help", Status: StatusInfo, Detail: "skipped because xcrun is unavailable"})
	}

	xcodeSelectResult, xcodeSelectErr := i.RunCommand(ctx, CommandRequest{Name: "xcode-select", Args: []string{"-p"}, Env: opts.BaseEnv})
	if xcodeSelectErr != nil {
		checks = append(checks, Check{Name: "xcode-select -p", Status: StatusFail, Detail: formatCommandFailure(xcodeSelectResult, xcodeSelectErr)})
	} else {
		checks = append(checks, Check{Name: "xcode-select -p", Status: StatusOK, Detail: strings.TrimSpace(xcodeSelectResult.Stdout)})
	}

	processes, procErr := i.ListProcesses(ctx)
	var xcodeCandidates []Process
	if procErr != nil {
		checks = append(checks, Check{Name: "running Xcode processes", Status: StatusFail, Detail: procErr.Error()})
	} else {
		xcodeCandidates = filterXcodeCandidates(processes)
		if len(xcodeCandidates) == 0 {
			checks = append(checks, Check{Name: "running Xcode processes", Status: StatusWarn, Detail: "no Xcode.app process detected"})
		} else {
			checks = append(checks, Check{Name: "running Xcode processes", Status: StatusOK, Detail: summarizeProcesses(xcodeCandidates)})
		}
	}

	pidValid := true
	if opts.XcodePID == "" {
		checks = append(checks, Check{Name: "effective MCP_XCODE_PID", Status: StatusInfo, Detail: "not set"})
	} else {
		pid, parseErr := bridge.ParsePID(opts.XcodePID)
		if parseErr != nil {
			pidValid = false
			checks = append(checks, Check{Name: "effective MCP_XCODE_PID", Status: StatusFail, Detail: parseErr.Error()})
		} else if procErr != nil {
			pidValid = false
			checks = append(checks, Check{Name: "effective MCP_XCODE_PID", Status: StatusFail, Detail: "cannot validate PID because process listing failed"})
		} else if proc, ok := findProcess(processes, pid); !ok {
			pidValid = false
			checks = append(checks, Check{Name: "effective MCP_XCODE_PID", Status: StatusFail, Detail: fmt.Sprintf("PID %d was not found", pid)})
		} else if !looksLikeXcodeProcess(proc) {
			pidValid = false
			checks = append(checks, Check{Name: "effective MCP_XCODE_PID", Status: StatusFail, Detail: fmt.Sprintf("PID %d does not look like an Xcode.app process (%s)", pid, proc.Command)})
		} else {
			checks = append(checks, Check{Name: "effective MCP_XCODE_PID", Status: StatusOK, Detail: fmt.Sprintf("PID %d -> %s", pid, proc.Command)})
		}
	}

	sessionValid := true
	if opts.SessionID == "" {
		checks = append(checks, Check{Name: "effective MCP_XCODE_SESSION_ID", Status: StatusInfo, Detail: "not set"})
	} else if !bridge.IsValidUUID(opts.SessionID) {
		sessionValid = false
		checks = append(checks, Check{Name: "effective MCP_XCODE_SESSION_ID", Status: StatusFail, Detail: "MCP_XCODE_SESSION_ID must be a UUID"})
	} else {
		checks = append(checks, Check{Name: "effective MCP_XCODE_SESSION_ID", Status: StatusOK, Detail: opts.SessionID})
	}

	smokeEnv := bridge.ApplyEnvOverrides(opts.BaseEnv, bridge.EnvOptions{XcodePID: opts.XcodePID, SessionID: opts.SessionID})
	smokeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	switch {
	case !xcrunAvailable:
		checks = append(checks, Check{Name: "spawn smoke test", Status: StatusInfo, Detail: "skipped because xcrun is unavailable"})
	case !pidValid || !sessionValid:
		checks = append(checks, Check{Name: "spawn smoke test", Status: StatusInfo, Detail: "skipped because explicit overrides failed validation"})
	default:
		startedAt := time.Now()
		result, runErr := i.RunCommand(smokeCtx, CommandRequest{Name: xcrunPath, Args: []string{"mcpbridge"}, Env: smokeEnv})
		if smokeCtx.Err() == context.DeadlineExceeded {
			checks = append(checks, Check{Name: "spawn smoke test", Status: StatusFail, Detail: "timed out waiting for xcrun mcpbridge to exit with closed stdin"})
		} else if runErr != nil {
			checks = append(checks, Check{Name: "spawn smoke test", Status: StatusFail, Detail: formatCommandFailure(result, runErr)})
		} else {
			checks = append(checks, Check{Name: "spawn smoke test", Status: StatusOK, Detail: fmt.Sprintf("exit 0 in %s", time.Since(startedAt).Round(10*time.Millisecond))})
		}
	}

	return Report{Checks: checks}
}

func statusIcon(status Status) string {
	switch status {
	case StatusOK:
		return "[OK]"
	case StatusWarn:
		return "[WARN]"
	case StatusFail:
		return "[FAIL]"
	case StatusInfo:
		return "[INFO]"
	default:
		return "[?]"
	}
}

func formatCommandFailure(result CommandResult, err error) string {
	parts := []string{err.Error()}
	if result.ExitCode != 0 {
		parts = append(parts, fmt.Sprintf("exit %d", result.ExitCode))
	}
	if text := strings.TrimSpace(strings.TrimSpace(result.Stderr + " " + result.Stdout)); text != "" {
		parts = append(parts, text)
	}
	return strings.Join(parts, "; ")
}

func filterXcodeCandidates(processes []Process) []Process {
	candidates := make([]Process, 0)
	for _, proc := range processes {
		if looksLikeXcodeProcess(proc) {
			candidates = append(candidates, proc)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].PID < candidates[j].PID
	})
	return candidates
}

func summarizeProcesses(processes []Process) string {
	parts := make([]string, 0, len(processes))
	for _, proc := range processes {
		parts = append(parts, fmt.Sprintf("%d %s", proc.PID, proc.Command))
	}
	return strings.Join(parts, " | ")
}

func findProcess(processes []Process, pid int) (Process, bool) {
	for _, proc := range processes {
		if proc.PID == pid {
			return proc, true
		}
	}
	return Process{}, false
}

func looksLikeXcodeProcess(proc Process) bool {
	firstToken := proc.Command
	if fields := strings.Fields(proc.Command); len(fields) > 0 {
		firstToken = fields[0]
	}
	base := filepath.Base(firstToken)
	return strings.Contains(firstToken, "/Contents/MacOS/Xcode") || base == "Xcode"
}

func defaultRunCommand(ctx context.Context, req CommandRequest) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, req.Name, req.Args...)
	cmd.Env = req.Env
	cmd.Stdin = bytes.NewReader(req.Stdin)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	result := CommandResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err == nil {
		return result, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	}
	return result, err
}

func defaultListProcesses(ctx context.Context) ([]Process, error) {
	result, err := defaultRunCommand(ctx, CommandRequest{Name: "ps", Args: []string{"-axo", "pid=,command="}})
	if err != nil {
		return nil, fmt.Errorf("list processes: %s", formatCommandFailure(result, err))
	}

	lines := strings.Split(result.Stdout, "\n")
	processes := make([]Process, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pidStr, cmd, ok := splitProcessLine(line)
		if !ok {
			continue
		}
		pid, convErr := strconv.Atoi(pidStr)
		if convErr != nil {
			continue
		}
		processes = append(processes, Process{PID: pid, Command: cmd})
	}
	return processes, nil
}

func splitProcessLine(line string) (string, string, bool) {
	index := strings.IndexFunc(line, unicode.IsSpace)
	if index == -1 {
		return "", "", false
	}
	pid := strings.TrimSpace(line[:index])
	cmd := strings.TrimSpace(line[index:])
	if pid == "" || cmd == "" {
		return "", "", false
	}
	return pid, cmd, true
}
