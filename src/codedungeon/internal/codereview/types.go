package codereview

import "github.com/loldinis/codedungeon/internal/reviewpipe"

const (
	VerdictApproved         = "APPROVED"
	VerdictChangesRequested = "CHANGES_REQUESTED"

	IntegrityPass = "PASS"
	IntegrityFail = "FAIL"
)

var DefaultPersonas = []string{"saboteur", "newhire", "security", "spec", "tests"}

type ProjectRulesEnvelope struct {
	Status string `json:"status"`
	Digest string `json:"digest"`
	Read   string `json:"read"`
}

type Request struct {
	URL            string               `json:"url"`
	ProjectContext string               `json:"project_context"`
	TaskContext    string               `json:"task_context"`
	TargetContext  string               `json:"target_context,omitempty"`
	OutputDir      string               `json:"output_dir"`
	BaseSHA        string               `json:"base_sha,omitempty"`
	HeadSHA        string               `json:"head_sha,omitempty"`
	PRNumber       string               `json:"pr_number,omitempty"`
	ProjectRules   ProjectRulesEnvelope `json:"project_rules"`
	Personas       []string             `json:"personas,omitempty"`
}

type PersonaReview struct {
	Persona             string                 `json:"persona"`
	Verdict             string                 `json:"verdict"`
	Model               string                 `json:"model"`
	Provider            string                 `json:"provider"`
	SessionID           string                 `json:"session_id"`
	ReviewedFiles       int                    `json:"reviewed_files"`
	ReviewedScope       []string               `json:"reviewed_scope"`
	ApprovalRationale   string                 `json:"approval_rationale,omitempty"`
	RisksConsidered     []string               `json:"risks_considered,omitempty"`
	VerificationChecked []string               `json:"verification_checked,omitempty"`
	ProjectRules        ProjectRulesEnvelope   `json:"project_rules"`
	Findings            []reviewpipe.Finding   `json:"findings"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

type Decision struct {
	Verdict           string            `json:"verdict"`
	DecidedBy         string            `json:"decided_by"`
	Model             string            `json:"model"`
	Provider          string            `json:"provider"`
	ApprovalRationale string            `json:"approval_rationale,omitempty"`
	PersonaVerdicts   map[string]string `json:"persona_verdicts"`
	CreatedAt         string            `json:"created_at,omitempty"`
}

type IntegrityReport struct {
	Status  string   `json:"status"`
	Reasons []string `json:"reasons,omitempty"`
}

type ReviewCoverage struct {
	Personas        []string          `json:"personas"`
	PersonaOutcomes map[string]string `json:"persona_outcomes"`
	Validator       string            `json:"validator"`
	Classifier      string            `json:"classifier"`
	StackSpecialist string            `json:"stack_specialist,omitempty"`
}

type ReviewArtifacts struct {
	ReviewJSONPath   string `json:"review_json_path"`
	ResultJSONPath   string `json:"result_json_path"`
	DecisionJSONPath string `json:"decision_json_path"`
	PersonaDir       string `json:"persona_dir"`
}

type ReviewSummary struct {
	URL                 string               `json:"url"`
	Verdict             string               `json:"verdict"`
	Integrity           IntegrityReport      `json:"integrity"`
	ProjectRules        ProjectRulesEnvelope `json:"project_rules"`
	Tally               reviewpipe.Tally     `json:"tally"`
	Findings            []reviewpipe.Finding `json:"findings"`
	DecisionRationale   string               `json:"decision_rationale"`
	SuppressedInComment int                  `json:"suppressed_in_comment,omitempty"`
	Coverage            ReviewCoverage       `json:"coverage"`
	FullArtifacts       ReviewArtifacts      `json:"full_artifacts"`
}

type Result struct {
	URL               string               `json:"url"`
	Verdict           string               `json:"verdict"`
	ProjectRules      ProjectRulesEnvelope `json:"project_rules"`
	Personas          []PersonaReview      `json:"personas"`
	Decision          Decision             `json:"decision"`
	Findings          []reviewpipe.Finding `json:"findings"`
	Integrity         IntegrityReport      `json:"integrity"`
	ReviewMDPath      string               `json:"review_md_path"`
	ReviewJSONPath    string               `json:"review_json_path"`
	ResultJSONPath    string               `json:"result_json_path"`
	ReviewSummaryPath string               `json:"review_summary_path"`
	DecisionJSONPath  string               `json:"decision_json_path"`
	Summary           ReviewSummary        `json:"summary"`
	CommentURL        string               `json:"comment_url,omitempty"`
}
