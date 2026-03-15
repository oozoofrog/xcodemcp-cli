package doctor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDoctorHelperFunctions(t *testing.T) {
	inspector := NewInspector()
	if inspector.LookPath == nil || inspector.RunCommand == nil || inspector.ListProcesses == nil {
		t.Fatalf("NewInspector returned incomplete inspector: %+v", inspector)
	}

	failure := formatCommandFailure(CommandResult{ExitCode: 7, Stderr: "stderr text", Stdout: "stdout text"}, errors.New("boom"))
	for _, want := range []string{"boom", "exit 7", "stderr text", "stdout text"} {
		if !strings.Contains(failure, want) {
			t.Fatalf("formatCommandFailure missing %q: %s", want, failure)
		}
	}

	pid, cmd, ok := splitProcessLine("123 /Applications/Xcode.app/Contents/MacOS/Xcode")
	if !ok || pid != "123" || cmd != "/Applications/Xcode.app/Contents/MacOS/Xcode" {
		t.Fatalf("splitProcessLine returned (%q, %q, %t)", pid, cmd, ok)
	}
	if _, _, ok := splitProcessLine("no-space"); ok {
		t.Fatal("splitProcessLine should reject lines without whitespace")
	}

	processes := []Process{
		{PID: 3, Command: "/bin/zsh"},
		{PID: 2, Command: "/Applications/Xcode.app/Contents/MacOS/Xcode"},
		{PID: 1, Command: "Xcode"},
	}
	filtered := filterXcodeCandidates(processes)
	if len(filtered) != 2 || filtered[0].PID != 1 || filtered[1].PID != 2 {
		t.Fatalf("unexpected filtered candidates: %+v", filtered)
	}
	if got := summarizeProcesses(filtered); !strings.Contains(got, "1 Xcode") || !strings.Contains(got, "2 /Applications/Xcode.app/Contents/MacOS/Xcode") {
		t.Fatalf("unexpected process summary: %s", got)
	}
	if proc, ok := findProcess(processes, 2); !ok || proc.PID != 2 {
		t.Fatalf("findProcess did not find expected pid: %+v ok=%t", proc, ok)
	}
	if !looksLikeXcodeProcess(Process{PID: 99, Command: "Xcode"}) {
		t.Fatal("looksLikeXcodeProcess should accept bare Xcode command")
	}
	if looksLikeXcodeProcess(Process{PID: 100, Command: "/bin/zsh"}) {
		t.Fatal("looksLikeXcodeProcess should reject unrelated process")
	}
}

func TestDefaultRunCommandAndDefaultListProcesses(t *testing.T) {
	result, err := defaultRunCommand(context.Background(), CommandRequest{Name: "/bin/sh", Args: []string{"-c", "printf hello"}})
	if err != nil {
		t.Fatalf("defaultRunCommand returned error: %v", err)
	}
	if result.Stdout != "hello" {
		t.Fatalf("stdout = %q, want hello", result.Stdout)
	}

	failResult, failErr := defaultRunCommand(context.Background(), CommandRequest{Name: "/bin/sh", Args: []string{"-c", "printf boom >&2; exit 7"}})
	if failErr == nil {
		t.Fatal("expected failing defaultRunCommand result")
	}
	if failResult.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", failResult.ExitCode)
	}
}
