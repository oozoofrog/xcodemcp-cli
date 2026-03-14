package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/oozoofrog/xcodemcp-cli/internal/agent"
	"github.com/oozoofrog/xcodemcp-cli/internal/bridge"
	"github.com/oozoofrog/xcodemcp-cli/internal/doctor"
)

const guideWorkflowCatalog = "catalog"

var guideWorkflowOrder = []string{"build", "test", "read", "search", "edit", "diagnose"}

var guideHighlightToolNames = []string{
	demoWindowsToolName,
	"BuildProject",
	"GetBuildLog",
	"RunAllTests",
	"GetTestList",
	"RunSomeTests",
	"XcodeLS",
	"XcodeRead",
	"XcodeGlob",
	"XcodeGrep",
	"XcodeUpdate",
	"XcodeWrite",
	"XcodeRefreshCodeIssuesInFile",
	"XcodeListNavigatorIssues",
}

var guideWorkflowExamples = map[string]string{
	"build":    "build Unicody",
	"test":     "run tests for Unicody",
	"read":     "read KeyboardState.swift",
	"search":   "search for AdManager",
	"edit":     "update KeyboardState.swift",
	"diagnose": "diagnose build errors",
}

var guideWorkflowTitles = map[string]string{
	guideWorkflowCatalog: "Workflow catalog overview",
	"build":              "Build a project",
	"test":               "Run tests",
	"read":               "Read a file",
	"search":             "Search code or files",
	"edit":               "Edit a file safely",
	"diagnose":           "Diagnose build or code issues",
}

var guideRelatedWorkflows = map[string][]string{
	"build":    {"diagnose", "test"},
	"test":     {"build", "diagnose"},
	"read":     {"search", "edit"},
	"search":   {"read", "diagnose"},
	"edit":     {"read", "diagnose"},
	"diagnose": {"build", "search"},
}

type guideIntentResult struct {
	Raw          string   `json:"raw"`
	WorkflowID   string   `json:"workflowId"`
	Confidence   float64  `json:"confidence"`
	Alternatives []string `json:"alternatives"`
}

type guideWindowEntry struct {
	TabIdentifier string `json:"tabIdentifier"`
	WorkspacePath string `json:"workspacePath"`
}

type guideWindowsResult struct {
	Attempted bool               `json:"attempted"`
	Ok        bool               `json:"ok"`
	ToolName  string             `json:"toolName"`
	Result    map[string]any     `json:"result,omitempty"`
	Entries   []guideWindowEntry `json:"entries"`
	Error     *demoStepError     `json:"error,omitempty"`
}

type guideEnvironment struct {
	Doctor      doctor.JSONReport  `json:"doctor"`
	AgentStatus *agent.Status      `json:"agentStatus,omitempty"`
	ToolCatalog demoToolCatalog    `json:"toolCatalog"`
	Windows     guideWindowsResult `json:"windows"`
}

type guideWorkflowStep struct {
	Why               string         `json:"why"`
	ToolName          string         `json:"toolName"`
	ArgumentsTemplate map[string]any `json:"argumentsTemplate"`
	WhenToSkip        string         `json:"whenToSkip"`
}

type guideWorkflowFallback struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Commands    []string `json:"commands"`
}

type guideWorkflowResult struct {
	ID        string                  `json:"id"`
	Title     string                  `json:"title"`
	Reason    string                  `json:"reason"`
	Steps     []guideWorkflowStep     `json:"steps"`
	Fallbacks []guideWorkflowFallback `json:"fallbacks"`
}

type agentGuideReport struct {
	Success      bool                `json:"success"`
	Intent       guideIntentResult   `json:"intent"`
	Environment  guideEnvironment    `json:"environment"`
	Workflow     guideWorkflowResult `json:"workflow"`
	NextCommands []string            `json:"nextCommands"`
	Errors       []demoStepError     `json:"errors"`
}

type guideIntentMatch struct {
	Raw          string
	WorkflowID   string
	Confidence   float64
	Alternatives []string
	Subject      string
}

type guideWindowMatch struct {
	MatchedEntry *guideWindowEntry
	Ambiguous    bool
	Note         string
}

func runAgentGuide(ctx context.Context, cfg cliConfig, env []string, stdout, stderr io.Writer, agentCfg agent.Config) int {
	resolved, err := resolveEffectiveOptions(env, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "xcodemcp: %v\n", err)
		return 1
	}
	if cfg.Debug {
		logResolvedSession(stderr, resolved)
	}
	effective := resolved.EnvOptions
	if err := bridge.ValidateEnvOptions(effective); err != nil {
		fmt.Fprintf(stderr, "xcodemcp: invalid MCP options: %v\n", err)
		return 1
	}

	intent := classifyGuideIntent(cfg.Intent)
	environment, errors := collectGuideEnvironment(ctx, cfg, env, agentCfg, resolved)
	windowMatch := resolveGuideWindowMatch(environment.Windows.Entries, intent.Subject)
	workflow, nextCommands := buildGuideWorkflow(intent, environment, windowMatch)

	report := agentGuideReport{
		Success: len(errors) == 0,
		Intent: guideIntentResult{
			Raw:          intent.Raw,
			WorkflowID:   intent.WorkflowID,
			Confidence:   intent.Confidence,
			Alternatives: append([]string{}, intent.Alternatives...),
		},
		Environment:  environment,
		Workflow:     workflow,
		NextCommands: nextCommands,
		Errors:       append([]demoStepError(nil), errors...),
	}

	if cfg.JSONOutput {
		if err := writeJSON(stdout, report); err != nil {
			fmt.Fprintf(stderr, "xcodemcp: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Fprint(stdout, formatAgentGuide(report, windowMatch))
	return 0
}

func collectGuideEnvironment(ctx context.Context, cfg cliConfig, env []string, agentCfg agent.Config, resolved bridge.ResolvedOptions) (guideEnvironment, []demoStepError) {
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

	environment := guideEnvironment{
		Doctor: doctorReport.JSON(),
		ToolCatalog: demoToolCatalog{
			Names:      []string{},
			Highlights: []demoToolHighlight{},
		},
		Windows: guideWindowsResult{
			ToolName: demoWindowsToolName,
			Entries:  []guideWindowEntry{},
		},
	}
	errors := []demoStepError{}

	request := agentRequest(env, resolved.EnvOptions, cfg)

	toolsCtx, cancelTools := requestTimeoutContext(ctx, cfg.Timeout)
	tools, toolsErr := defaultToolsListFunc(toolsCtx, agentCfg, request)
	cancelTools()
	if toolsErr != nil {
		errors = append(errors, demoStepError{Step: "tools list", Message: toolsErr.Error()})
	} else {
		environment.ToolCatalog = buildGuideToolCatalog(tools)
	}

	postToolsStatus, postToolsStatusErr := defaultAgentStatusFunc(ctx, agentCfg)
	if postToolsStatusErr != nil {
		errors = append(errors, demoStepError{Step: "agent status", Message: postToolsStatusErr.Error()})
	} else {
		environment.AgentStatus = &postToolsStatus
	}

	if toolsErr != nil {
		return environment, errors
	}
	if _, found := findToolByName(tools, demoWindowsToolName); !found {
		environment.Windows.Error = &demoStepError{Step: "windows", Message: "tool not found: XcodeListWindows"}
		errors = append(errors, *environment.Windows.Error)
		return environment, errors
	}

	environment.Windows.Attempted = true
	windowsCtx, cancelWindows := requestTimeoutContext(ctx, cfg.Timeout)
	result, windowsErr := defaultToolCallFunc(windowsCtx, agentCfg, request, demoWindowsToolName, map[string]any{})
	cancelWindows()
	if windowsErr != nil {
		environment.Windows.Error = &demoStepError{Step: "windows", Message: windowsErr.Error()}
		errors = append(errors, *environment.Windows.Error)
		return environment, errors
	}

	environment.Windows.Result = result.Result
	environment.Windows.Ok = !result.IsError
	if result.IsError {
		message := extractDemoToolMessage(result.Result)
		if message == "" {
			message = "tool returned isError=true"
		}
		environment.Windows.Error = &demoStepError{Step: "windows", Message: message}
		errors = append(errors, *environment.Windows.Error)
		return environment, errors
	}
	environment.Windows.Entries = parseGuideWindowEntries(result.Result)
	return environment, errors
}

func buildGuideToolCatalog(tools []map[string]any) demoToolCatalog {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if name, _ := tool["name"].(string); name != "" {
			names = append(names, name)
		}
	}

	highlights := make([]demoToolHighlight, 0, len(guideHighlightToolNames))
	for _, name := range guideHighlightToolNames {
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

func parseGuideWindowEntries(result map[string]any) []guideWindowEntry {
	message := extractDemoToolMessage(result)
	if strings.TrimSpace(message) == "" {
		return []guideWindowEntry{}
	}
	lines := strings.Split(message, "\n")
	entries := make([]guideWindowEntry, 0, len(lines))
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "* ")
		const leftPrefix = "tabIdentifier: "
		const middle = ", workspacePath: "
		if !strings.HasPrefix(line, leftPrefix) {
			continue
		}
		rest := strings.TrimPrefix(line, leftPrefix)
		middleIndex := strings.Index(rest, middle)
		if middleIndex == -1 {
			continue
		}
		tabIdentifier := strings.TrimSpace(rest[:middleIndex])
		workspacePath := strings.TrimSpace(rest[middleIndex+len(middle):])
		if tabIdentifier == "" || workspacePath == "" {
			continue
		}
		entries = append(entries, guideWindowEntry{
			TabIdentifier: tabIdentifier,
			WorkspacePath: workspacePath,
		})
	}
	return entries
}

func classifyGuideIntent(raw string) guideIntentMatch {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return guideIntentMatch{
			Raw:          "",
			WorkflowID:   guideWorkflowCatalog,
			Confidence:   1,
			Alternatives: append([]string{}, guideWorkflowOrder...),
			Subject:      "",
		}
	}

	text := strings.ToLower(trimmed)
	scores := map[string]int{
		"build":    0,
		"test":     0,
		"read":     0,
		"search":   0,
		"edit":     0,
		"diagnose": 0,
	}

	addGuideScore(scores, "diagnose", text, 5, "error", "warning", "fail", "failed", "issue", "issues", "log", "diagnostic", "diagnostics")
	addGuideScore(scores, "build", text, 3, "build", "compile", "rebuild", "app", "project")
	addGuideScore(scores, "test", text, 4, "test", "tests", "xctest", "ui test", "uitest")
	addGuideScore(scores, "read", text, 4, "read", "open", "show", "view", "inspect file", "source")
	addGuideScore(scores, "search", text, 4, "find", "search", "grep", "where", "list files")
	addGuideScore(scores, "edit", text, 4, "edit", "change", "update", "replace", "write", "create", "modify")

	if strings.Contains(text, ".swift") || strings.Contains(text, ".plist") || strings.Contains(text, ".xcodeproj") || strings.Contains(text, ".xcworkspace") {
		scores["read"] += 2
	}
	if strings.Contains(text, "build error") || strings.Contains(text, "build failure") {
		scores["diagnose"] += 3
	}
	if strings.Contains(text, "run tests") || strings.Contains(text, "all tests") {
		scores["test"] += 3
	}

	bestWorkflow := ""
	bestScore := -1
	for _, workflowID := range guideWorkflowOrder {
		if scores[workflowID] > bestScore {
			bestWorkflow = workflowID
			bestScore = scores[workflowID]
		}
	}
	if bestScore <= 0 {
		bestWorkflow = "search"
		bestScore = 1
	}

	type candidate struct {
		workflowID string
		score      int
	}
	candidates := make([]candidate, 0, len(scores))
	for workflowID, score := range scores {
		if workflowID == bestWorkflow {
			continue
		}
		candidates = append(candidates, candidate{workflowID: workflowID, score: score})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].workflowID < candidates[j].workflowID
		}
		return candidates[i].score > candidates[j].score
	})
	alternatives := make([]string, 0, 2)
	for _, candidate := range candidates {
		if candidate.score <= 0 {
			continue
		}
		alternatives = append(alternatives, candidate.workflowID)
		if len(alternatives) == 2 {
			break
		}
	}
	if len(alternatives) < 2 {
		for _, workflowID := range guideRelatedWorkflows[bestWorkflow] {
			if workflowID == bestWorkflow || containsString(alternatives, workflowID) {
				continue
			}
			alternatives = append(alternatives, workflowID)
			if len(alternatives) == 2 {
				break
			}
		}
	}

	confidence := 0.35 + float64(bestScore)*0.1
	if confidence > 0.99 {
		confidence = 0.99
	}

	return guideIntentMatch{
		Raw:          trimmed,
		WorkflowID:   bestWorkflow,
		Confidence:   confidence,
		Alternatives: alternatives,
		Subject:      extractGuideSubject(trimmed, bestWorkflow),
	}
}

func addGuideScore(scores map[string]int, workflowID, text string, value int, keywords ...string) {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			scores[workflowID] += value
		}
	}
}

func extractGuideSubject(raw, workflowID string) string {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	prefixes := map[string][]string{
		"build":    {"build ", "compile ", "rebuild "},
		"test":     {"run all tests for ", "run tests for ", "run test for ", "test ", "tests for ", "run all tests ", "run tests "},
		"read":     {"inspect file ", "inspect ", "read ", "open ", "show ", "view ", "source "},
		"search":   {"search for ", "search ", "find ", "grep ", "where is ", "where "},
		"edit":     {"update ", "edit ", "change ", "replace ", "write ", "create ", "modify "},
		"diagnose": {"diagnose ", "fix ", "investigate ", "debug "},
	}
	for _, prefix := range prefixes[workflowID] {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(trimmed[len(prefix):])
		}
	}
	return trimmed
}

func resolveGuideWindowMatch(entries []guideWindowEntry, subject string) guideWindowMatch {
	if len(entries) == 0 {
		return guideWindowMatch{Note: "No live Xcode windows were discovered."}
	}
	tokens := guideWindowMatchTokens(subject)
	if len(tokens) == 0 {
		return guideWindowMatch{Note: "No workspace or project hint was detected in the request, so tabIdentifier is still a placeholder."}
	}

	bestScore := 0
	bestIndexes := []int{}
	for index := range entries {
		score := guideWindowEntryScore(entries[index], tokens)
		switch {
		case score > bestScore:
			bestScore = score
			bestIndexes = []int{index}
		case score == bestScore && score > 0:
			bestIndexes = append(bestIndexes, index)
		}
	}
	if bestScore == 0 {
		return guideWindowMatch{Note: "No current Xcode window matched the project hint, so tabIdentifier is still a placeholder."}
	}
	if len(bestIndexes) > 1 {
		return guideWindowMatch{Ambiguous: true, Note: "More than one Xcode window matched the project hint. Keep tabIdentifier as a placeholder until you choose one."}
	}
	entry := entries[bestIndexes[0]]
	return guideWindowMatch{
		MatchedEntry: &guideWindowEntry{
			TabIdentifier: entry.TabIdentifier,
			WorkspacePath: entry.WorkspacePath,
		},
		Note: fmt.Sprintf("Matched %s to %s.", entry.TabIdentifier, entry.WorkspacePath),
	}
}

func guideWindowMatchTokens(subject string) []string {
	normalized := strings.ToLower(subject)
	replacer := strings.NewReplacer(
		".xcodeproj", " ",
		".xcworkspace", " ",
		".swift", " ",
		".m", " ",
		".mm", " ",
		".plist", " ",
		".md", " ",
		"/", " ",
		"-", " ",
		"_", " ",
		".", " ",
		",", " ",
		":", " ",
	)
	normalized = replacer.Replace(normalized)
	parts := strings.Fields(normalized)
	stopwords := map[string]struct{}{
		"build": {}, "compile": {}, "rebuild": {}, "test": {}, "tests": {}, "run": {}, "read": {}, "search": {}, "find": {},
		"edit": {}, "update": {}, "replace": {}, "write": {}, "create": {}, "modify": {}, "fix": {}, "error": {}, "errors": {},
		"warning": {}, "warnings": {}, "issue": {}, "issues": {}, "project": {}, "workspace": {}, "file": {}, "source": {},
		"for": {}, "the": {}, "a": {}, "an": {}, "in": {}, "on": {}, "of": {}, "all": {}, "diagnose": {}, "debug": {},
	}
	tokens := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		if len(part) < 2 {
			continue
		}
		if _, skip := stopwords[part]; skip {
			continue
		}
		tokens = append(tokens, part)
	}
	return uniqueStrings(tokens)
}

func guideWindowEntryScore(entry guideWindowEntry, tokens []string) int {
	pathLower := strings.ToLower(entry.WorkspacePath)
	baseLower := strings.ToLower(filepath.Base(entry.WorkspacePath))
	stemLower := strings.TrimSuffix(baseLower, filepath.Ext(baseLower))
	segments := guidePathSegments(entry.WorkspacePath)

	best := 0
	for _, token := range tokens {
		switch {
		case stemLower == token:
			best = maxInt(best, 100)
		case strings.Contains(stemLower, token):
			best = maxInt(best, 90)
		case strings.Contains(baseLower, token):
			best = maxInt(best, 80)
		case containsGuidePathSegment(segments, token):
			best = maxInt(best, 70)
		case strings.Contains(pathLower, token):
			best = maxInt(best, 50)
		}
	}
	return best
}

func guidePathSegments(path string) []string {
	rawSegments := strings.FieldsFunc(strings.ToLower(path), func(r rune) bool {
		switch r {
		case '/', '-', '_', '.', ' ':
			return true
		default:
			return false
		}
	})
	return uniqueStrings(rawSegments)
}

func containsGuidePathSegment(segments []string, token string) bool {
	for _, segment := range segments {
		if segment == token || strings.Contains(segment, token) {
			return true
		}
	}
	return false
}

func buildGuideWorkflow(intent guideIntentMatch, environment guideEnvironment, windowMatch guideWindowMatch) (guideWorkflowResult, []string) {
	if intent.WorkflowID == guideWorkflowCatalog {
		return buildGuideCatalogWorkflow()
	}

	tabIdentifier := "<tabIdentifier from XcodeListWindows>"
	if windowMatch.MatchedEntry != nil {
		tabIdentifier = windowMatch.MatchedEntry.TabIdentifier
	}

	switch intent.WorkflowID {
	case "build":
		return buildGuideBuildWorkflow(intent, tabIdentifier, windowMatch)
	case "test":
		return buildGuideTestWorkflow(intent, tabIdentifier, windowMatch)
	case "read":
		return buildGuideReadWorkflow(intent, tabIdentifier, windowMatch)
	case "search":
		return buildGuideSearchWorkflow(intent, tabIdentifier, windowMatch)
	case "edit":
		return buildGuideEditWorkflow(intent, tabIdentifier, windowMatch)
	case "diagnose":
		return buildGuideDiagnoseWorkflow(intent, tabIdentifier, windowMatch)
	default:
		return buildGuideCatalogWorkflow()
	}
}

func buildGuideCatalogWorkflow() (guideWorkflowResult, []string) {
	lines := []guideWorkflowStep{}
	for _, workflowID := range guideWorkflowOrder {
		lines = append(lines, guideWorkflowStep{
			Why:               fmt.Sprintf("Representative request: %q", guideWorkflowExamples[workflowID]),
			ToolName:          strings.Join(guideWorkflowToolChain(workflowID), " -> "),
			ArgumentsTemplate: map[string]any{},
			WhenToSkip:        "Skip this overview once you know which workflow family matches your request.",
		})
	}

	nextCommands := make([]string, 0, len(guideWorkflowOrder))
	for _, workflowID := range guideWorkflowOrder {
		nextCommands = append(nextCommands, fmt.Sprintf(`xcodemcp agent guide %s`, shellQuote(guideWorkflowExamples[workflowID])))
	}

	return guideWorkflowResult{
		ID:     guideWorkflowCatalog,
		Title:  guideWorkflowTitles[guideWorkflowCatalog],
		Reason: "No specific intent was provided, so this is a broad overview of the most common workflow families.",
		Steps:  lines,
		Fallbacks: []guideWorkflowFallback{
			{
				Title:       "If you already know the request",
				Description: "Re-run agent guide with the exact user intent to get concrete next commands and, when possible, a real tabIdentifier.",
				Commands:    nextCommands,
			},
			{
				Title:       "If you want safe live context first",
				Description: "Use agent demo to see the live window list and current tool catalog before picking a workflow.",
				Commands:    []string{"xcodemcp agent demo"},
			},
		},
	}, nextCommands
}

func buildGuideBuildWorkflow(intent guideIntentMatch, tabIdentifier string, windowMatch guideWindowMatch) (guideWorkflowResult, []string) {
	steps := []guideWorkflowStep{
		{
			Why:               "Use XcodeListWindows to identify the correct tabIdentifier for the project you want to build.",
			ToolName:          "XcodeListWindows",
			ArgumentsTemplate: map[string]any{},
			WhenToSkip:        guideWindowSkipReason(windowMatch),
		},
		{
			Why:               "BuildProject asks Xcode to build the active project or workspace shown in that tab.",
			ToolName:          "BuildProject",
			ArgumentsTemplate: map[string]any{"tabIdentifier": tabIdentifier},
			WhenToSkip:        "Skip only if you decided not to build after checking the window list.",
		},
		{
			Why:               "GetBuildLog is the fastest follow-up when BuildProject fails and you need the actionable error summary.",
			ToolName:          "GetBuildLog",
			ArgumentsTemplate: map[string]any{"tabIdentifier": tabIdentifier, "severity": "error"},
			WhenToSkip:        "Skip unless the build reports errors or you need error-only output.",
		},
	}

	nextCommands := buildGuideBuildCommands(tabIdentifier, windowMatch)
	fallbacks := []guideWorkflowFallback{
		{
			Title:       "If the window match looks wrong",
			Description: "Re-check the live Xcode windows and swap in the exact tabIdentifier yourself.",
			Commands: []string{
				`xcodemcp tool call XcodeListWindows --json '{}'`,
				formatBuildProjectCommand("<tabIdentifier from above>"),
			},
		},
		{
			Title:       "If you want schema reassurance",
			Description: "Inspect the tool schemas before executing the build flow.",
			Commands: []string{
				`xcodemcp tool inspect BuildProject --json`,
				`xcodemcp tool inspect GetBuildLog --json`,
			},
		},
	}

	return guideWorkflowResult{
		ID:        "build",
		Title:     guideWorkflowTitles["build"],
		Reason:    guideReasonForIntent(intent, windowMatch, "The request is about building, so the shortest safe sequence is window resolution -> build -> build log on failure."),
		Steps:     steps,
		Fallbacks: fallbacks,
	}, nextCommands
}

func buildGuideTestWorkflow(intent guideIntentMatch, tabIdentifier string, windowMatch guideWindowMatch) (guideWorkflowResult, []string) {
	steps := []guideWorkflowStep{
		{
			Why:               "Use XcodeListWindows first so the test run targets the correct workspace tab.",
			ToolName:          "XcodeListWindows",
			ArgumentsTemplate: map[string]any{},
			WhenToSkip:        guideWindowSkipReason(windowMatch),
		},
		{
			Why:               "RunAllTests is the fastest default when the request is to run the current scheme's full test plan.",
			ToolName:          "RunAllTests",
			ArgumentsTemplate: map[string]any{"tabIdentifier": tabIdentifier},
			WhenToSkip:        "Skip this step only if you already know you want a narrower subset of tests.",
		},
		{
			Why:               "GetTestList lets you narrow the run to specific tests before using RunSomeTests.",
			ToolName:          "GetTestList",
			ArgumentsTemplate: map[string]any{"tabIdentifier": tabIdentifier},
			WhenToSkip:        "Skip if running the full test plan is acceptable.",
		},
		{
			Why:               "GetBuildLog surfaces the underlying failure output if the test run fails early or produces build errors.",
			ToolName:          "GetBuildLog",
			ArgumentsTemplate: map[string]any{"tabIdentifier": tabIdentifier, "severity": "error"},
			WhenToSkip:        "Skip unless the test run fails or emits build errors.",
		},
	}

	nextCommands := buildGuideTestCommands(tabIdentifier, windowMatch)
	fallbacks := []guideWorkflowFallback{
		{
			Title:       "If you need to run only some tests",
			Description: "Enumerate the available tests first, then switch to RunSomeTests with targetName and testIdentifier values from the list.",
			Commands: []string{
				formatGetTestListCommand(tabIdentifier),
				formatRunSomeTestsTemplate(tabIdentifier),
			},
		},
		{
			Title:       "If schema details matter",
			Description: "Inspect the testing tool schemas before composing a narrower payload.",
			Commands: []string{
				`xcodemcp tool inspect GetTestList --json`,
				`xcodemcp tool inspect RunSomeTests --json`,
			},
		},
	}

	return guideWorkflowResult{
		ID:        "test",
		Title:     guideWorkflowTitles["test"],
		Reason:    guideReasonForIntent(intent, windowMatch, "The request is about tests, so the default path is window resolution -> full test run -> narrower test selection only if needed."),
		Steps:     steps,
		Fallbacks: fallbacks,
	}, nextCommands
}

func buildGuideReadWorkflow(intent guideIntentMatch, tabIdentifier string, windowMatch guideWindowMatch) (guideWorkflowResult, []string) {
	lookupTool := "XcodeLS"
	lookupArgs := map[string]any{"tabIdentifier": tabIdentifier, "path": ""}
	lookupWhy := "XcodeLS is the simplest starting point when you only need to browse the project tree before opening a file."
	readPathPlaceholder := "<path from XcodeLS>"
	subject := strings.TrimSpace(intent.Subject)
	if looksLikeFileHint(subject) {
		lookupTool = "XcodeGlob"
		lookupArgs = map[string]any{"tabIdentifier": tabIdentifier, "pattern": guideGlobPattern(subject)}
		lookupWhy = "XcodeGlob is faster when the request already hints at a filename or file extension."
		readPathPlaceholder = "<path from XcodeGlob>"
	}

	steps := []guideWorkflowStep{
		{
			Why:               "Use XcodeListWindows first so the subsequent file operations point at the right workspace tab.",
			ToolName:          "XcodeListWindows",
			ArgumentsTemplate: map[string]any{},
			WhenToSkip:        guideWindowSkipReason(windowMatch),
		},
		{
			Why:               lookupWhy,
			ToolName:          lookupTool,
			ArgumentsTemplate: lookupArgs,
			WhenToSkip:        "Skip if you already know the exact project-relative file path.",
		},
		{
			Why:               "XcodeRead opens the target source file once you have its project-relative path.",
			ToolName:          "XcodeRead",
			ArgumentsTemplate: map[string]any{"tabIdentifier": tabIdentifier, "filePath": readPathPlaceholder},
			WhenToSkip:        "Skip only if the earlier lookup already answered the question without opening the file.",
		},
	}

	nextCommands := buildGuideReadCommands(tabIdentifier, subject, windowMatch)
	fallbacks := []guideWorkflowFallback{
		{
			Title:       "If the file path is still unclear",
			Description: "Browse the project tree manually before opening the file.",
			Commands: []string{
				formatMaybeWindowsCommand(windowMatch),
				formatXcodeLSCommand(tabIdentifier, ""),
			},
		},
		{
			Title:       "If you want schema reassurance",
			Description: "Inspect the lookup and read schemas before composing a larger payload.",
			Commands: []string{
				fmt.Sprintf(`xcodemcp tool inspect %s --json`, lookupTool),
				`xcodemcp tool inspect XcodeRead --json`,
			},
		},
	}

	return guideWorkflowResult{
		ID:        "read",
		Title:     guideWorkflowTitles["read"],
		Reason:    guideReasonForIntent(intent, windowMatch, "The request is about reading source, so the efficient path is window resolution -> file lookup -> file read."),
		Steps:     steps,
		Fallbacks: fallbacks,
	}, nextCommands
}

func buildGuideSearchWorkflow(intent guideIntentMatch, tabIdentifier string, windowMatch guideWindowMatch) (guideWorkflowResult, []string) {
	subject := strings.TrimSpace(intent.Subject)
	searchTool := "XcodeGrep"
	searchArgs := map[string]any{"tabIdentifier": tabIdentifier, "pattern": guideSearchPattern(subject), "outputMode": "content", "showLineNumbers": true}
	searchWhy := "XcodeGrep is the best default when the request is to find a symbol or text inside files."
	if looksLikeFileHint(subject) {
		searchTool = "XcodeGlob"
		searchArgs = map[string]any{"tabIdentifier": tabIdentifier, "pattern": guideGlobPattern(subject)}
		searchWhy = "XcodeGlob is better when the request looks like a filename search instead of a content search."
	}

	steps := []guideWorkflowStep{
		{
			Why:               "Use XcodeListWindows first so the search runs against the right project tab.",
			ToolName:          "XcodeListWindows",
			ArgumentsTemplate: map[string]any{},
			WhenToSkip:        guideWindowSkipReason(windowMatch),
		},
		{
			Why:               searchWhy,
			ToolName:          searchTool,
			ArgumentsTemplate: searchArgs,
			WhenToSkip:        "Skip only if you already have the exact file path or symbol location.",
		},
	}

	nextCommands := buildGuideSearchCommands(tabIdentifier, subject, windowMatch)
	fallbacks := []guideWorkflowFallback{
		{
			Title:       "If the first search is too broad",
			Description: "Refine the glob, grep pattern, or output mode after you see the initial results.",
			Commands: []string{
				`xcodemcp tool inspect XcodeGrep --json`,
				`xcodemcp tool inspect XcodeGlob --json`,
			},
		},
	}

	return guideWorkflowResult{
		ID:        "search",
		Title:     guideWorkflowTitles["search"],
		Reason:    guideReasonForIntent(intent, windowMatch, "The request is about locating code or files, so the shortest safe path is window resolution -> targeted search."),
		Steps:     steps,
		Fallbacks: fallbacks,
	}, nextCommands
}

func buildGuideEditWorkflow(intent guideIntentMatch, tabIdentifier string, windowMatch guideWindowMatch) (guideWorkflowResult, []string) {
	subject := strings.TrimSpace(intent.Subject)
	lookupTool := "XcodeLS"
	lookupArgs := map[string]any{"tabIdentifier": tabIdentifier, "path": ""}
	pathPlaceholder := "<path from XcodeLS>"
	if looksLikeFileHint(subject) {
		lookupTool = "XcodeGlob"
		lookupArgs = map[string]any{"tabIdentifier": tabIdentifier, "pattern": guideGlobPattern(subject)}
		pathPlaceholder = "<path from XcodeGlob>"
	}

	steps := []guideWorkflowStep{
		{
			Why:               "Use XcodeListWindows first so the edit applies to the right workspace tab.",
			ToolName:          "XcodeListWindows",
			ArgumentsTemplate: map[string]any{},
			WhenToSkip:        guideWindowSkipReason(windowMatch),
		},
		{
			Why:               "Read the target file before changing it so you can compose the smallest safe edit payload.",
			ToolName:          lookupTool,
			ArgumentsTemplate: lookupArgs,
			WhenToSkip:        "Skip if you already know the exact project-relative path.",
		},
		{
			Why:               "Open the file contents before deciding between XcodeUpdate and XcodeWrite.",
			ToolName:          "XcodeRead",
			ArgumentsTemplate: map[string]any{"tabIdentifier": tabIdentifier, "filePath": pathPlaceholder},
			WhenToSkip:        "Skip only if you already have the file contents in hand.",
		},
		{
			Why:               "XcodeUpdate is the safer first choice for targeted in-file replacements.",
			ToolName:          "XcodeUpdate",
			ArgumentsTemplate: map[string]any{"tabIdentifier": tabIdentifier, "filePath": pathPlaceholder, "oldString": "<exact text to replace>", "newString": "<replacement text>"},
			WhenToSkip:        "Skip this step if the change is a full-file rewrite, in which case XcodeWrite may be simpler.",
		},
		{
			Why:               "Refresh diagnostics immediately after the edit so you can verify that the file still parses or compiles cleanly.",
			ToolName:          "XcodeRefreshCodeIssuesInFile",
			ArgumentsTemplate: map[string]any{"tabIdentifier": tabIdentifier, "filePath": pathPlaceholder},
			WhenToSkip:        "Skip only if you intentionally want to postpone validation.",
		},
	}

	nextCommands := buildGuideEditCommands(tabIdentifier, subject, windowMatch)
	fallbacks := []guideWorkflowFallback{
		{
			Title:       "If the change is a full rewrite",
			Description: "Switch from XcodeUpdate to XcodeWrite once you know the entire target file contents.",
			Commands: []string{
				`xcodemcp tool inspect XcodeWrite --json`,
				formatXcodeWriteTemplate(tabIdentifier, pathPlaceholder),
			},
		},
		{
			Title:       "If you want schema reassurance",
			Description: "Inspect the edit tool schemas before composing a large replacement payload.",
			Commands: []string{
				`xcodemcp tool inspect XcodeUpdate --json`,
				`xcodemcp tool inspect XcodeRefreshCodeIssuesInFile --json`,
			},
		},
	}

	return guideWorkflowResult{
		ID:        "edit",
		Title:     guideWorkflowTitles["edit"],
		Reason:    guideReasonForIntent(intent, windowMatch, "The request is about changing code, so the safe path is window resolution -> locate/read the file -> small edit -> refresh diagnostics."),
		Steps:     steps,
		Fallbacks: fallbacks,
	}, nextCommands
}

func buildGuideDiagnoseWorkflow(intent guideIntentMatch, tabIdentifier string, windowMatch guideWindowMatch) (guideWorkflowResult, []string) {
	steps := []guideWorkflowStep{
		{
			Why:               "Use XcodeListWindows first so the diagnostics query targets the right workspace tab.",
			ToolName:          "XcodeListWindows",
			ArgumentsTemplate: map[string]any{},
			WhenToSkip:        guideWindowSkipReason(windowMatch),
		},
		{
			Why:               "GetBuildLog is the fastest route to the failing compiler or build messages.",
			ToolName:          "GetBuildLog",
			ArgumentsTemplate: map[string]any{"tabIdentifier": tabIdentifier, "severity": "error"},
			WhenToSkip:        "Skip only if you already know the exact failing file or line from somewhere else.",
		},
		{
			Why:               "XcodeListNavigatorIssues is a good secondary view when the problem is already visible in Xcode's issue navigator.",
			ToolName:          "XcodeListNavigatorIssues",
			ArgumentsTemplate: map[string]any{"tabIdentifier": tabIdentifier},
			WhenToSkip:        "Skip unless you want the issue navigator perspective in addition to the build log.",
		},
		{
			Why:               "Open the failing file after the log points you at a concrete path.",
			ToolName:          "XcodeRead",
			ArgumentsTemplate: map[string]any{"tabIdentifier": tabIdentifier, "filePath": "<file path from the log or issue navigator>"},
			WhenToSkip:        "Skip only if the log already tells you everything you need.",
		},
	}

	nextCommands := buildGuideDiagnoseCommands(tabIdentifier, windowMatch)
	fallbacks := []guideWorkflowFallback{
		{
			Title:       "If you need issue navigator context",
			Description: "Inspect the issue navigator tool schema before composing a filtered request.",
			Commands: []string{
				`xcodemcp tool inspect XcodeListNavigatorIssues --json`,
			},
		},
		{
			Title:       "If the problem is obviously file-specific",
			Description: "Jump straight from the build log to XcodeRead for the failing file.",
			Commands: []string{
				formatXcodeReadCommand(tabIdentifier, "<file path from the log>"),
			},
		},
	}

	return guideWorkflowResult{
		ID:        "diagnose",
		Title:     guideWorkflowTitles["diagnose"],
		Reason:    guideReasonForIntent(intent, windowMatch, "The request is about errors or failure analysis, so the efficient path is window resolution -> diagnostics -> open the failing file."),
		Steps:     steps,
		Fallbacks: fallbacks,
	}, nextCommands
}

func formatAgentGuide(report agentGuideReport, windowMatch guideWindowMatch) string {
	var b strings.Builder
	b.WriteString("xcodemcp agent guide\n\n")

	b.WriteString("Intent\n")
	b.WriteString("------\n")
	rawIntent := report.Intent.Raw
	if rawIntent == "" {
		rawIntent = "(none)"
	}
	fmt.Fprintf(&b, "request: %s\n", rawIntent)
	fmt.Fprintf(&b, "workflow: %s (confidence %.2f)\n", report.Intent.WorkflowID, report.Intent.Confidence)
	if len(report.Intent.Alternatives) > 0 {
		fmt.Fprintf(&b, "alternatives: %s\n", strings.Join(report.Intent.Alternatives, ", "))
	}

	b.WriteString("\nEnvironment\n")
	b.WriteString("-----------\n")
	summary := report.Environment.Doctor.Summary
	fmt.Fprintf(&b, "doctor: %t (%d ok, %d warn, %d fail, %d info)\n", report.Environment.Doctor.Success, summary.OK, summary.Warn, summary.Fail, summary.Info)
	fmt.Fprintf(&b, "tool catalog: %d tools\n", report.Environment.ToolCatalog.Count)
	if report.Environment.AgentStatus != nil {
		fmt.Fprintf(&b, "launchagent: running=%t socketReachable=%t backendSessions=%d\n", report.Environment.AgentStatus.Running, report.Environment.AgentStatus.SocketReachable, report.Environment.AgentStatus.BackendSessions)
	}
	if report.Environment.Windows.Attempted {
		fmt.Fprintf(&b, "windows: %d discovered\n", len(report.Environment.Windows.Entries))
	} else {
		b.WriteString("windows: not collected\n")
	}
	if windowMatch.Note != "" {
		fmt.Fprintf(&b, "window match: %s\n", windowMatch.Note)
	}
	if len(report.Errors) > 0 {
		b.WriteString("notes:\n")
		for _, item := range report.Errors {
			fmt.Fprintf(&b, "- %s: %s\n", item.Step, item.Message)
		}
	}

	b.WriteString("\nRecommended Workflow\n")
	b.WriteString("--------------------\n")
	fmt.Fprintf(&b, "%s — %s\n", report.Workflow.Title, report.Workflow.Reason)
	if report.Workflow.ID == guideWorkflowCatalog {
		for _, step := range report.Workflow.Steps {
			fmt.Fprintf(&b, "- %s: %s\n", step.ToolName, step.Why)
		}
	} else {
		for index, step := range report.Workflow.Steps {
			fmt.Fprintf(&b, "%d. %s\n", index+1, step.ToolName)
			fmt.Fprintf(&b, "   why: %s\n", step.Why)
			fmt.Fprintf(&b, "   args: %s\n", formatArgumentsTemplate(step.ArgumentsTemplate))
			fmt.Fprintf(&b, "   skip: %s\n", step.WhenToSkip)
		}
	}

	b.WriteString("\nExact Next Commands\n")
	b.WriteString("-------------------\n")
	for _, command := range report.NextCommands {
		fmt.Fprintf(&b, "- %s\n", command)
	}

	b.WriteString("\nFallbacks\n")
	b.WriteString("---------\n")
	for _, fallback := range report.Workflow.Fallbacks {
		fmt.Fprintf(&b, "- %s: %s\n", fallback.Title, fallback.Description)
		for _, command := range fallback.Commands {
			fmt.Fprintf(&b, "  %s\n", command)
		}
	}

	return b.String()
}

func formatArgumentsTemplate(arguments map[string]any) string {
	if len(arguments) == 0 {
		return "{}"
	}
	data, err := json.Marshal(arguments)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func guideReasonForIntent(intent guideIntentMatch, windowMatch guideWindowMatch, base string) string {
	if windowMatch.MatchedEntry != nil {
		return fmt.Sprintf("%s Live window matching already suggests %s.", base, windowMatch.MatchedEntry.TabIdentifier)
	}
	if windowMatch.Ambiguous {
		return fmt.Sprintf("%s The window match is ambiguous, so keep tabIdentifier as a placeholder until you pick one.", base)
	}
	if windowMatch.Note != "" {
		return fmt.Sprintf("%s %s", base, windowMatch.Note)
	}
	return base
}

func guideWindowSkipReason(windowMatch guideWindowMatch) string {
	if windowMatch.MatchedEntry != nil {
		return fmt.Sprintf("Skip because agent guide already matched %s from the live Xcode window list.", windowMatch.MatchedEntry.TabIdentifier)
	}
	return "Skip only if you already know the exact tabIdentifier you want to use."
}

func buildGuideBuildCommands(tabIdentifier string, windowMatch guideWindowMatch) []string {
	commands := []string{}
	if windowMatch.MatchedEntry == nil {
		commands = append(commands, `xcodemcp tool call XcodeListWindows --json '{}'`)
	}
	commands = append(commands,
		formatBuildProjectCommand(tabIdentifier),
		formatGetBuildLogCommand(tabIdentifier, "error"),
	)
	return commands
}

func buildGuideTestCommands(tabIdentifier string, windowMatch guideWindowMatch) []string {
	commands := []string{}
	if windowMatch.MatchedEntry == nil {
		commands = append(commands, `xcodemcp tool call XcodeListWindows --json '{}'`)
	}
	commands = append(commands,
		formatRunAllTestsCommand(tabIdentifier),
		formatGetBuildLogCommand(tabIdentifier, "error"),
	)
	return commands
}

func buildGuideReadCommands(tabIdentifier, subject string, windowMatch guideWindowMatch) []string {
	commands := []string{}
	if windowMatch.MatchedEntry == nil {
		commands = append(commands, `xcodemcp tool call XcodeListWindows --json '{}'`)
	}
	if looksLikeFileHint(subject) {
		commands = append(commands,
			formatXcodeGlobCommand(tabIdentifier, guideGlobPattern(subject)),
			formatXcodeReadCommand(tabIdentifier, "<path from XcodeGlob>"),
		)
		return commands
	}
	commands = append(commands,
		formatXcodeLSCommand(tabIdentifier, ""),
		formatXcodeReadCommand(tabIdentifier, "<path from XcodeLS>"),
	)
	return commands
}

func buildGuideSearchCommands(tabIdentifier, subject string, windowMatch guideWindowMatch) []string {
	commands := []string{}
	if windowMatch.MatchedEntry == nil {
		commands = append(commands, `xcodemcp tool call XcodeListWindows --json '{}'`)
	}
	if looksLikeFileHint(subject) {
		commands = append(commands, formatXcodeGlobCommand(tabIdentifier, guideGlobPattern(subject)))
		return commands
	}
	commands = append(commands, formatXcodeGrepCommand(tabIdentifier, guideSearchPattern(subject)))
	return commands
}

func buildGuideEditCommands(tabIdentifier, subject string, windowMatch guideWindowMatch) []string {
	commands := []string{}
	if windowMatch.MatchedEntry == nil {
		commands = append(commands, `xcodemcp tool call XcodeListWindows --json '{}'`)
	}
	pathPlaceholder := "<path from XcodeLS>"
	if looksLikeFileHint(subject) {
		commands = append(commands, formatXcodeGlobCommand(tabIdentifier, guideGlobPattern(subject)))
		pathPlaceholder = "<path from XcodeGlob>"
	} else {
		commands = append(commands, formatXcodeLSCommand(tabIdentifier, ""))
	}
	commands = append(commands,
		formatXcodeReadCommand(tabIdentifier, pathPlaceholder),
		formatXcodeUpdateTemplate(tabIdentifier, pathPlaceholder),
		formatRefreshIssuesCommand(tabIdentifier, pathPlaceholder),
	)
	return commands
}

func buildGuideDiagnoseCommands(tabIdentifier string, windowMatch guideWindowMatch) []string {
	commands := []string{}
	if windowMatch.MatchedEntry == nil {
		commands = append(commands, `xcodemcp tool call XcodeListWindows --json '{}'`)
	}
	commands = append(commands,
		formatGetBuildLogCommand(tabIdentifier, "error"),
		formatXcodeReadCommand(tabIdentifier, "<file path from the log or issue navigator>"),
	)
	return commands
}

func guideWorkflowToolChain(workflowID string) []string {
	switch workflowID {
	case "build":
		return []string{"XcodeListWindows", "BuildProject", "GetBuildLog"}
	case "test":
		return []string{"XcodeListWindows", "RunAllTests", "GetTestList/RunSomeTests", "GetBuildLog"}
	case "read":
		return []string{"XcodeListWindows", "XcodeGlob/XcodeLS", "XcodeRead"}
	case "search":
		return []string{"XcodeListWindows", "XcodeGrep/XcodeGlob"}
	case "edit":
		return []string{"XcodeListWindows", "XcodeGlob/XcodeLS", "XcodeRead", "XcodeUpdate/XcodeWrite", "XcodeRefreshCodeIssuesInFile"}
	case "diagnose":
		return []string{"XcodeListWindows", "GetBuildLog/XcodeListNavigatorIssues", "XcodeRead"}
	default:
		return []string{}
	}
}

func looksLikeFileHint(subject string) bool {
	trimmed := strings.TrimSpace(subject)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, ".") || strings.Contains(trimmed, "/")
}

func guideGlobPattern(subject string) string {
	trimmed := strings.TrimSpace(subject)
	if trimmed == "" {
		return "**/*"
	}
	if strings.Contains(trimmed, "*") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "**/") {
		return trimmed
	}
	if strings.Contains(trimmed, "/") {
		return trimmed
	}
	if strings.Contains(trimmed, ".") {
		return "**/" + trimmed
	}
	return "**/*" + trimmed + "*"
}

func guideSearchPattern(subject string) string {
	trimmed := strings.TrimSpace(subject)
	if trimmed == "" {
		return "<search pattern>"
	}
	return trimmed
}

func formatBuildProjectCommand(tabIdentifier string) string {
	return fmt.Sprintf(`xcodemcp tool call BuildProject --timeout 300s --json '{"tabIdentifier":%s}'`, jsonQuote(tabIdentifier))
}

func formatGetBuildLogCommand(tabIdentifier, severity string) string {
	return fmt.Sprintf(`xcodemcp tool call GetBuildLog --timeout 60s --json '{"tabIdentifier":%s,"severity":%s}'`, jsonQuote(tabIdentifier), jsonQuote(severity))
}

func formatRunAllTestsCommand(tabIdentifier string) string {
	return fmt.Sprintf(`xcodemcp tool call RunAllTests --timeout 300s --json '{"tabIdentifier":%s}'`, jsonQuote(tabIdentifier))
}

func formatGetTestListCommand(tabIdentifier string) string {
	return fmt.Sprintf(`xcodemcp tool call GetTestList --timeout 60s --json '{"tabIdentifier":%s}'`, jsonQuote(tabIdentifier))
}

func formatRunSomeTestsTemplate(tabIdentifier string) string {
	return fmt.Sprintf(`xcodemcp tool call RunSomeTests --timeout 300s --json '{"tabIdentifier":%s,"tests":[{"targetName":"<targetName>","testIdentifier":"<identifier>"}]}'`, jsonQuote(tabIdentifier))
}

func formatXcodeLSCommand(tabIdentifier, path string) string {
	return fmt.Sprintf(`xcodemcp tool call XcodeLS --timeout 60s --json '{"tabIdentifier":%s,"path":%s}'`, jsonQuote(tabIdentifier), jsonQuote(path))
}

func formatXcodeGlobCommand(tabIdentifier, pattern string) string {
	return fmt.Sprintf(`xcodemcp tool call XcodeGlob --timeout 60s --json '{"tabIdentifier":%s,"pattern":%s}'`, jsonQuote(tabIdentifier), jsonQuote(pattern))
}

func formatXcodeReadCommand(tabIdentifier, filePath string) string {
	return fmt.Sprintf(`xcodemcp tool call XcodeRead --timeout 60s --json '{"tabIdentifier":%s,"filePath":%s}'`, jsonQuote(tabIdentifier), jsonQuote(filePath))
}

func formatXcodeGrepCommand(tabIdentifier, pattern string) string {
	return fmt.Sprintf(`xcodemcp tool call XcodeGrep --timeout 60s --json '{"tabIdentifier":%s,"pattern":%s,"outputMode":"content","showLineNumbers":true}'`, jsonQuote(tabIdentifier), jsonQuote(pattern))
}

func formatXcodeUpdateTemplate(tabIdentifier, filePath string) string {
	return fmt.Sprintf(`xcodemcp tool call XcodeUpdate --timeout 60s --json '{"tabIdentifier":%s,"filePath":%s,"oldString":"<exact text to replace>","newString":"<replacement text>"}'`, jsonQuote(tabIdentifier), jsonQuote(filePath))
}

func formatRefreshIssuesCommand(tabIdentifier, filePath string) string {
	return fmt.Sprintf(`xcodemcp tool call XcodeRefreshCodeIssuesInFile --timeout 60s --json '{"tabIdentifier":%s,"filePath":%s}'`, jsonQuote(tabIdentifier), jsonQuote(filePath))
}

func formatXcodeWriteTemplate(tabIdentifier, filePath string) string {
	return fmt.Sprintf(`xcodemcp tool call XcodeWrite --timeout 60s --json '{"tabIdentifier":%s,"filePath":%s,"content":"<full file contents>"}'`, jsonQuote(tabIdentifier), jsonQuote(filePath))
}

func formatMaybeWindowsCommand(windowMatch guideWindowMatch) string {
	if windowMatch.MatchedEntry != nil {
		return fmt.Sprintf("# already matched %s", windowMatch.MatchedEntry.TabIdentifier)
	}
	return `xcodemcp tool call XcodeListWindows --json '{}'`
}

func jsonQuote(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
