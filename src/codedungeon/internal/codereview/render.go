package codereview

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/loldinis/codedungeon/internal/reviewpipe"
)

const MaxCommentFindings = 8

func BuildReviewSummary(result Result) (ReviewSummary, error) {
	if err := ValidateResult(result); err != nil {
		return ReviewSummary{}, err
	}
	allFindings := dedupeSummaryFindings(result.Findings)
	actionable := make([]reviewpipe.Finding, 0, len(allFindings))
	for _, finding := range allFindings {
		if finding.Actionable {
			actionable = append(actionable, finding)
		}
	}
	sortFindings(actionable)
	commentFindings := actionable
	suppressed := 0
	if len(commentFindings) > MaxCommentFindings {
		suppressed = len(commentFindings) - MaxCommentFindings
		commentFindings = commentFindings[:MaxCommentFindings]
	}

	return ReviewSummary{
		URL:                 result.URL,
		Verdict:             result.Verdict,
		Integrity:           result.Integrity,
		ProjectRules:        result.ProjectRules,
		Tally:               reviewpipe.BuildTally(allFindings, 0, 0),
		Findings:            commentFindings,
		DecisionRationale:   conciseDecisionRationale(result, len(allFindings)),
		SuppressedInComment: suppressed,
		Coverage: ReviewCoverage{
			Personas:        personaNames(result.Personas),
			PersonaOutcomes: personaOutcomes(result),
			Validator:       "standalone-adjudicator",
			Classifier:      "standalone-adjudicator",
		},
		FullArtifacts: ReviewArtifacts{
			ReviewJSONPath:   result.ReviewJSONPath,
			ResultJSONPath:   result.ResultJSONPath,
			DecisionJSONPath: result.DecisionJSONPath,
			PersonaDir:       filepath.Join(filepath.Dir(result.ReviewJSONPath), "personas"),
		},
	}, nil
}

func RenderMarkdown(result Result) (string, error) {
	summary, err := BuildReviewSummary(result)
	if err != nil {
		return "", err
	}
	return RenderSummaryMarkdown(summary), nil
}

func RenderSummaryMarkdown(summary ReviewSummary) string {
	var b bytes.Buffer
	fmt.Fprintln(&b, "## CodeDungeon Code Review")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "**Verdict**: %s\n", summary.Verdict)
	fmt.Fprintf(&b, "**Review Integrity**: %s\n", summary.Integrity.Status)
	fmt.Fprintf(&b, "**URL**: %s\n", summary.URL)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "### Findings")
	fmt.Fprintln(&b)
	if len(summary.Findings) == 0 {
		fmt.Fprintln(&b, "No actionable findings remain after final adjudication.")
	} else {
		for _, finding := range summary.Findings {
			fmt.Fprintf(&b, "- %s %s:%d - %s\n", finding.Severity, finding.File, finding.LineStart, finding.Title)
			fmt.Fprintf(&b, "  Evidence: %s\n", commentEvidence(summary, finding.EvidenceQuote))
			if strings.TrimSpace(finding.SuggestedFix) != "" {
				fmt.Fprintf(&b, "  Suggested fix: %s\n", strings.TrimSpace(finding.SuggestedFix))
			}
		}
		if summary.SuppressedInComment > 0 {
			fmt.Fprintf(&b, "\nAdditional findings not shown in comment: %d. See full artifacts.\n", summary.SuppressedInComment)
		}
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "### Review Summary")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Actionable: %d P0 / %d P1 / %d P2\n", summary.Tally.Actionable.P0, summary.Tally.Actionable.P1, summary.Tally.Actionable.P2)
	fmt.Fprintf(&b, "- Accepted design decisions: %d\n", summary.Tally.DesignDecisions)
	fmt.Fprintf(&b, "- Dropped by validator: %d\n", summary.Tally.Dropped)
	if summary.Tally.SuppressedNits > 0 {
		fmt.Fprintf(&b, "- Suppressed nits: %d\n", summary.Tally.SuppressedNits)
	}
	fmt.Fprintf(&b, "- Personas: %s\n", personaOutcomeString(summary.Coverage))
	fmt.Fprintf(&b, "- Validator: %s\n", emptyDefault(summary.Coverage.Validator, "not-recorded"))
	fmt.Fprintf(&b, "- Classifier: %s\n", emptyDefault(summary.Coverage.Classifier, "not-recorded"))
	if strings.TrimSpace(summary.Coverage.StackSpecialist) != "" {
		fmt.Fprintf(&b, "- Stack specialist: %s\n", summary.Coverage.StackSpecialist)
	}
	fmt.Fprintf(&b, "- Decision: %s\n", strings.TrimSpace(summary.DecisionRationale))
	fmt.Fprintf(&b, "- Artifacts: %s, %s, %s\n", summary.FullArtifacts.ReviewJSONPath, summary.FullArtifacts.ResultJSONPath, summary.FullArtifacts.DecisionJSONPath)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "### Review Context")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Project rules context: %s\n", rulesContextLabel(summary.ProjectRules))
	fmt.Fprintf(&b, "- Project rules digest: %s\n", shortDigest(summary.ProjectRules.Digest))
	return b.String()
}

func dedupeSummaryFindings(findings []reviewpipe.Finding) []reviewpipe.Finding {
	out := make([]reviewpipe.Finding, 0, len(findings))
	for _, finding := range findings {
		finding = normalizeSummaryFinding(finding)
		merged := false
		for i := range out {
			if sameSummaryFinding(out[i], finding) {
				out[i] = mergeSummaryFinding(out[i], finding)
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, finding)
		}
	}
	sortFindings(out)
	return out
}

func normalizeSummaryFinding(finding reviewpipe.Finding) reviewpipe.Finding {
	if finding.LineEnd <= 0 {
		finding.LineEnd = finding.LineStart
	}
	if finding.LineStart <= 0 {
		finding.LineStart = 1
	}
	if finding.LineEnd < finding.LineStart {
		finding.LineEnd = finding.LineStart
	}
	if finding.Persona != "" && !containsString(finding.FlaggedBy, finding.Persona) {
		finding.FlaggedBy = append(finding.FlaggedBy, finding.Persona)
	}
	if !finding.DesignDecision {
		finding.Actionable = true
	}
	return finding
}

func sameSummaryFinding(a, b reviewpipe.Finding) bool {
	if strings.TrimSpace(a.File) != strings.TrimSpace(b.File) {
		return false
	}
	if a.LineStart > b.LineEnd || b.LineStart > a.LineEnd {
		return false
	}
	aCategory := summaryCategory(a)
	bCategory := summaryCategory(b)
	return aCategory == "" || bCategory == "" || aCategory == bCategory
}

func mergeSummaryFinding(a, b reviewpipe.Finding) reviewpipe.Finding {
	if severityRank(b.Severity) < severityRank(a.Severity) {
		a.Severity = b.Severity
	}
	if b.LineStart < a.LineStart {
		a.LineStart = b.LineStart
	}
	if b.LineEnd > a.LineEnd {
		a.LineEnd = b.LineEnd
	}
	for _, persona := range b.FlaggedBy {
		if !containsString(a.FlaggedBy, persona) {
			a.FlaggedBy = append(a.FlaggedBy, persona)
		}
	}
	if strings.TrimSpace(a.EvidenceQuote) == "" {
		a.EvidenceQuote = b.EvidenceQuote
	}
	if strings.TrimSpace(a.SuggestedFix) == "" {
		a.SuggestedFix = b.SuggestedFix
	}
	a.Actionable = a.Actionable || b.Actionable
	a.DesignDecision = a.DesignDecision && b.DesignDecision
	return a
}

func sortFindings(findings []reviewpipe.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if severityRank(findings[i].Severity) != severityRank(findings[j].Severity) {
			return severityRank(findings[i].Severity) < severityRank(findings[j].Severity)
		}
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		if findings[i].LineStart != findings[j].LineStart {
			return findings[i].LineStart < findings[j].LineStart
		}
		return findings[i].Title < findings[j].Title
	})
}

func summaryCategory(finding reviewpipe.Finding) string {
	switch {
	case strings.TrimSpace(finding.FailureClass) != "":
		return strings.TrimSpace(finding.FailureClass)
	case strings.TrimSpace(finding.Category) != "":
		return strings.TrimSpace(finding.Category)
	default:
		return ""
	}
}

func conciseDecisionRationale(result Result, findingCount int) string {
	if result.Verdict == VerdictApproved {
		return "Final adjudicator approved after all personas reported no blocking findings."
	}
	return fmt.Sprintf("Final adjudicator returned CHANGES_REQUESTED with %d final finding(s).", findingCount)
}

func personaNames(personas []PersonaReview) []string {
	out := make([]string, 0, len(personas))
	for _, persona := range personas {
		out = append(out, persona.Persona)
	}
	sort.Strings(out)
	return out
}

func personaOutcomes(result Result) map[string]string {
	out := map[string]string{}
	for _, persona := range result.Personas {
		verdict := persona.Verdict
		if result.Verdict != VerdictApproved && verdict == VerdictApproved {
			verdict = "NO_BLOCKING_FINDINGS"
		}
		out[persona.Persona] = verdict
	}
	return out
}

func personaOutcomeString(coverage ReviewCoverage) string {
	names := append([]string{}, coverage.Personas...)
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%s", name, coverage.PersonaOutcomes[name]))
	}
	return strings.Join(parts, ", ")
}

func severityRank(severity string) int {
	switch severity {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	default:
		return 99
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func emptyDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func rulesContextLabel(rules ProjectRulesEnvelope) string {
	if strings.TrimSpace(rules.Read) == "yes" && strings.TrimSpace(rules.Digest) != "" {
		return "read"
	}
	return "not recorded"
}

func shortDigest(digest string) string {
	digest = strings.TrimSpace(digest)
	if len(digest) <= 12 {
		return digest
	}
	return digest[:12]
}

func commentEvidence(summary ReviewSummary, evidence string) string {
	evidence = strings.TrimSpace(evidence)
	if summary.Verdict == VerdictApproved {
		return evidence
	}
	evidence = strings.ReplaceAll(evidence, "The approved project rules say:", "The project rules say:")
	evidence = strings.ReplaceAll(evidence, "the approved project rules say:", "the project rules say:")
	evidence = strings.ReplaceAll(evidence, "approved project rules", "project rules")
	evidence = strings.ReplaceAll(evidence, "Approved project rules", "Project rules")
	return evidence
}
