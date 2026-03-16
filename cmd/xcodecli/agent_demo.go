package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/oozoofrog/xcodecli/internal/agent"
	"github.com/oozoofrog/xcodecli/internal/bridge"
	"github.com/oozoofrog/xcodecli/internal/doctor"
)

const demoWindowsToolName = "XcodeListWindows"

var demoHighlightToolNames = []string{
	demoWindowsToolName,
	"XcodeLS",
	"XcodeRead",
	"BuildProject",
	"RunAllTests",
}

type demoStepError struct {
	Step    string `json:"step"`
	Message string `json:"message"`
}

type demoToolHighlight struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	RequiredArgs []string `json:"requiredArgs"`
}

type demoToolCatalog struct {
	Count      int                 `json:"count"`
	Names      []string            `json:"names"`
	Highlights []demoToolHighlight `json:"highlights"`
}

type demoWindowsResult struct {
	Attempted bool           `json:"attempted"`
	Ok        bool           `json:"ok"`
	ToolName  string         `json:"toolName"`
	Arguments map[string]any `json:"arguments"`
	Result    map[string]any `json:"result"`
	Error     *demoStepError `json:"error"`
}

type agentDemoReport struct {
	Success      bool              `json:"success"`
	Doctor       doctor.JSONReport `json:"doctor"`
	AgentStatus  *agent.Status     `json:"agentStatus"`
	ToolCatalog  demoToolCatalog   `json:"toolCatalog"`
	WindowsDemo  demoWindowsResult `json:"windowsDemo"`
	NextCommands []string          `json:"nextCommands"`
	Errors       []demoStepError   `json:"errors"`
}

func runAgentDemo(ctx context.Context, cfg cliConfig, env []string, stdout, stderr io.Writer, agentCfg agent.Config) int {
	resolved, err := resolveEffectiveOptions(env, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "xcodecli: %v\n", err)
		return 1
	}
	if cfg.Debug {
		logResolvedSession(stderr, resolved)
	}
	effective := resolved.EnvOptions
	if err := bridge.ValidateEnvOptions(effective); err != nil {
		fmt.Fprintf(stderr, "xcodecli: invalid MCP options: %v\n", err)
		return 1
	}

	initialStatus, initialStatusErr := defaultAgentStatusFunc(ctx, agentCfg)
	doctorReport := defaultDoctorRunFunc(ctx, doctor.Options{
		BaseEnv:        env,
		XcodePID:       resolved.XcodePID,
		SessionID:      resolved.SessionID,
		SessionSource:  resolved.SessionSource,
		SessionPath:    resolved.SessionPath,
		AgentStatus:    agentStatusPointer(initialStatus, initialStatusErr),
		AgentStatusErr: initialStatusErr,
	})

	report := agentDemoReport{
		Doctor: doctorReport.JSON(),
		ToolCatalog: demoToolCatalog{
			Names:      []string{},
			Highlights: []demoToolHighlight{},
		},
		WindowsDemo: demoWindowsResult{
			ToolName:  demoWindowsToolName,
			Arguments: map[string]any{},
		},
		NextCommands: agentDemoNextCommands(),
		Errors:       []demoStepError{},
	}

	request := agentRequest(env, effective, cfg)

	toolsCtx, cancelTools := requestTimeoutContext(ctx, cfg.Timeout)
	tools, toolsErr := defaultToolsListFunc(toolsCtx, agentCfg, request)
	cancelTools()
	if toolsErr != nil {
		report.addError("tools list", toolsErr.Error())
	} else {
		report.ToolCatalog = buildDemoToolCatalog(tools)
	}

	postToolsStatus, postToolsStatusErr := defaultAgentStatusFunc(ctx, agentCfg)
	if postToolsStatusErr != nil {
		report.addError("agent status", postToolsStatusErr.Error())
	} else {
		report.AgentStatus = &postToolsStatus
	}

	if toolsErr == nil {
		if _, found := findToolByName(tools, demoWindowsToolName); !found {
			report.WindowsDemo.Error = &demoStepError{Step: "windows demo", Message: "tool not found: XcodeListWindows"}
			report.addError("windows demo", "tool not found: XcodeListWindows")
		} else {
			report.WindowsDemo.Attempted = true
			callCtx, cancelCall := requestTimeoutContext(ctx, cfg.Timeout)
			result, callErr := defaultToolCallFunc(callCtx, agentCfg, request, demoWindowsToolName, report.WindowsDemo.Arguments)
			cancelCall()
			if callErr != nil {
				report.WindowsDemo.Error = &demoStepError{Step: "windows demo", Message: callErr.Error()}
				report.addError("windows demo", callErr.Error())
			} else {
				report.WindowsDemo.Result = result.Result
				report.WindowsDemo.Ok = !result.IsError
				if result.IsError {
					message := extractDemoToolMessage(result.Result)
					if message == "" {
						message = "tool returned isError=true"
					}
					report.WindowsDemo.Error = &demoStepError{Step: "windows demo", Message: message}
					report.addError("windows demo", message)
				}
			}
		}
	}

	report.Success = doctorReport.Success() && toolsErr == nil && report.WindowsDemo.Attempted && report.WindowsDemo.Ok

	if cfg.JSONOutput {
		if err := writeJSON(stdout, report); err != nil {
			fmt.Fprintf(stderr, "xcodecli: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprint(stdout, formatAgentDemo(report))
	}

	if report.Success {
		return 0
	}
	return 1
}

func agentStatusPointer(status agent.Status, err error) *agent.Status {
	if err != nil {
		return nil
	}
	return &status
}

func (r *agentDemoReport) addError(step, message string) {
	r.Errors = append(r.Errors, demoStepError{Step: step, Message: message})
}

func buildDemoToolCatalog(tools []map[string]any) demoToolCatalog {
	return buildToolCatalog(tools, demoHighlightToolNames)
}

func buildToolCatalog(tools []map[string]any, highlightNames []string) demoToolCatalog {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if name, _ := tool["name"].(string); name != "" {
			names = append(names, name)
		}
	}

	highlights := make([]demoToolHighlight, 0, len(highlightNames))
	for _, name := range highlightNames {
		tool, found := findToolByName(tools, name)
		if !found {
			continue
		}
		highlights = append(highlights, demoToolHighlight{
			Name:         name,
			Description:  stringValue(tool["description"]),
			RequiredArgs: requiredArgsFromTool(tool),
		})
	}

	return demoToolCatalog{
		Count:      len(names),
		Names:      names,
		Highlights: highlights,
	}
}

func requiredArgsFromTool(tool map[string]any) []string {
	inputSchema, ok := tool["inputSchema"].(map[string]any)
	if !ok {
		return []string{}
	}
	rawRequired, ok := inputSchema["required"]
	if !ok {
		return []string{}
	}
	switch values := rawRequired.(type) {
	case []string:
		return append([]string{}, values...)
	case []any:
		required := make([]string, 0, len(values))
		for _, value := range values {
			if str, ok := value.(string); ok && str != "" {
				required = append(required, str)
			}
		}
		return required
	default:
		return []string{}
	}
}

func agentDemoNextCommands() []string {
	return []string{
		`xcodecli tool inspect XcodeRead --json --timeout 60s`,
		`xcodecli tool call XcodeLS --timeout 60s --json '{"tabIdentifier":"<tabIdentifier from above>","path":""}'`,
		`xcodecli tool call XcodeRead --timeout 60s --json '{"tabIdentifier":"<tabIdentifier from above>","filePath":"<path from XcodeLS>"}'`,
	}
}

func formatAgentDemo(report agentDemoReport) string {
	var b strings.Builder
	b.WriteString("xcodecli agent demo\n\n")

	b.WriteString("Environment\n")
	b.WriteString("-----------\n")
	doctorSummary := report.Doctor.Summary
	doctorState := "ok"
	if !report.Doctor.Success {
		doctorState = "needs attention"
	}
	fmt.Fprintf(&b, "doctor: %s (%d ok, %d warn, %d fail, %d info)\n", doctorState, doctorSummary.OK, doctorSummary.Warn, doctorSummary.Fail, doctorSummary.Info)
	notableChecks := doctorNotableChecks(report.Doctor)
	if len(notableChecks) > 0 {
		b.WriteString("notable checks:\n")
		for _, check := range notableChecks {
			fmt.Fprintf(&b, "- %s [%s]: %s\n", check.Name, check.Status, check.Detail)
		}
	}
	if report.AgentStatus != nil {
		fmt.Fprintf(&b, "launchagent after tools discovery: running=%t socketReachable=%t backendSessions=%d\n", report.AgentStatus.Running, report.AgentStatus.SocketReachable, report.AgentStatus.BackendSessions)
	} else {
		b.WriteString("launchagent after tools discovery: unavailable\n")
	}
	if err := findDemoError(report.Errors, "agent status"); err != nil {
		fmt.Fprintf(&b, "agent status note: %s\n", err.Message)
	}

	b.WriteString("\nTool Catalog\n")
	b.WriteString("------------\n")
	fmt.Fprintf(&b, "count: %d\n", report.ToolCatalog.Count)
	if len(report.ToolCatalog.Names) > 0 {
		fmt.Fprintf(&b, "names: %s\n", strings.Join(report.ToolCatalog.Names, ", "))
	} else {
		b.WriteString("names: unavailable\n")
	}
	if len(report.ToolCatalog.Highlights) > 0 {
		b.WriteString("highlights:\n")
		for _, highlight := range report.ToolCatalog.Highlights {
			required := "none"
			if len(highlight.RequiredArgs) > 0 {
				required = strings.Join(highlight.RequiredArgs, ", ")
			}
			fmt.Fprintf(&b, "- %s (required: %s): %s\n", highlight.Name, required, highlight.Description)
		}
	}
	if err := findDemoError(report.Errors, "tools list"); err != nil {
		fmt.Fprintf(&b, "catalog error: %s\n", err.Message)
	}

	b.WriteString("\nSafe Live Demo\n")
	b.WriteString("--------------\n")
	fmt.Fprintf(&b, "tool: %s --json '{}'\n", report.WindowsDemo.ToolName)
	switch {
	case report.WindowsDemo.Ok:
		b.WriteString("status: ok\n")
		if message := extractDemoToolMessage(report.WindowsDemo.Result); message != "" {
			b.WriteString("output:\n")
			b.WriteString(indentText(message, "  "))
			if !strings.HasSuffix(message, "\n") {
				b.WriteString("\n")
			}
		}
	case report.WindowsDemo.Attempted:
		b.WriteString("status: failed\n")
		if report.WindowsDemo.Error != nil {
			fmt.Fprintf(&b, "error: %s\n", report.WindowsDemo.Error.Message)
		}
	default:
		b.WriteString("status: skipped\n")
		if report.WindowsDemo.Error != nil {
			fmt.Fprintf(&b, "reason: %s\n", report.WindowsDemo.Error.Message)
		}
	}

	b.WriteString("\nNext Commands\n")
	b.WriteString("-------------\n")
	for _, command := range report.NextCommands {
		fmt.Fprintf(&b, "- %s\n", command)
	}

	return b.String()
}

func doctorNotableChecks(report doctor.JSONReport) []doctor.Check {
	checks := make([]doctor.Check, 0, len(report.Checks))
	for _, check := range report.Checks {
		if check.Status == doctor.StatusOK {
			continue
		}
		checks = append(checks, check)
	}
	return checks
}

func findDemoError(errors []demoStepError, step string) *demoStepError {
	for i := range errors {
		if errors[i].Step == step {
			return &errors[i]
		}
	}
	return nil
}

func extractDemoToolMessage(result map[string]any) string {
	if result == nil {
		return ""
	}
	if structured, ok := result["structuredContent"].(map[string]any); ok {
		if message, ok := structured["message"].(string); ok && strings.TrimSpace(message) != "" {
			return message
		}
	}
	if content, ok := result["content"].([]any); ok {
		messages := make([]string, 0, len(content))
		for _, item := range content {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := block["text"].(string); ok && strings.TrimSpace(text) != "" {
				messages = append(messages, text)
			}
		}
		if len(messages) > 0 {
			return strings.Join(messages, "\n")
		}
	}
	var buf bytes.Buffer
	if err := writeJSON(&buf, result); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

func indentText(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func stringValue(value any) string {
	str, _ := value.(string)
	return str
}
