package codereview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/loldinis/codedungeon/internal/reviewpipe"
)

type Runner interface {
	RunPersona(ctx context.Context, req Request, persona string, outPath string) error
	RunAdjudicator(ctx context.Context, req Request, personas []PersonaReview, outPath string) error
}

func Execute(ctx context.Context, req Request, runner Runner) (Result, error) {
	if runner == nil {
		return Result{}, fmt.Errorf("code review runner is required")
	}
	if strings.TrimSpace(req.OutputDir) == "" {
		return Result{}, fmt.Errorf("output_dir is required")
	}
	personas := normalizePersonas(req.Personas)
	if len(personas) == 0 {
		return Result{}, fmt.Errorf("at least one review persona is required")
	}
	if err := validateRequest(req); err != nil {
		return Result{}, err
	}
	attempt := newReviewAttempt(req.OutputDir)
	if err := clearPublishedReviewArtifacts(req.OutputDir); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(attempt.PersonaDir, 0o755); err != nil {
		return Result{}, err
	}
	if err := writeAttemptManifest(req.OutputDir, attempt, "RUNNING"); err != nil {
		return Result{}, err
	}
	for _, path := range []string{filepath.Join(req.OutputDir, "review-request.json"), attempt.RequestPath} {
		if err := writeJSON(path, req); err != nil {
			return Result{}, err
		}
	}

	reviews := make([]PersonaReview, 0, len(personas))
	for _, persona := range personas {
		outPath := filepath.Join(attempt.PersonaDir, persona+".json")
		if err := removeStaleArtifact(outPath); err != nil {
			return Result{}, err
		}
		if err := runner.RunPersona(ctx, req, persona, outPath); err != nil {
			if writeErr := writeReviewFailure(req.OutputDir, attempt, persona, err); writeErr != nil {
				return Result{}, fmt.Errorf("persona %s failed: %w; write review failure: %v", persona, err, writeErr)
			}
			return Result{}, fmt.Errorf("persona %s failed: %w", persona, err)
		}
		review, err := readPersonaReview(outPath)
		if err != nil {
			if writeErr := writeReviewFailure(req.OutputDir, attempt, persona, err); writeErr != nil {
				return Result{}, fmt.Errorf("%w; write review failure: %v", err, writeErr)
			}
			return Result{}, err
		}
		if err := ValidatePersonaReview(review, req.ProjectRules); err != nil {
			if writeErr := writeReviewFailure(req.OutputDir, attempt, persona, err); writeErr != nil {
				return Result{}, fmt.Errorf("%w; write review failure: %v", err, writeErr)
			}
			return Result{}, err
		}
		reviews = append(reviews, review)
	}

	decisionPath := filepath.Join(attempt.Dir, "review-decision.json")
	if err := removeStaleArtifact(decisionPath); err != nil {
		return Result{}, err
	}
	if err := runner.RunAdjudicator(ctx, req, reviews, decisionPath); err != nil {
		if writeErr := writeReviewFailure(req.OutputDir, attempt, "adjudicator", err); writeErr != nil {
			return Result{}, fmt.Errorf("review-decision failed: %w; write review failure: %v", err, writeErr)
		}
		return Result{}, fmt.Errorf("review-decision failed: %w", err)
	}
	decision, err := readDecision(decisionPath)
	if err != nil {
		if writeErr := writeReviewFailure(req.OutputDir, attempt, "adjudicator", err); writeErr != nil {
			return Result{}, fmt.Errorf("%w; write review failure: %v", err, writeErr)
		}
		return Result{}, err
	}
	findings := collectFindings(reviews)
	if err := ValidateDecision(decision, reviews, findings); err != nil {
		if writeErr := writeReviewFailure(req.OutputDir, attempt, "adjudicator", err); writeErr != nil {
			return Result{}, fmt.Errorf("%w; write review failure: %v", err, writeErr)
		}
		return Result{}, err
	}

	result := Result{
		URL:               req.URL,
		Verdict:           decision.Verdict,
		AttemptID:         attempt.ID,
		AttemptDir:        attempt.Dir,
		PersonaDir:        attempt.PersonaDir,
		ProjectRules:      req.ProjectRules,
		Personas:          reviews,
		Decision:          decision,
		Findings:          findings,
		Integrity:         IntegrityReport{Status: IntegrityPass},
		ReviewMDPath:      filepath.Join(req.OutputDir, "review.md"),
		ReviewJSONPath:    filepath.Join(req.OutputDir, "review.json"),
		ResultJSONPath:    filepath.Join(req.OutputDir, "review-result.json"),
		ReviewSummaryPath: filepath.Join(req.OutputDir, "review-summary.json"),
		DecisionJSONPath:  decisionPath,
	}
	summary, err := BuildReviewSummary(result)
	if err != nil {
		return Result{}, err
	}
	result.Summary = summary
	markdown := RenderSummaryMarkdown(summary)
	for _, path := range []string{filepath.Join(attempt.Dir, "review.md"), result.ReviewMDPath} {
		if err := os.WriteFile(path, []byte(markdown), 0o644); err != nil {
			return Result{}, err
		}
	}
	for _, path := range []string{filepath.Join(attempt.Dir, "review-summary.json"), result.ReviewSummaryPath} {
		if err := writeJSON(path, summary); err != nil {
			return Result{}, err
		}
	}
	for _, path := range []string{filepath.Join(attempt.Dir, "review-result.json"), result.ResultJSONPath} {
		if err := writeJSON(path, result); err != nil {
			return Result{}, err
		}
	}
	for _, path := range []string{filepath.Join(attempt.Dir, "review.json"), result.ReviewJSONPath} {
		if err := writeJSON(path, result); err != nil {
			return Result{}, err
		}
	}
	if err := writeAttemptManifest(req.OutputDir, attempt, "COMPLETED"); err != nil {
		return Result{}, err
	}
	return result, nil
}

func ValidateResult(result Result) error {
	if result.Integrity.Status != IntegrityPass {
		return fmt.Errorf("review integrity is %s", result.Integrity.Status)
	}
	if len(result.Personas) == 0 {
		return fmt.Errorf("review result has no persona reviews")
	}
	if err := validateProjectRules(result.ProjectRules); err != nil {
		return err
	}
	for _, persona := range result.Personas {
		if err := ValidatePersonaReview(persona, result.ProjectRules); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(result.Findings, collectFindings(result.Personas)) {
		return fmt.Errorf("review result findings do not match persona findings")
	}
	if err := ValidateDecision(result.Decision, result.Personas, result.Findings); err != nil {
		return err
	}
	return nil
}

func ValidateResultDir(dir string) (Result, error) {
	path := filepath.Join(dir, "review-result.json")
	body, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("review-result.json not readable: %w", err)
	}
	var result Result
	if err := json.Unmarshal(body, &result); err != nil {
		return Result{}, fmt.Errorf("review-result.json invalid: %w", err)
	}
	if err := ValidateResult(result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func ValidatePersonaReview(review PersonaReview, rules ProjectRulesEnvelope) error {
	persona := strings.TrimSpace(review.Persona)
	if persona == "" {
		return fmt.Errorf("persona review missing persona")
	}
	if review.Verdict != VerdictApproved && review.Verdict != VerdictChangesRequested {
		return fmt.Errorf("persona %s invalid verdict %q", persona, review.Verdict)
	}
	if strings.TrimSpace(review.Model) == "" || strings.TrimSpace(review.Provider) == "" || strings.TrimSpace(review.SessionID) == "" {
		return fmt.Errorf("persona %s missing model/provider/session_id", persona)
	}
	if review.ReviewedFiles <= 0 {
		return fmt.Errorf("persona %s reviewed_files must be > 0", persona)
	}
	if len(nonEmptyStrings(review.ReviewedScope)) == 0 {
		return fmt.Errorf("persona %s reviewed_scope is required", persona)
	}
	if !sameRules(review.ProjectRules, rules) {
		return fmt.Errorf("persona %s project rules envelope does not match request", persona)
	}
	if len(review.Findings) == 0 {
		if review.Verdict != VerdictApproved {
			return fmt.Errorf("persona %s has no findings but verdict is %s", persona, review.Verdict)
		}
		if !substantive(review.ApprovalRationale) {
			return fmt.Errorf("persona %s approval_rationale is required and must be substantive", persona)
		}
		if len(nonEmptyStrings(review.RisksConsidered)) < 2 {
			return fmt.Errorf("persona %s risks_considered must include at least two concrete risks", persona)
		}
		return nil
	}
	if review.Verdict != VerdictChangesRequested {
		return fmt.Errorf("persona %s reported findings but verdict is %s", persona, review.Verdict)
	}
	for i, finding := range review.Findings {
		if err := validateFinding(finding); err != nil {
			return fmt.Errorf("persona %s finding %d: %w", persona, i+1, err)
		}
	}
	return nil
}

func ValidateDecision(decision Decision, personas []PersonaReview, findings []reviewpipe.Finding) error {
	if decision.Verdict != VerdictApproved && decision.Verdict != VerdictChangesRequested {
		return fmt.Errorf("review-decision invalid verdict %q", decision.Verdict)
	}
	if strings.TrimSpace(decision.DecidedBy) == "" || strings.TrimSpace(decision.Model) == "" || strings.TrimSpace(decision.Provider) == "" {
		return fmt.Errorf("review-decision missing decided_by/model/provider")
	}
	if len(decision.PersonaVerdicts) == 0 {
		return fmt.Errorf("review-decision missing persona_verdicts")
	}
	for _, persona := range personas {
		got := decision.PersonaVerdicts[persona.Persona]
		if got == "" {
			return fmt.Errorf("review-decision missing verdict for persona %s", persona.Persona)
		}
		if got != persona.Verdict {
			return fmt.Errorf("review-decision verdict for persona %s is %s, want %s", persona.Persona, got, persona.Verdict)
		}
	}
	if decision.Verdict == VerdictApproved {
		if !substantive(decision.ApprovalRationale) {
			return fmt.Errorf("review-decision approval_rationale is required and must be substantive")
		}
		if len(findings) > 0 {
			return fmt.Errorf("review-decision cannot approve with %d finding(s)", len(findings))
		}
		for _, persona := range personas {
			if persona.Verdict != VerdictApproved {
				return fmt.Errorf("review-decision cannot approve while persona %s is %s", persona.Persona, persona.Verdict)
			}
		}
	}
	return nil
}

func readPersonaReview(path string) (PersonaReview, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return PersonaReview{}, fmt.Errorf("persona review not readable %s: %w", path, err)
	}
	var review PersonaReview
	if err := json.Unmarshal(body, &review); err != nil {
		return PersonaReview{}, fmt.Errorf("persona review invalid %s: %w", path, err)
	}
	return review, nil
}

func readDecision(path string) (Decision, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Decision{}, fmt.Errorf("review-decision not readable: %w", err)
	}
	var decision Decision
	if err := json.Unmarshal(body, &decision); err != nil {
		return Decision{}, fmt.Errorf("review-decision invalid: %w", err)
	}
	if decision.CreatedAt == "" {
		decision.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return decision, nil
}

func validateRequest(req Request) error {
	if strings.TrimSpace(req.URL) == "" {
		return fmt.Errorf("url is required")
	}
	if !substantive(req.ProjectContext) {
		return fmt.Errorf("project_context is required and must be substantive")
	}
	if !substantive(req.TaskContext) {
		return fmt.Errorf("task_context is required and must be substantive")
	}
	return validateProjectRules(req.ProjectRules)
}

func validateProjectRules(rules ProjectRulesEnvelope) error {
	if strings.TrimSpace(rules.Status) == "" {
		return fmt.Errorf("PROJECT_RULES_STATUS is required")
	}
	if strings.TrimSpace(rules.Digest) == "" {
		return fmt.Errorf("PROJECT_RULES_DIGEST is required")
	}
	if strings.TrimSpace(rules.Read) != "yes" {
		return fmt.Errorf("PROJECT_RULES_READ must be yes")
	}
	return nil
}

func validateFinding(f reviewpipe.Finding) error {
	if strings.TrimSpace(f.Severity) == "" {
		return fmt.Errorf("severity is required")
	}
	if strings.TrimSpace(f.File) == "" {
		return fmt.Errorf("file is required")
	}
	if f.LineStart <= 0 || f.LineEnd <= 0 {
		return fmt.Errorf("line_start and line_end must be positive")
	}
	if f.LineEnd < f.LineStart {
		return fmt.Errorf("line_end must be >= line_start")
	}
	if strings.TrimSpace(f.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if !evidenceQuotePresent(f.EvidenceQuote) {
		return fmt.Errorf("evidence_quote is required and must be a concrete source or diff excerpt")
	}
	return nil
}

func collectFindings(personas []PersonaReview) []reviewpipe.Finding {
	var findings []reviewpipe.Finding
	for _, persona := range personas {
		for _, finding := range persona.Findings {
			if finding.Persona == "" {
				finding.Persona = persona.Persona
			}
			findings = append(findings, finding)
		}
	}
	return findings
}

func normalizePersonas(personas []string) []string {
	if len(personas) == 0 {
		personas = DefaultPersonas
	}
	seen := map[string]bool{}
	var out []string
	for _, persona := range personas {
		persona = strings.TrimSpace(persona)
		if persona == "" || seen[persona] {
			continue
		}
		seen[persona] = true
		out = append(out, persona)
	}
	sort.Strings(out)
	return out
}

func substantive(value string) bool {
	compact := strings.Join(strings.Fields(value), " ")
	return len(compact) >= 80
}

func evidenceQuotePresent(value string) bool {
	compact := strings.Join(strings.Fields(value), " ")
	if len(compact) < 4 {
		return false
	}
	lower := strings.ToLower(compact)
	for _, placeholder := range []string{
		"substantive source/diff evidence",
		"verbatim source or log excerpt",
		"exact verbatim quote",
		"exact quote",
		"source quote",
		"todo",
		"n/a",
		"none",
	} {
		if lower == placeholder {
			return false
		}
	}
	return true
}

func nonEmptyStrings(values []string) []string {
	var out []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func sameRules(a, b ProjectRulesEnvelope) bool {
	return strings.TrimSpace(a.Status) == strings.TrimSpace(b.Status) &&
		strings.TrimSpace(a.Digest) == strings.TrimSpace(b.Digest) &&
		strings.TrimSpace(a.Read) == strings.TrimSpace(b.Read)
}

func writeJSON(path string, value interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

type reviewAttempt struct {
	ID          string `json:"attempt_id"`
	Dir         string `json:"attempt_dir"`
	PersonaDir  string `json:"persona_dir"`
	RequestPath string `json:"request_path"`
	StartedAt   string `json:"started_at"`
}

func newReviewAttempt(outputDir string) reviewAttempt {
	id := time.Now().UTC().Format("20060102T150405.000000000Z")
	dir := filepath.Join(outputDir, "attempts", id)
	return reviewAttempt{
		ID:          id,
		Dir:         dir,
		PersonaDir:  filepath.Join(dir, "personas"),
		RequestPath: filepath.Join(dir, "review-request.json"),
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
	}
}

func clearPublishedReviewArtifacts(outputDir string) error {
	for _, name := range []string{
		"review.md",
		"review.json",
		"review-result.json",
		"review-summary.json",
		"review-decision.json",
		"review-failure.json",
	} {
		if err := removeStaleArtifact(filepath.Join(outputDir, name)); err != nil {
			return err
		}
	}
	return nil
}

func writeAttemptManifest(outputDir string, attempt reviewAttempt, status string) error {
	manifest := map[string]any{
		"attempt_id":   attempt.ID,
		"status":       status,
		"attempt_dir":  attempt.Dir,
		"persona_dir":  attempt.PersonaDir,
		"request_path": attempt.RequestPath,
		"started_at":   attempt.StartedAt,
		"updated_at":   time.Now().UTC().Format(time.RFC3339),
	}
	return writeJSON(filepath.Join(outputDir, "current-attempt.json"), manifest)
}

func writeReviewFailure(outputDir string, attempt reviewAttempt, persona string, err error) error {
	failurePath := filepath.Join(attempt.Dir, "review-failure.json")
	failure := ReviewFailure{
		OK:            false,
		AttemptID:     attempt.ID,
		Status:        "FAILED",
		FailureKind:   classifyFailureKind(err),
		Persona:       strings.TrimSpace(persona),
		Message:       strings.TrimSpace(err.Error()),
		FailurePath:   failurePath,
		ResumeCommand: "codedungeon code-review",
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	var providerErr ProviderFailureError
	if errors.As(err, &providerErr) {
		failure.RetryAfter = providerErr.RetryAfter
	}
	if err := writeAttemptManifest(outputDir, attempt, "FAILED"); err != nil {
		return err
	}
	if err := writeJSON(failurePath, failure); err != nil {
		return err
	}
	return writeJSON(filepath.Join(outputDir, "review-failure.json"), failure)
}

func classifyFailureKind(err error) FailureKind {
	var providerErr ProviderFailureError
	if errors.As(err, &providerErr) && providerErr.Kind != "" {
		return providerErr.Kind
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "rate_limit") || strings.Contains(lower, "rate limit") || strings.Contains(lower, "out_of_credits") || strings.Contains(lower, "429"):
		return FailureProviderRateLimit
	case strings.Contains(lower, "auth") || strings.Contains(lower, "api key") || strings.Contains(lower, "unauthorized"):
		return FailureProviderAuth
	case strings.Contains(lower, "context window") || strings.Contains(lower, "context length") || strings.Contains(lower, "maximum context"):
		return FailureProviderContext
	case strings.Contains(lower, "provider") || strings.Contains(lower, "claude") || strings.Contains(lower, "codex"):
		return FailureProviderProcess
	default:
		return FailureReviewValidation
	}
}

func removeStaleArtifact(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale review artifact %s: %w", path, err)
	}
	return nil
}
