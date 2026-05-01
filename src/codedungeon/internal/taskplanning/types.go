package taskplanning

const (
	StatusRunning        = "RUNNING"
	StatusCompleted      = "COMPLETED"
	StatusNeedsUserInput = "NEEDS_USER_INPUT"
	StatusFailed         = "FAILED"

	HumanGateMaterialAmbiguity = "material_ambiguity"
	HumanGateAlwaysBeforeSplit = "always_before_split"
	HumanGateNever             = "never"
)

var DefaultExplorationRoles = []string{"planner_architect", "domain_planner", "qa_planner", "risk_planner"}

type ProjectRulesEnvelope struct {
	Status string `json:"status"`
	Digest string `json:"digest"`
	Read   string `json:"read"`
}

type Request struct {
	SessionID       string               `json:"session_id,omitempty"`
	RunID           int64                `json:"run_id,omitempty"`
	Prompt          string               `json:"prompt"`
	Mode            string               `json:"mode"`
	ProjectContext  string               `json:"project_context"`
	OutputDir       string               `json:"output_dir"`
	Roles           []string             `json:"roles,omitempty"`
	HumanGatePolicy string               `json:"human_gate_policy"`
	ProjectRules    ProjectRulesEnvelope `json:"project_rules"`
	MaxRounds       int                  `json:"max_rounds,omitempty"`
	AutoRepair      bool                 `json:"auto_repair,omitempty"`
}

type Result struct {
	OK             bool                 `json:"ok"`
	SessionID      string               `json:"session_id"`
	RunID          int64                `json:"run_id,omitempty"`
	Status         string               `json:"status"`
	NeedsUserInput bool                 `json:"needs_user_input"`
	OutputDir      string               `json:"output_dir"`
	RequestPath    string               `json:"request_path"`
	BlackboardPath string               `json:"blackboard_path"`
	EvaluationPath string               `json:"evaluation_path,omitempty"`
	TaskGraphPath  string               `json:"task_graph_path,omitempty"`
	MasterPath     string               `json:"master_path,omitempty"`
	Agents         []AgentOutput        `json:"agents"`
	Evaluation     *Evaluation          `json:"evaluation,omitempty"`
	TaskGraph      *TaskGraph           `json:"task_graph,omitempty"`
	Artifacts      []string             `json:"artifacts"`
	ProjectRules   ProjectRulesEnvelope `json:"project_rules"`
	Metadata       map[string]any       `json:"metadata,omitempty"`
}

type AgentRequest struct {
	SessionID       string               `json:"session_id"`
	RunID           int64                `json:"run_id,omitempty"`
	Role            string               `json:"role"`
	Round           int                  `json:"round"`
	ContextPacket   string               `json:"context_packet"`
	OutputDir       string               `json:"output_dir"`
	OutputPath      string               `json:"output_path"`
	BlackboardPath  string               `json:"blackboard_path"`
	PreviousOutputs []AgentOutput        `json:"previous_outputs,omitempty"`
	ProjectRules    ProjectRulesEnvelope `json:"project_rules"`
}

type AgentOutput struct {
	Role         string               `json:"role"`
	AgentName    string               `json:"agent_name,omitempty"`
	Provider     string               `json:"provider"`
	Model        string               `json:"model"`
	SessionID    string               `json:"session_id"`
	Verdict      string               `json:"verdict,omitempty"`
	Confidence   float64              `json:"confidence"`
	Score        float64              `json:"score,omitempty"`
	Summary      string               `json:"summary"`
	Proposals    []Proposal           `json:"proposals,omitempty"`
	Risks        []Risk               `json:"risks,omitempty"`
	Questions    []Question           `json:"questions,omitempty"`
	Dependencies []Dependency         `json:"dependencies,omitempty"`
	Claims       []Claim              `json:"claims,omitempty"`
	ProjectRules ProjectRulesEnvelope `json:"project_rules,omitempty"`
	TaskGraph    *TaskGraph           `json:"task_graph,omitempty"`
	Metadata     map[string]any       `json:"metadata,omitempty"`
}

type Proposal struct {
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Files       []string `json:"files,omitempty"`
	OwnerRole   string   `json:"owner_role,omitempty"`
	Confidence  float64  `json:"confidence,omitempty"`
	SourceAgent string   `json:"source_agent,omitempty"`
}

type Risk struct {
	Title      string `json:"title"`
	Impact     string `json:"impact"`
	Mitigation string `json:"mitigation,omitempty"`
	Severity   string `json:"severity,omitempty"`
}

type Question struct {
	Question string `json:"question"`
	Impact   string `json:"impact"`
	Material bool   `json:"material"`
}

type Dependency struct {
	Before string `json:"before"`
	After  string `json:"after"`
	Reason string `json:"reason,omitempty"`
}

type Claim struct {
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

type Evaluation struct {
	Verdict        string      `json:"verdict"`
	Score          float64     `json:"score,omitempty"`
	NeedsUserInput bool        `json:"needs_user_input"`
	Questions      []Question  `json:"questions,omitempty"`
	Issues         []string    `json:"issues,omitempty"`
	Summary        string      `json:"summary"`
	Raw            AgentOutput `json:"raw"`
}

type TaskGraph struct {
	Version int        `json:"version"`
	Tasks   []TaskSpec `json:"tasks"`
}

type TaskSpec struct {
	ID                   string   `json:"id"`
	Repo                 string   `json:"repo"`
	Kind                 string   `json:"kind,omitempty"`
	Title                string   `json:"title"`
	Objective            string   `json:"objective"`
	Context              []string `json:"context,omitempty"`
	WriteScope           []string `json:"write_scope"`
	DependsOn            []string `json:"depends_on,omitempty"`
	Wave                 int      `json:"wave"`
	ParallelGroup        string   `json:"parallel_group,omitempty"`
	OwnerRole            string   `json:"owner_role,omitempty"`
	AcceptanceCriteria   []string `json:"acceptance_criteria"`
	VerificationCommands []string `json:"verification_commands"`
	RiskNotes            []string `json:"risk_notes,omitempty"`
}
