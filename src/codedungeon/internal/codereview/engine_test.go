package codereview

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/reviewpipe"
)

type fakeRunner struct {
	personas map[string]PersonaReview
	decision *Decision
	failures map[string]error
}

func (f fakeRunner) RunPersona(_ context.Context, _ Request, persona string, outPath string) error {
	if err := f.failures[persona]; err != nil {
		return err
	}
	review, ok := f.personas[persona]
	if !ok {
		return os.ErrNotExist
	}
	return writeTestJSON(outPath, review)
}

func (f fakeRunner) RunAdjudicator(_ context.Context, _ Request, _ []PersonaReview, outPath string) error {
	if err := f.failures["adjudicator"]; err != nil {
		return err
	}
	if f.decision == nil {
		return os.ErrNotExist
	}
	return writeTestJSON(outPath, *f.decision)
}

func TestExecuteFailureDoesNotLeaveStalePublishedReviewArtifacts(t *testing.T) {
	req := validRequest(t)
	if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	staleSummary := filepath.Join(req.OutputDir, "review-summary.json")
	staleResult := filepath.Join(req.OutputDir, "review-result.json")
	if err := os.WriteFile(staleSummary, []byte(`{"verdict":"CHANGES_REQUESTED","stale":true}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staleResult, []byte(`{"verdict":"CHANGES_REQUESTED","stale":true}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := fakeRunner{
		personas: approvedPersonas(),
		failures: map[string]error{"security": errors.New("provider rate limit")},
	}

	_, err := Execute(context.Background(), req, runner)
	if err == nil {
		t.Fatal("Execute succeeded despite provider failure")
	}
	for _, stalePath := range []string{staleSummary, staleResult, filepath.Join(req.OutputDir, "review.md"), filepath.Join(req.OutputDir, "review.json")} {
		if _, statErr := os.Stat(stalePath); !os.IsNotExist(statErr) {
			t.Fatalf("stale published artifact still exists after failed attempt: %s", stalePath)
		}
	}
	failurePath := filepath.Join(req.OutputDir, "review-failure.json")
	body, readErr := os.ReadFile(failurePath)
	if readErr != nil {
		t.Fatalf("review failure artifact missing: %v", readErr)
	}
	var failure ReviewFailure
	if err := json.Unmarshal(body, &failure); err != nil {
		t.Fatalf("review failure artifact invalid: %v\n%s", err, body)
	}
	if failure.AttemptID == "" || failure.Status != "FAILED" || failure.Persona != "security" || failure.FailureKind == "" {
		t.Fatalf("failure artifact missing required fields: %+v", failure)
	}
	if failure.FailurePath == "" {
		t.Fatalf("failure artifact should point at the failed attempt artifact: %+v", failure)
	}
	if _, err := os.Stat(filepath.Join(req.OutputDir, "attempts", failure.AttemptID)); err != nil {
		t.Fatalf("attempt directory missing: %v", err)
	}
}

func TestExecuteRejectsApprovedPersonasWithoutFinalDecision(t *testing.T) {
	req := validRequest(t)
	runner := fakeRunner{personas: approvedPersonas()}

	_, err := Execute(context.Background(), req, runner)
	if err == nil {
		t.Fatal("Execute accepted approvals without a final adjudicator decision")
	}
	if !strings.Contains(err.Error(), "review-decision") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteRejectsEmptyPersonaWithoutSubstantiveApproval(t *testing.T) {
	req := validRequest(t)
	personas := approvedPersonas()
	p := personas["security"]
	p.ApprovalRationale = "looks fine"
	p.RisksConsidered = nil
	personas["security"] = p
	runner := fakeRunner{
		personas: personas,
		decision: &Decision{
			Verdict:           VerdictApproved,
			DecidedBy:         "adjudicator",
			Model:             "test-model",
			Provider:          "test",
			ApprovalRationale: strings.Repeat("all personas provided concrete approval evidence; ", 3),
			PersonaVerdicts:   allPersonaVerdicts(VerdictApproved),
		},
	}

	_, err := Execute(context.Background(), req, runner)
	if err == nil {
		t.Fatal("Execute accepted an empty persona review without substantive approval")
	}
	if !strings.Contains(err.Error(), "security") || !strings.Contains(err.Error(), "approval_rationale") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteRejectsApprovedDecisionWithActionableFinding(t *testing.T) {
	req := validRequest(t)
	personas := approvedPersonas()
	p := personas["saboteur"]
	p.Verdict = VerdictChangesRequested
	p.ApprovalRationale = ""
	p.Findings = []reviewpipe.Finding{{
		Severity:      "P1",
		File:          "backend/src/main.rs",
		LineStart:     12,
		LineEnd:       12,
		Title:         "request can fail silently",
		EvidenceQuote: "The handler returns Ok(()) after the provider failure path without persisting the failed assistant message or exposing the error.",
		Actionable:    true,
	}}
	personas["saboteur"] = p
	runner := fakeRunner{
		personas: personas,
		decision: &Decision{
			Verdict:           VerdictApproved,
			DecidedBy:         "adjudicator",
			Model:             "test-model",
			Provider:          "test",
			ApprovalRationale: strings.Repeat("approved despite findings should be rejected; ", 3),
			PersonaVerdicts:   allPersonaVerdicts(VerdictApproved),
		},
	}

	_, err := Execute(context.Background(), req, runner)
	if err == nil {
		t.Fatal("Execute accepted APPROVED while an actionable finding remained")
	}
	if !strings.Contains(err.Error(), "actionable") && !strings.Contains(err.Error(), "saboteur") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePersonaReviewAcceptsShortVerbatimEvidenceQuote(t *testing.T) {
	review := approvedPersonas()["tests"]
	review.Verdict = VerdictChangesRequested
	review.ApprovalRationale = ""
	review.RisksConsidered = nil
	review.Findings = []reviewpipe.Finding{{
		Severity:      "P2",
		File:          "backend/tests/api_conversations.rs",
		LineStart:     91,
		LineEnd:       92,
		Title:         "ordering assertion is missing",
		EvidenceQuote: "assert_eq!(arr.len(), 2);",
		SuggestedFix:  "Assert the ordered titles instead of only the row count.",
	}}

	if err := ValidatePersonaReview(review, review.ProjectRules); err != nil {
		t.Fatalf("short verbatim evidence quote was rejected: %v", err)
	}
}

func TestValidatePersonaReviewRejectsPlaceholderEvidenceQuote(t *testing.T) {
	review := approvedPersonas()["tests"]
	review.Verdict = VerdictChangesRequested
	review.ApprovalRationale = ""
	review.RisksConsidered = nil
	review.Findings = []reviewpipe.Finding{{
		Severity:      "P2",
		File:          "backend/tests/api_conversations.rs",
		LineStart:     91,
		LineEnd:       92,
		Title:         "ordering assertion is missing",
		EvidenceQuote: "exact quote",
		SuggestedFix:  "Assert the ordered titles instead of only the row count.",
	}}

	err := ValidatePersonaReview(review, review.ProjectRules)
	if err == nil {
		t.Fatal("placeholder evidence quote was accepted")
	}
	if !strings.Contains(err.Error(), "evidence_quote") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteWritesSubstantiveStandaloneReview(t *testing.T) {
	req := validRequest(t)
	runner := fakeRunner{
		personas: approvedPersonas(),
		decision: &Decision{
			Verdict:           VerdictApproved,
			DecidedBy:         "adjudicator",
			Model:             "test-model",
			Provider:          "test",
			ApprovalRationale: strings.Repeat("the implementation was reviewed against project context, task context, diff, failure modes, and verification evidence; ", 2),
			PersonaVerdicts:   allPersonaVerdicts(VerdictApproved),
		},
	}

	result, err := Execute(context.Background(), req, runner)
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != VerdictApproved || result.Integrity.Status != IntegrityPass {
		t.Fatalf("unexpected result: %+v", result)
	}
	body, err := os.ReadFile(result.ReviewMDPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{"Review Summary", "No actionable findings remain", "Personas:", "Artifacts:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("review.md missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"Risks considered", "Reviewed scope:", "#### "} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("review.md contains verbose detail %q:\n%s", forbidden, text)
		}
	}
	if strings.Contains(text, "_None._") {
		t.Fatalf("review.md contains empty-template marker:\n%s", text)
	}
}

func TestRenderBlockedReviewDoesNotDisplayPersonaApproved(t *testing.T) {
	personasByName := approvedPersonas()
	persona := personasByName["saboteur"]
	persona.Verdict = VerdictChangesRequested
	persona.ApprovalRationale = ""
	persona.Findings = []reviewpipe.Finding{{
		Severity:      "P1",
		File:          "backend/src/main.rs",
		LineStart:     22,
		LineEnd:       22,
		Title:         "failure path loses user-visible error",
		EvidenceQuote: "The approved project rules say generated artifacts must not be committed, but the changed handler records success state before checking the provider error branch.",
	}}
	personasByName["saboteur"] = persona

	var personas []PersonaReview
	verdicts := map[string]string{}
	for _, name := range DefaultPersonas {
		personas = append(personas, personasByName[name])
		verdicts[name] = personasByName[name].Verdict
	}
	result := Result{
		URL:          "https://github.com/acme/example/pull/1",
		Verdict:      VerdictChangesRequested,
		ProjectRules: ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"},
		Personas:     personas,
		Decision: Decision{
			Verdict:           VerdictChangesRequested,
			DecidedBy:         "adjudicator",
			Model:             "test-model",
			Provider:          "test",
			ApprovalRationale: strings.Repeat("blocked because at least one persona reported a concrete failure path; ", 2),
			PersonaVerdicts:   verdicts,
		},
		Findings:  collectFindings(personas),
		Integrity: IntegrityReport{Status: IntegrityPass},
	}
	body, err := RenderMarkdown(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "Persona Approvals") {
		t.Fatalf("blocked review still renders approval section:\n%s", body)
	}
	if strings.Contains(body, "newhire - APPROVED") {
		t.Fatalf("blocked review renders persona approval label:\n%s", body)
	}
	if strings.Contains(body, "APPROVED") {
		t.Fatalf("blocked review renders uppercase approved marker:\n%s", body)
	}
	for _, forbidden := range []string{
		"#### newhire",
		"Persona outcome:",
		"Reviewed scope:",
		"Verification checked:",
		"reviewed concrete project and task risks",
		"PROJECT_RULES_STATUS",
		"PROJECT_RULES_DIGEST",
		"PROJECT_RULES_READ",
		"PROJECT_RULES_STATUS: approved",
		"approved project rules",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("blocked review rendered verbose persona detail %q:\n%s", forbidden, body)
		}
	}
	for _, want := range []string{
		"### Findings",
		"### Review Summary",
		"### Review Context",
		"Project rules context: read",
		"Personas: newhire=NO_BLOCKING_FINDINGS",
		"Artifacts:",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("blocked review missing concise section %q:\n%s", want, body)
		}
	}
}

func TestBuildReviewSummaryDedupesAndCapsCommentFindings(t *testing.T) {
	personasByName := approvedPersonas()
	saboteur := personasByName["saboteur"]
	saboteur.Verdict = VerdictChangesRequested
	saboteur.ApprovalRationale = ""
	saboteur.Findings = []reviewpipe.Finding{
		reviewFinding("P1", ".codedungeon/codedungeon.db", 1, "Generated database committed", "Generated database binary delta is present in the PR diff and should not be reviewed as source."),
		reviewFinding("P2", ".codedungeon/codedungeon.db", 1, "Database state committed", "The same generated database binary delta appears in another persona output and carries the same generated artifact risk."),
	}
	personasByName["saboteur"] = saboteur
	security := personasByName["security"]
	security.Verdict = VerdictChangesRequested
	security.ApprovalRationale = ""
	security.Findings = []reviewpipe.Finding{
		reviewFinding("P1", ".codedungeon/codedungeon.db", 1, "Generated DB in PR", "The generated database binary changed from base to head and must not be committed."),
	}
	personasByName["security"] = security
	tests := personasByName["tests"]
	tests.Verdict = VerdictChangesRequested
	tests.ApprovalRationale = ""
	for i := 0; i < 10; i++ {
		tests.Findings = append(tests.Findings, reviewFinding("P2", "backend/src/main.go", 10+i, "extra issue", "Additional lower severity finding with concrete evidence for comment capping and deterministic summary overflow handling."))
	}
	personasByName["tests"] = tests

	var personas []PersonaReview
	verdicts := map[string]string{}
	for _, name := range DefaultPersonas {
		personas = append(personas, personasByName[name])
		verdicts[name] = personasByName[name].Verdict
	}
	result := Result{
		URL:              "https://github.com/acme/example/pull/1",
		Verdict:          VerdictChangesRequested,
		ProjectRules:     ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"},
		Personas:         personas,
		Decision:         Decision{Verdict: VerdictChangesRequested, DecidedBy: "adjudicator", Model: "test-model", Provider: "test", PersonaVerdicts: verdicts},
		Findings:         collectFindings(personas),
		Integrity:        IntegrityReport{Status: IntegrityPass},
		ReviewJSONPath:   "review.json",
		ResultJSONPath:   "review-result.json",
		DecisionJSONPath: "review-decision.json",
	}
	summary, err := BuildReviewSummary(result)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Tally.Actionable.P1 != 1 {
		t.Fatalf("P1 tally = %d, want 1 after dedupe", summary.Tally.Actionable.P1)
	}
	if len(summary.Findings) != MaxCommentFindings {
		t.Fatalf("comment findings = %d, want cap %d", len(summary.Findings), MaxCommentFindings)
	}
	if summary.SuppressedInComment == 0 {
		t.Fatal("expected suppressed_in_comment to record overflow findings")
	}
	body, err := RenderMarkdown(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(body, ".codedungeon/codedungeon.db") != 1 {
		t.Fatalf("deduped database finding should appear once:\n%s", body)
	}
	if !strings.Contains(body, "Additional findings not shown in comment:") {
		t.Fatalf("comment did not disclose capped findings:\n%s", body)
	}
}

func TestValidateResultRejectsFindingsMismatch(t *testing.T) {
	personasByName := approvedPersonas()
	persona := personasByName["saboteur"]
	persona.Verdict = VerdictChangesRequested
	persona.ApprovalRationale = ""
	persona.Findings = []reviewpipe.Finding{{
		Severity:      "P1",
		File:          "backend/src/main.rs",
		LineStart:     22,
		LineEnd:       22,
		Title:         "failure path loses user-visible error",
		EvidenceQuote: "The changed handler records success state before checking the provider error branch, so the UI can report completion after a failed request.",
	}}
	personasByName["saboteur"] = persona

	var personas []PersonaReview
	verdicts := map[string]string{}
	for _, name := range DefaultPersonas {
		personas = append(personas, personasByName[name])
		verdicts[name] = personasByName[name].Verdict
	}
	err := ValidateResult(Result{
		URL:          "https://github.com/acme/example/pull/1",
		Verdict:      VerdictChangesRequested,
		ProjectRules: ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"},
		Personas:     personas,
		Decision: Decision{
			Verdict:           VerdictChangesRequested,
			DecidedBy:         "adjudicator",
			Model:             "test-model",
			Provider:          "test",
			ApprovalRationale: strings.Repeat("blocked because at least one persona reported a concrete failure path; ", 2),
			PersonaVerdicts:   verdicts,
		},
		Findings:  nil,
		Integrity: IntegrityReport{Status: IntegrityPass},
	})
	if err == nil {
		t.Fatal("ValidateResult accepted a result whose top-level findings omitted persona findings")
	}
	if !strings.Contains(err.Error(), "findings do not match") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func validRequest(t *testing.T) Request {
	t.Helper()
	return Request{
		URL:            "https://github.com/acme/example/pull/1",
		ProjectContext: strings.Repeat("Project requires explicit review evidence, strict review integrity, and no empty approvals. ", 2),
		TaskContext:    strings.Repeat("Implement a chat app with backend, frontend, persistence, streaming, verification, and documented risks. ", 2),
		OutputDir:      filepath.Join(t.TempDir(), "review"),
		ProjectRules: ProjectRulesEnvelope{
			Status: "approved",
			Digest: "rules-digest",
			Read:   "yes",
		},
	}
}

func approvedPersonas() map[string]PersonaReview {
	out := map[string]PersonaReview{}
	for _, persona := range DefaultPersonas {
		out[persona] = PersonaReview{
			Persona:             persona,
			Verdict:             VerdictApproved,
			Model:               "test-model",
			Provider:            "test",
			SessionID:           "session-" + persona,
			ReviewedFiles:       3,
			ReviewedScope:       []string{"backend/src/main.rs", "frontend/src/app/page.tsx"},
			ApprovalRationale:   persona + " reviewed concrete project and task risks with no blocking defect after checking changed code and verification evidence.",
			RisksConsidered:     []string{"correctness reviewed", "verification reviewed"},
			ProjectRules:        ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"},
			Findings:            []reviewpipe.Finding{},
			VerificationChecked: []string{"go test ./..."},
		}
	}
	return out
}

func reviewFinding(severity, file string, line int, title, evidence string) reviewpipe.Finding {
	return reviewpipe.Finding{
		Severity:      severity,
		File:          file,
		LineStart:     line,
		LineEnd:       line,
		Title:         title,
		Category:      "generated-artifact",
		EvidenceQuote: evidence,
		SuggestedFix:  "Remove the generated artifact from the PR and rerun the review gate.",
	}
}

func allPersonaVerdicts(verdict string) map[string]string {
	out := map[string]string{}
	for _, persona := range DefaultPersonas {
		out[persona] = verdict
	}
	return out
}

func writeTestJSON(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}
