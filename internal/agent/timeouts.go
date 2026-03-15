package agent

import (
	"fmt"
	"time"
)

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

func formatTimeoutMillis(timeoutMS int64) string {
	if timeoutMS <= 0 {
		return "the configured request timeout"
	}
	return formatTimeoutDuration(time.Duration(timeoutMS) * time.Millisecond)
}

func requestTimeoutError(timeoutMS int64, action string, cause error) error {
	return fmt.Errorf("request timed out after %s while %s: %w (this was the request timeout, not the mcpbridge session idle timeout)", formatTimeoutMillis(timeoutMS), action, cause)
}

func requestTimeoutAction(method, toolName string) string {
	switch method {
	case "tools/list":
		return "listing tools"
	case "tools/call":
		if toolName != "" {
			return fmt.Sprintf("calling %s", toolName)
		}
		return "calling a tool"
	default:
		return "waiting for request completion"
	}
}
