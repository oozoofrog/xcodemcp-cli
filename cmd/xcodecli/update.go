package main

import (
	"context"
	"fmt"
	"io"

	selfupdate "github.com/oozoofrog/xcodecli/internal/update"
)

var defaultSelfUpdateFunc = func(ctx context.Context, currentVersion string) (selfupdate.Result, error) {
	return selfupdate.Run(ctx, selfupdate.Config{CurrentVersion: currentVersion})
}

func runUpdate(ctx context.Context, stdout, stderr io.Writer) int {
	result, err := defaultSelfUpdateFunc(ctx, currentVersion())
	if err != nil {
		fmt.Fprintf(stderr, "xcodecli: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, formatUpdateResult(result))
	return 0
}

func formatUpdateResult(result selfupdate.Result) string {
	targetVersion := result.TargetVersion
	if targetVersion == "" {
		targetVersion = result.CurrentVersion
	}
	switch result.Mode {
	case "homebrew":
		if result.AlreadyUpToDate {
			return fmt.Sprintf("xcodecli is already up to date via Homebrew (%s)", targetVersion)
		}
		return fmt.Sprintf("updated xcodecli via Homebrew: %s -> %s", result.CurrentVersion, targetVersion)
	default:
		if result.AlreadyUpToDate {
			return fmt.Sprintf("xcodecli is already up to date (%s)", targetVersion)
		}
		return fmt.Sprintf("updated xcodecli: %s -> %s", result.CurrentVersion, targetVersion)
	}
}
