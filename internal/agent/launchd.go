package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Launchd interface {
	Print(ctx context.Context, target string) (string, error)
	Bootstrap(ctx context.Context, domainTarget, plistPath string) error
	Kickstart(ctx context.Context, serviceTarget string) error
	Bootout(ctx context.Context, target string) error
}

type commandLaunchd struct{}

func defaultLaunchd() Launchd {
	return commandLaunchd{}
}

func launchAgentDomainTarget() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func launchAgentServiceTarget(label string) string {
	return launchAgentDomainTarget() + "/" + label
}

func (commandLaunchd) Print(ctx context.Context, target string) (string, error) {
	return runLaunchctl(ctx, "print", target)
}

func (commandLaunchd) Bootstrap(ctx context.Context, domainTarget, plistPath string) error {
	_, err := runLaunchctl(ctx, "bootstrap", domainTarget, plistPath)
	return err
}

func (commandLaunchd) Kickstart(ctx context.Context, serviceTarget string) error {
	_, err := runLaunchctl(ctx, "kickstart", serviceTarget)
	return err
}

func (commandLaunchd) Bootout(ctx context.Context, target string) error {
	_, err := runLaunchctl(ctx, "bootout", target)
	return err
}

func runLaunchctl(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "launchctl", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = strings.TrimSpace(stdout.String())
		}
		if text != "" {
			return stdout.String(), fmt.Errorf("launchctl %s: %w (%s)", strings.Join(args, " "), err, text)
		}
		return stdout.String(), fmt.Errorf("launchctl %s: %w", strings.Join(args, " "), err)
	}
	return stdout.String(), nil
}
