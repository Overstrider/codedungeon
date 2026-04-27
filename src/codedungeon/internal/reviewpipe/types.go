// Package reviewpipe implements the deterministic steps of /code-review
// (dedupe, validator filter, classifier merge, render, verdict).
// Persona bug-hunting and validator/classifier semantic judgment stay LLM;
// the mechanics live here.
package reviewpipe

// Finding is the universal schema used across personas, validators,
// classifiers, and the final output. Unused fields are omitted via JSON tags.
type Finding struct {
	// Identity
	ID        string   `json:"id,omitempty"`
	Persona   string   `json:"persona,omitempty"`
	FlaggedBy []string `json:"flagged_by,omitempty"`

	// Core (required)
	Severity      string `json:"severity"`
	File          string `json:"file"`
	LineStart     int    `json:"line_start"`
	LineEnd       int    `json:"line_end"`
	Title         string `json:"title"`
	EvidenceQuote string `json:"evidence_quote,omitempty"`
	SuggestedFix  string `json:"suggested_fix,omitempty"`

	// Saboteur-specific
	FailureClass     string `json:"failure_class,omitempty"`
	ExploitSketch    string `json:"exploit_sketch,omitempty"`
	Steelman         string `json:"steelman,omitempty"`
	WhySteelmanFails string `json:"why_steelman_fails,omitempty"`

	// Newhire / security / spec
	Category        string `json:"category,omitempty"`
	AttackScenario  string `json:"attack_scenario,omitempty"`
	WhyItMatters    string `json:"why_it_matters,omitempty"`
	SpecClauseQuote string `json:"spec_clause_quote,omitempty"`

	// Post-validator
	Confirmed  *bool  `json:"confirmed,omitempty"`
	Confidence string `json:"confidence,omitempty"` // high | medium | low

	// Post-classifier
	Actionable               bool   `json:"actionable"`
	DesignDecision           bool   `json:"design_decision"`
	ClassifierEvidenceSource string `json:"classifier_evidence_source,omitempty"`
	ClassifierEvidenceQuote  string `json:"classifier_evidence_quote,omitempty"`
	ClassifierRationale      string `json:"classifier_rationale,omitempty"`

	// Stack-specialist source marker
	Source string `json:"source,omitempty"` // "persona" | "lang-specialist"
}

// PersonaFile is the top-level JSON each persona writes to findings-<persona>.json.
type PersonaFile struct {
	Persona             string    `json:"persona"`
	Model               string    `json:"model,omitempty"`
	Provider            string    `json:"provider,omitempty"`
	SessionID           string    `json:"session_id,omitempty"`
	BaseSHA             string    `json:"base_sha,omitempty"`
	HeadSHA             string    `json:"head_sha,omitempty"`
	ReviewedFiles       int       `json:"reviewed_files,omitempty"`
	Summary             string    `json:"summary,omitempty"`
	NoFindingsRationale string    `json:"no_findings_rationale,omitempty"`
	Findings            []Finding `json:"findings"`
}

// ValidatorResult is what oracle-reviewer-validator writes per validator-<idx>.json.
type ValidatorResult struct {
	FindingID  string `json:"id,omitempty"`  // matches Finding.ID
	Idx        int    `json:"idx,omitempty"` // fallback when ID missing
	Confirmed  bool   `json:"confirmed"`
	Confidence string `json:"confidence"` // high | medium | low
	Rationale  string `json:"rationale,omitempty"`
}

// ClassifierResult is what sage-reviewer-classifier writes per classifier-<idx>.json.
type ClassifierResult struct {
	FindingID      string `json:"id,omitempty"`
	Idx            int    `json:"idx,omitempty"`
	Classification string `json:"classification"` // "actionable" | "design_decision"
	Confidence     string `json:"confidence"`     // high | medium | low
	EvidenceSource string `json:"evidence_source,omitempty"`
	EvidenceQuote  string `json:"evidence_quote,omitempty"`
	Rationale      string `json:"rationale,omitempty"`
}

// Tally is the counts JSON embedded in review.json and the PR comment marker.
type Tally struct {
	Actionable struct {
		P0 int `json:"p0"`
		P1 int `json:"p1"`
		P2 int `json:"p2"`
	} `json:"actionable"`
	DesignDecisions int `json:"design_decisions"`
	Dropped         int `json:"dropped"`
	SuppressedNits  int `json:"suppressed_nits,omitempty"`
}

// ReviewJSON is the final review.json body.
type ReviewJSON struct {
	Verdict         string    `json:"verdict"` // APPROVED | CHANGES_REQUESTED
	Tally           Tally     `json:"tally"`
	Findings        []Finding `json:"findings"`
	PersonasRun     []string  `json:"personas_run"`
	ValidatorModel  string    `json:"validator_model"`
	ClassifierModel string    `json:"classifier_model"`
	StackSpecialist string    `json:"stack_specialist,omitempty"`
}

// sevRank returns a numeric rank where 0=P0, 1=P1, 2=P2, -1=unknown.
func sevRank(s string) int {
	switch s {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	}
	return -1
}

// sevFromRank is the inverse.
func sevFromRank(r int) string {
	switch r {
	case 0:
		return "P0"
	case 1:
		return "P1"
	case 2:
		return "P2"
	}
	return ""
}

// promote bumps severity one tier: P2→P1→P0. Caps at P0.
func promote(s string) string {
	r := sevRank(s)
	if r <= 0 {
		return "P0"
	}
	return sevFromRank(r - 1)
}
