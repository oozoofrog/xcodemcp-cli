package agent

import (
	"os"
	"path/filepath"
	"strings"
)

func deriveStatusWarnings(status Status) []string {
	warnings := []string{}
	if strings.TrimSpace(status.RegisteredBinary) != "" && !filepath.IsAbs(status.RegisteredBinary) {
		warnings = append(warnings, "registered LaunchAgent binary path is relative; stale older installs can make launchctl bootstrap fail with Input/output error")
	}
	if strings.TrimSpace(status.RegisteredBinary) != "" {
		if info, err := os.Stat(status.RegisteredBinary); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			warnings = append(warnings, "registered LaunchAgent binary is missing or not executable; the next LaunchAgent bootstrap may fail until the plist is rewritten")
		}
	}
	if strings.TrimSpace(status.RegisteredBinary) != "" && strings.TrimSpace(status.CurrentBinary) != "" && !status.BinaryPathMatches {
		warnings = append(warnings, "registered LaunchAgent binary differs from the current binary; switching binaries recycles the backend session and can surface fresh Xcode authorization prompts")
	}
	return dedupeWarnings(warnings)
}

func deriveStatusNextSteps(status Status, warnings []string) []string {
	steps := []string{}
	if len(warnings) > 0 {
		steps = append(steps, "if the registration looks stale, run `xcodecli agent uninstall` and then re-register from one stable xcodecli path")
	}
	if status.PlistInstalled && !status.Running && !status.SocketReachable {
		steps = append(steps, "run `xcodecli agent demo` or `xcodecli tools list --json` to bootstrap the LaunchAgent again")
	}
	return steps
}

func dedupeWarnings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
