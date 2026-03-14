package agent

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ensureLaunchAgentPlist(paths Paths, label, binaryPath string) (bool, string, error) {
	desired := renderLaunchAgentPlist(paths, label, binaryPath)
	existing, err := os.ReadFile(paths.PlistPath)
	if err == nil {
		registered, parseErr := readLaunchAgentBinaryPathFromBytes(existing)
		if parseErr != nil {
			registered = ""
		}
		if string(existing) == desired {
			return false, registered, nil
		}
		if err := os.MkdirAll(filepath.Dir(paths.PlistPath), 0o755); err != nil {
			return false, registered, fmt.Errorf("create LaunchAgents directory: %w", err)
		}
		if err := os.WriteFile(paths.PlistPath, []byte(desired), 0o644); err != nil {
			return false, registered, fmt.Errorf("write launch agent plist: %w", err)
		}
		return true, registered, nil
	}
	if !os.IsNotExist(err) {
		return false, "", fmt.Errorf("read launch agent plist: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.PlistPath), 0o755); err != nil {
		return false, "", fmt.Errorf("create LaunchAgents directory: %w", err)
	}
	if err := os.WriteFile(paths.PlistPath, []byte(desired), 0o644); err != nil {
		return false, "", fmt.Errorf("write launch agent plist: %w", err)
	}
	return true, "", nil
}

func readLaunchAgentBinaryPath(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return readLaunchAgentBinaryPathFromBytes(data)
}

func readLaunchAgentBinaryPathFromBytes(data []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	inProgramArguments := false
	for {
		tok, err := decoder.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", fmt.Errorf("decode launch agent plist: %w", err)
		}
		switch token := tok.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "key":
				var text string
				if err := decoder.DecodeElement(&text, &token); err != nil {
					return "", fmt.Errorf("decode launch agent plist key: %w", err)
				}
				inProgramArguments = text == "ProgramArguments"
			case "array":
			case "string":
				var text string
				if err := decoder.DecodeElement(&text, &token); err != nil {
					return "", fmt.Errorf("decode launch agent plist string: %w", err)
				}
				if inProgramArguments {
					return text, nil
				}
			}
		case xml.EndElement:
			if token.Name.Local == "array" && inProgramArguments {
				inProgramArguments = false
			}
		}
	}
	return "", nil
}

func renderLaunchAgentPlist(paths Paths, label, binaryPath string) string {
	return strings.TrimSpace(fmt.Sprintf(`
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>agent</string>
		<string>run</string>
		<string>--launch-agent</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, xmlEscape(label), xmlEscape(binaryPath), xmlEscape(paths.LogPath), xmlEscape(paths.LogPath))) + "\n"
}

func xmlEscape(raw string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(raw))
	return b.String()
}
