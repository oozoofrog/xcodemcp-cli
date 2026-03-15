package main

import (
	"fmt"
	"time"
)

const (
	defaultToolsListRequestTimeout   = 60 * time.Second
	defaultToolInspectRequestTimeout = 60 * time.Second
	defaultAgentGuideRequestTimeout  = 60 * time.Second
	defaultAgentDemoRequestTimeout   = 60 * time.Second

	defaultToolCallReadRequestTimeout  = 60 * time.Second
	defaultToolCallWriteRequestTimeout = 120 * time.Second
	defaultToolCallLongRequestTimeout  = 30 * time.Minute
	defaultToolCallFallbackTimeout     = 5 * time.Minute
)

func defaultToolCallTimeout(toolName string) time.Duration {
	switch toolName {
	case "XcodeListWindows", "XcodeLS", "XcodeGlob", "XcodeRead", "XcodeGrep", "GetBuildLog", "GetTestList", "XcodeListNavigatorIssues":
		return defaultToolCallReadRequestTimeout
	case "XcodeUpdate", "XcodeWrite", "XcodeRefreshCodeIssuesInFile":
		return defaultToolCallWriteRequestTimeout
	case "BuildProject", "RunAllTests", "RunSomeTests":
		return defaultToolCallLongRequestTimeout
	default:
		return defaultToolCallFallbackTimeout
	}
}

func formatTimeoutDuration(value time.Duration) string {
	if value <= 0 {
		return "0s"
	}
	if value%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(value/time.Hour))
	}
	if value >= 5*time.Minute && value%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(value/time.Minute))
	}
	if value%time.Second == 0 {
		return fmt.Sprintf("%ds", int(value/time.Second))
	}
	return value.String()
}
