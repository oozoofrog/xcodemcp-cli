package main

import "strings"

const sourceVersion = "v1.1.0"

var cliVersion = sourceVersion
var cliBuildChannel = "dev"

func currentVersion() string {
	version := strings.TrimSpace(cliVersion)
	if version == "" {
		return sourceVersion
	}
	return version
}

func versionLine() string {
	line := "xcodecli " + currentVersion()
	if strings.EqualFold(strings.TrimSpace(cliBuildChannel), "dev") {
		line += " (dev)"
	}
	return line
}
