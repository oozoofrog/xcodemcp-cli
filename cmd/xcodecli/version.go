package main

import "strings"

const sourceVersion = "v0.5.4"

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
