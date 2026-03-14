package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	LaunchAgentLabel   = "io.oozoofrog.xcodemcp"
	DefaultIdleTimeout = 10 * time.Minute
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

func ResolvePaths(homeDir string) Paths {
	supportDir := filepath.Join(homeDir, "Library", "Application Support", "xcodemcp")
	return Paths{
		SupportDir: supportDir,
		SocketPath: filepath.Join(supportDir, "daemon.sock"),
		PIDPath:    filepath.Join(supportDir, "daemon.pid"),
		LogPath:    filepath.Join(supportDir, "agent.log"),
		PlistPath:  filepath.Join(homeDir, "Library", "LaunchAgents", LaunchAgentLabel+".plist"),
	}
}
