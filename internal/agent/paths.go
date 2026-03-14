package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	LaunchAgentLabel       = "io.oozoofrog.xcodecli"
	LegacyLaunchAgentLabel = "io.oozoofrog.xcodemcp"
	SupportDirName         = "xcodecli"
	LegacySupportDirName   = "xcodemcp"
	DefaultIdleTimeout     = 10 * time.Minute
)

type Paths struct {
	SupportDir string
	SocketPath string
	PIDPath    string
	LogPath    string
	PlistPath  string
}

func DefaultPaths() (Paths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home directory for agent paths: %w", err)
	}
	return ResolvePaths(homeDir), nil
}

func DefaultLegacyPaths() (Paths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home directory for legacy agent paths: %w", err)
	}
	return ResolveLegacyPaths(homeDir), nil
}

func ResolvePaths(homeDir string) Paths {
	return resolveNamedPaths(homeDir, SupportDirName, LaunchAgentLabel)
}

func ResolveLegacyPaths(homeDir string) Paths {
	return resolveNamedPaths(homeDir, LegacySupportDirName, LegacyLaunchAgentLabel)
}

func resolveNamedPaths(homeDir, supportDirName, label string) Paths {
	supportDir := filepath.Join(homeDir, "Library", "Application Support", supportDirName)
	return Paths{
		SupportDir: supportDir,
		SocketPath: filepath.Join(supportDir, "daemon.sock"),
		PIDPath:    filepath.Join(supportDir, "daemon.pid"),
		LogPath:    filepath.Join(supportDir, "agent.log"),
		PlistPath:  filepath.Join(homeDir, "Library", "LaunchAgents", label+".plist"),
	}
}
