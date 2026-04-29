package codereview

import (
	"context"
	"encoding/json"
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
	if err := os.MkdirAll(filepath.Join(req.OutputDir, "personas"), 0o755); err != nil {
		return Result{}, err
	}
	if err := writeJSON(filepath.Join(req.OutputDir, "review-request.json"), req); err != nil {
		return Result{}, err
	}

	reviews := make([]PersonaReview, 0, len(personas))
	for _, persona := range personas {
		outPath := filepath.Join(req.OutputDir, "personas", persona+".json")
		if err := runner.RunPersona(ctx, req, persona, outPath); err != nil {
			return Result{}, fmt.Errorf("persona %s failed: %w", persona, err)
		}
		review, err := readPersonaReview(outPath)
		if err != nil {
			return Result{}, err
		}
		if err := ValidatePersonaReview(review, req.ProjectRules); err != nil {
			return Result{}, err
		}
		reviews = append(reviews, review)
	}

	decisionPath := filepath.Join(req.OutputDir, "review-decision.json")
	if err := runner.RunAdjudicator(ctx, req, reviews, decisionPath); err != nil {
		return Result{}, fmt.Errorf("review-decision failed: %w", err)
	}
	decision, err := readDecision(decisionPath)
	if err != nil {
		return Result{}, err
	}
	findings := collectFindings(reviews)
	if err := ValidateDecision(decision, reviews, findings); err != nil {
		return Result{}, err
	}

	result := Result{
		URL:               req.URL,
		Verdict:           decision.Verdict,
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
	if err := os.WriteFile(result.ReviewMDPath, []byte(markdown), 0o644); err != nil {
		return Result{}, err
	}
	if err := writeJSON(result.ReviewSummaryPath, summary); err != nil {
		return Result{}, err
	}
	if err := writeJSON(result.ResultJSONPath, result); err != nil {
		return Result{}, err
	}
	if err := writeJSON(result.ReviewJSONPath, result); err != nil {
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
	if !substantive(f.EvidenceQuote) {
		return fmt.Errorf("evidence_quote is required and must be substantive")
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
