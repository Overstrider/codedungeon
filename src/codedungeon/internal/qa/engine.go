package qa

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	artifactreg "github.com/loldinis/codedungeon/internal/artifacts"
	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/tooladapter"
)

const defaultTimeoutSeconds = 600

func Run(ctx context.Context, req Request) (Result, error) {
	if err := normalizeRequest(&req); err != nil {
		return Result{}, err
	}
	started := time.Now()
	sessionID := newSessionID()
	result := Result{
		SessionID:   sessionID,
		Status:      StatusPass,
		StartedAt:   started,
		EvidenceDir: evidenceDir(req, sessionID),
	}
	if err := makeEvidenceDirs(result.EvidenceDir); err != nil {
		return Result{}, err
	}
	if req.Store != nil && req.RunID > 0 && req.Fresh {
		if err := req.Store.SupersedeVerificationRecords(req.RunID, req.Phase); err != nil {
			return Result{}, err
		}
	}
	if err := writeJSON(filepath.Join(result.EvidenceDir, "request.json"), requestForArtifact(req)); err != nil {
		return Result{}, err
	}

	checks := buildChecks(req)
	result.Dependencies = preflight(req, checks)
	if blockedByRequiredDependency(result.Dependencies) {
		result.Status = StatusBlocked
		result.FinishedAt = time.Now()
		result.Findings = append(result.Findings, Finding{
			Severity: "blocking",
			Category: "dependency_missing",
			Title:    "Required QA dependency is missing",
			Detail:   missingDependencySummary(result.Dependencies),
		})
		return finalizeResult(req, result)
	}
	if len(checks) == 0 {
		result.Status = StatusBlocked
		result.FinishedAt = time.Now()
		result.Findings = append(result.Findings, Finding{
			Severity: "blocking",
			Category: "no_checks",
			Title:    "No executable QA checks were found",
			Detail:   "Pass --cmd, --plan, or use --auto in a project with a recognized test framework.",
		})
		return finalizeResult(req, result)
	}
	if req.PreflightOnly {
		result.Status = StatusPass
		result.FinishedAt = time.Now()
		return finalizeResult(req, result)
	}

	for _, check := range checks {
		checkResult := runCheck(ctx, req, result.EvidenceDir, check)
		result.Checks = append(result.Checks, checkResult)
		if checkResult.Status != StatusPass {
			result.Status = StatusFail
			result.Findings = append(result.Findings, findingForCheck(checkResult))
			break
		}
	}
	if result.Status == "" {
		result.Status = StatusPass
	}
	result.FinishedAt = time.Now()
	return finalizeResult(req, result)
}

func normalizeRequest(req *Request) error {
	if strings.TrimSpace(req.Root) == "" {
		cwd, _ := os.Getwd()
		req.Root = cwd
	}
	abs, err := filepath.Abs(req.Root)
	if err != nil {
		return err
	}
	req.Root = abs
	if req.Entrypoint == "" {
		req.Entrypoint = EntrypointStandalone
	}
	if req.Mode == "" {
		req.Mode = ModeAuto
	}
	if req.Phase == "" {
		req.Phase = "6"
	}
	if req.DependencyMode == "" {
		req.DependencyMode = DependencyStrict
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = defaultTimeoutSeconds
	}
	return nil
}

func evidenceDir(req Request, sessionID string) string {
	if strings.TrimSpace(req.OutputDir) != "" {
		return req.OutputDir
	}
	return filepath.Join(req.Root, ".codedungeon", "qa", "sessions", sessionID)
}

func makeEvidenceDirs(root string) error {
	for _, dir := range []string{"", "checks", "logs", "playwright", "fix-tasks", "api"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func buildChecks(req Request) []CommandSpec {
	if len(req.Commands) > 0 {
		return normalizeCommands(req.Commands)
	}
	switch req.Mode {
	case ModeE2E:
		if checks := detectedPlaywrightChecks(req); len(checks) > 0 {
			return normalizeCommands(checks)
		}
		return []CommandSpec{{
			ID:             "playwright",
			Kind:           CheckPlaywright,
			Name:           "Playwright E2E",
			Command:        "npx playwright test --reporter=json",
			CWD:            ".",
			Required:       true,
			TimeoutSeconds: 900,
		}}
	case ModeAuto, ModeVerify, ModeUnit, ModeIntegration, ModeFull:
		framework := DetectFramework(req.Root)
		var out []CommandSpec
		for _, command := range framework.RunCommands {
			out = append(out, commandSpecFromDetected(command, req.TimeoutSeconds))
		}
		return normalizeCommands(out)
	default:
		return nil
	}
}

func detectedPlaywrightChecks(req Request) []CommandSpec {
	framework := DetectFramework(req.Root)
	var out []CommandSpec
	for _, command := range framework.RunCommands {
		check := commandSpecFromDetected(command, req.TimeoutSeconds)
		if check.Kind != CheckPlaywright {
			continue
		}
		check.Name = "Playwright E2E"
		check.Required = true
		check.TimeoutSeconds = 900
		out = append(out, check)
	}
	return out
}

func normalizeCommands(commands []CommandSpec) []CommandSpec {
	out := make([]CommandSpec, 0, len(commands))
	for i, command := range commands {
		if command.Kind == "" {
			command.Kind = CheckCommand
		}
		if strings.TrimSpace(command.ID) == "" {
			command.ID = fmt.Sprintf("check-%02d", i+1)
		}
		if strings.TrimSpace(command.Name) == "" {
			command.Name = string(command.Kind)
		}
		if strings.TrimSpace(command.CWD) == "" {
			command.CWD = "."
		}
		if command.TimeoutSeconds <= 0 {
			command.TimeoutSeconds = defaultTimeoutSeconds
		}
		out = append(out, command)
	}
	return out
}

func preflight(req Request, checks []CommandSpec) []DependencyResult {
	var deps []DependencyResult
	for _, check := range checks {
		if check.Kind != CheckPlaywright {
			continue
		}
		required := check.Required || req.Mode == ModeE2E || req.Mode == ModeFull
		deps = append(deps, detectPlaywright(safeCWD(req.Root, check.CWD), required))
	}
	return deps
}

func commandSpecFromDetected(command string, timeoutSeconds int) CommandSpec {
	cwd := "."
	cmd := strings.TrimSpace(command)
	if strings.HasPrefix(cmd, "cd ") && strings.Contains(cmd, " && ") {
		parts := strings.SplitN(cmd, " && ", 2)
		cwd = strings.TrimSpace(strings.TrimPrefix(parts[0], "cd "))
		cmd = strings.TrimSpace(parts[1])
	}
	kind := CheckCommand
	if strings.Contains(strings.ToLower(cmd), "playwright") {
		kind = CheckPlaywright
		cmd = ensurePlaywrightJSONReporter(cmd)
	}
	return CommandSpec{
		ID:             commandID(command),
		Kind:           kind,
		Name:           "Framework verification",
		Command:        cmd,
		CWD:            cwd,
		Required:       true,
		TimeoutSeconds: timeoutSeconds,
	}
}

func ensurePlaywrightJSONReporter(command string) string {
	if strings.Contains(strings.ToLower(command), "--reporter") {
		return command
	}
	return command + " --reporter=json"
}

func detectPlaywright(root string, required bool) DependencyResult {
	dep := DependencyResult{
		Name:        "playwright",
		Required:    required,
		Status:      DependencyMissing,
		InstallHint: "npm i -D @playwright/test && npx playwright install --with-deps",
	}
	if !hasPlaywrightManifest(root) {
		dep.Detail = "no package.json playwright dependency or playwright.config.* found"
		return dep
	}
	stdout, stderr, err := runExec(context.Background(), root, 10*time.Second, "npx", "playwright", "--version")
	if err != nil {
		dep.Detail = strings.TrimSpace(firstNonEmpty(stderr, err.Error()))
		return dep
	}
	dep.Status = DependencyPresent
	dep.Version = strings.TrimSpace(stdout)
	return dep
}

func hasPlaywrightManifest(root string) bool {
	for _, name := range []string{"playwright.config.ts", "playwright.config.js", "playwright.config.mjs", "playwright.config.cjs"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	body, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return false
	}
	raw := string(body)
	return strings.Contains(raw, `"@playwright/test"`) || strings.Contains(raw, `"playwright"`)
}

func blockedByRequiredDependency(deps []DependencyResult) bool {
	for _, dep := range deps {
		if dep.Required && dep.Status != DependencyPresent {
			return true
		}
	}
	return false
}

func runCheck(ctx context.Context, req Request, evidence string, check CommandSpec) CheckResult {
	switch check.Kind {
	case CheckPlaywright:
		return runCommandCheck(ctx, req, evidence, check)
	default:
		return runCommandCheck(ctx, req, evidence, check)
	}
}

func runCommandCheck(ctx context.Context, req Request, evidence string, check CommandSpec) CheckResult {
	started := time.Now()
	cwd := safeCWD(req.Root, check.CWD)
	timeout := time.Duration(firstPositive(check.TimeoutSeconds, req.TimeoutSeconds, defaultTimeoutSeconds)) * time.Second
	stdout, stderr, err := runShell(ctx, cwd, timeout, commandWithPlaywrightEnv(check, evidence))
	finished := time.Now()
	status := StatusPass
	exitCode := 0
	errText := ""
	if err != nil {
		status = StatusFail
		exitCode = exitCodeFromError(err)
		errText = err.Error()
	}
	logPath := filepath.Join(evidence, "logs", check.ID+".log")
	body := fmt.Sprintf("$ %s\n\n[stdout]\n%s\n\n[stderr]\n%s\n", check.Command, stdout, stderr)
	if err != nil {
		body += fmt.Sprintf("\n[error]\n%v\n", err)
	}
	_ = os.WriteFile(logPath, []byte(body), 0o644)
	out := CheckResult{
		ID:         check.ID,
		Kind:       check.Kind,
		Name:       check.Name,
		Command:    check.Command,
		CWD:        check.CWD,
		Status:     status,
		ExitCode:   exitCode,
		DurationMs: finished.Sub(started).Milliseconds(),
		LogPath:    logPath,
		Error:      errText,
		StartedAt:  started,
		FinishedAt: finished,
	}
	if check.Kind == CheckPlaywright {
		out.ReportPath = filepath.Join(evidence, "playwright", "results.json")
		out.Artifacts = collectExisting(filepath.Join(evidence, "playwright"), filepath.Join(cwd, "playwright-report"), filepath.Join(cwd, "test-results"))
	}
	_ = writeJSON(filepath.Join(evidence, "checks", check.ID+".json"), out)
	return out
}

func commandWithPlaywrightEnv(check CommandSpec, evidence string) string {
	if check.Kind != CheckPlaywright {
		return check.Command
	}
	resultPath := filepath.Join(evidence, "playwright", "results.json")
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("set PLAYWRIGHT_JSON_OUTPUT_FILE=%s&& %s", resultPath, check.Command)
	}
	return fmt.Sprintf("PLAYWRIGHT_JSON_OUTPUT_FILE=%q %s", resultPath, check.Command)
}

func safeCWD(root, rel string) string {
	if strings.TrimSpace(rel) == "" || rel == "." {
		return root
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(root, rel)
}

func runShell(ctx context.Context, dir string, timeout time.Duration, command string) (string, string, error) {
	result, err := tooladapter.NewSystemRunner().RunShell(ctx, dir, command, timeout)
	return result.Stdout, result.Stderr, err
}

func runExec(ctx context.Context, dir string, timeout time.Duration, name string, args ...string) (string, string, error) {
	result, err := tooladapter.NewSystemRunner().Run(ctx, tooladapter.Command{Dir: dir, Name: name, Args: args, Timeout: timeout})
	return result.Stdout, result.Stderr, err
}

func finalizeResult(req Request, result Result) (Result, error) {
	if result.FinishedAt.IsZero() {
		result.FinishedAt = time.Now()
	}
	if err := writeJSON(filepath.Join(result.EvidenceDir, "preflight.json"), map[string]any{
		"status":       result.Status,
		"dependencies": result.Dependencies,
	}); err != nil {
		return result, err
	}
	if err := writeJSON(filepath.Join(result.EvidenceDir, "findings.json"), result.Findings); err != nil {
		return result, err
	}
	summaryPath := filepath.Join(result.EvidenceDir, "summary.md")
	if err := os.WriteFile(summaryPath, []byte(renderSummary(result)), 0o644); err != nil {
		return result, err
	}
	result.SummaryPath = summaryPath
	result.ResultPath = filepath.Join(result.EvidenceDir, "result.json")
	if err := writeJSON(result.ResultPath, result); err != nil {
		return result, err
	}
	if req.Store != nil {
		if err := persistResult(req, result); err != nil {
			return result, err
		}
	}
	return result, nil
}

func persistResult(req Request, result Result) error {
	if err := req.Store.UpsertQASession(db.QASession{
		ID:             result.SessionID,
		RunID:          req.RunID,
		ExecutionID:    req.ExecutionID,
		Entrypoint:     string(req.Entrypoint),
		Mode:           string(req.Mode),
		Status:         string(result.Status),
		Root:           req.Root,
		PlanPath:       req.PlanPath,
		EvidenceDir:    result.EvidenceDir,
		StartedAt:      result.StartedAt.Unix(),
		UpdatedAt:      time.Now().Unix(),
		FinishedAt:     result.FinishedAt.Unix(),
		FailureMessage: result.Error,
	}); err != nil {
		return err
	}
	for i, dep := range result.Dependencies {
		if err := req.Store.InsertQADependency(db.QADependency{
			ID:          fmt.Sprintf("%s-dep-%02d", result.SessionID, i+1),
			SessionID:   result.SessionID,
			Name:        dep.Name,
			Required:    dep.Required,
			Status:      string(dep.Status),
			Version:     dep.Version,
			InstallHint: dep.InstallHint,
			Detail:      dep.Detail,
		}); err != nil {
			return err
		}
	}
	for _, check := range result.Checks {
		if err := req.Store.InsertQACheck(db.QACheck{
			ID:         result.SessionID + "-" + check.ID,
			SessionID:  result.SessionID,
			Kind:       string(check.Kind),
			Name:       check.Name,
			Status:     string(check.Status),
			Command:    check.Command,
			CWD:        check.CWD,
			ExitCode:   check.ExitCode,
			DurationMs: check.DurationMs,
			LogPath:    check.LogPath,
			ReportPath: check.ReportPath,
			Artifacts:  check.Artifacts,
			StartedAt:  check.StartedAt.Unix(),
			FinishedAt: check.FinishedAt.Unix(),
		}); err != nil {
			return err
		}
		if req.RunID > 0 {
			if _, err := req.Store.InsertVerificationRecord(db.VerificationRecord{
				RunID:   req.RunID,
				Phase:   req.Phase,
				Command: check.Command,
				Status:  string(check.Status),
				LogPath: check.LogPath,
			}); err != nil {
				return err
			}
		}
	}
	for i, finding := range result.Findings {
		evidence := ""
		if len(finding.Evidence) > 0 {
			evidence = finding.Evidence[0]
		}
		if err := req.Store.InsertQAFinding(db.QAFinding{
			ID:           fmt.Sprintf("%s-finding-%02d", result.SessionID, i+1),
			SessionID:    result.SessionID,
			Severity:     finding.Severity,
			Category:     finding.Category,
			Title:        finding.Title,
			Detail:       finding.Detail,
			EvidencePath: evidence,
			FixTaskPath:  finding.FixTaskPath,
		}); err != nil {
			return err
		}
	}
	return persistArtifacts(req, result)
}

func persistArtifacts(req Request, result Result) error {
	registry := artifactreg.NewRegistry(req.Store, req.Root)
	meta := map[string]any{
		"status":     result.Status,
		"mode":       req.Mode,
		"entrypoint": req.Entrypoint,
	}
	for _, item := range []struct {
		role string
		kind string
		path string
	}{
		{"directory", "directory", result.EvidenceDir},
		{"request", "json", filepath.Join(result.EvidenceDir, "request.json")},
		{"preflight", "json", filepath.Join(result.EvidenceDir, "preflight.json")},
		{"findings", "json", filepath.Join(result.EvidenceDir, "findings.json")},
		{"summary", "markdown", result.SummaryPath},
		{"result", "json", result.ResultPath},
		{"plan", artifactreg.KindForPath(req.PlanPath), req.PlanPath},
	} {
		if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
			RunID: req.RunID, Module: "qa", OwnerType: "qa_session", OwnerID: result.SessionID,
			Phase: req.Phase, Role: item.role, Kind: item.kind, Path: item.path, Metadata: meta,
		}); err != nil {
			return err
		}
	}
	for _, check := range result.Checks {
		ownerID := result.SessionID + "-" + check.ID
		checkMeta := map[string]any{
			"session_id": result.SessionID,
			"status":     check.Status,
			"name":       check.Name,
			"kind":       check.Kind,
		}
		for _, item := range []struct {
			role string
			path string
		}{
			{"check", filepath.Join(result.EvidenceDir, "checks", check.ID+".json")},
			{"log", check.LogPath},
			{"report", check.ReportPath},
		} {
			if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
				RunID: req.RunID, Module: "qa", OwnerType: "qa_check", OwnerID: ownerID,
				Phase: req.Phase, Role: item.role, Kind: artifactreg.KindForPath(item.path), Path: item.path, Metadata: checkMeta,
			}); err != nil {
				return err
			}
		}
		for _, path := range check.Artifacts {
			if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
				RunID: req.RunID, Module: "qa", OwnerType: "qa_check", OwnerID: ownerID,
				Phase: req.Phase, Role: "artifact", Kind: artifactreg.KindForPath(path), Path: path, Metadata: checkMeta,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderSummary(result Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# QA Session %s\n\n", result.SessionID)
	fmt.Fprintf(&b, "Status: %s\n\n", result.Status)
	if len(result.Dependencies) > 0 {
		b.WriteString("## Dependencies\n\n")
		for _, dep := range result.Dependencies {
			fmt.Fprintf(&b, "- %s: %s", dep.Name, dep.Status)
			if dep.Required {
				b.WriteString(" (required)")
			}
			if dep.InstallHint != "" && dep.Status != DependencyPresent {
				fmt.Fprintf(&b, " - %s", dep.InstallHint)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(result.Checks) > 0 {
		b.WriteString("## Checks\n\n")
		for _, check := range result.Checks {
			fmt.Fprintf(&b, "- %s: %s\n", check.ID, check.Status)
		}
	}
	return b.String()
}

func findingForCheck(check CheckResult) Finding {
	category := "command_failed"
	if strings.Contains(strings.ToLower(check.Error), "timed out") {
		category = "timeout"
	}
	title := "QA check failed: " + check.ID
	if check.Name != "" {
		title = "QA check failed: " + check.Name
	}
	return Finding{
		Severity: "blocking",
		Category: category,
		Title:    title,
		Detail:   check.Error,
		Evidence: nonEmptyList(check.LogPath),
	}
}

func missingDependencySummary(deps []DependencyResult) string {
	var parts []string
	for _, dep := range deps {
		if dep.Required && dep.Status != DependencyPresent {
			parts = append(parts, dep.Name+": "+dep.InstallHint)
		}
	}
	return strings.Join(parts, "; ")
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

func requestForArtifact(req Request) Request {
	req.Store = nil
	return req
}

func commandID(command string) string {
	sum := sha256.Sum256([]byte(command))
	return "cmd-" + hex.EncodeToString(sum[:])[:10]
}

func newSessionID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "qa-" + time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b[:])
	}
	return "qa-" + time.Now().UTC().Format("20060102-150405") + "-fallback"
}

func exitCodeFromError(err error) int {
	var toolErr tooladapter.ToolError
	if tooladapter.AsToolError(err, &toolErr) && toolErr.ExitCode > 0 {
		return toolErr.ExitCode
	}
	return 1
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func collectExisting(paths ...string) []string {
	var out []string
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			out = append(out, path)
		}
	}
	return out
}
