package qa

import (
	"time"

	"github.com/loldinis/codedungeon/internal/db"
)

type Entrypoint string

const (
	EntrypointStandalone Entrypoint = "standalone"
	EntrypointWorkflow   Entrypoint = "workflow"
)

type Mode string

const (
	ModeAuto        Mode = "auto"
	ModeVerify      Mode = "verify"
	ModeUnit        Mode = "unit"
	ModeIntegration Mode = "integration"
	ModeAPI         Mode = "api"
	ModeE2E         Mode = "e2e"
	ModeFull        Mode = "full"
)

type Status string

const (
	StatusPass    Status = "PASS"
	StatusFail    Status = "FAIL"
	StatusBlocked Status = "BLOCKED"
	StatusSkipped Status = "SKIPPED"
)

type DependencyStatus string

const (
	DependencyPresent  DependencyStatus = "present"
	DependencyMissing  DependencyStatus = "missing"
	DependencyBlocked  DependencyStatus = "blocked"
	DependencyOptional DependencyStatus = "optional"
)

type CheckKind string

const (
	CheckCommand    CheckKind = "command"
	CheckPlaywright CheckKind = "playwright"
)

type DependencyMode string

const (
	DependencyStrict     DependencyMode = "strict"
	DependencyBestEffort DependencyMode = "best-effort"
	DependencyReportOnly DependencyMode = "report-only"
)

type Request struct {
	Root           string         `json:"root"`
	RunID          int64          `json:"run_id,omitempty"`
	ExecutionID    string         `json:"execution_id,omitempty"`
	Entrypoint     Entrypoint     `json:"entrypoint"`
	Mode           Mode           `json:"mode"`
	Phase          string         `json:"phase,omitempty"`
	PlanPath       string         `json:"plan_path,omitempty"`
	ProjectContext string         `json:"project_context,omitempty"`
	TaskContext    string         `json:"task_context,omitempty"`
	ChangedFiles   []string       `json:"changed_files,omitempty"`
	Commands       []CommandSpec  `json:"commands,omitempty"`
	DependencyMode DependencyMode `json:"dependency_mode,omitempty"`
	OutputDir      string         `json:"output_dir,omitempty"`
	Fresh          bool           `json:"fresh,omitempty"`
	TimeoutSeconds int            `json:"timeout_seconds,omitempty"`
	Store          *db.Store      `json:"-"`
}

type Result struct {
	SessionID    string             `json:"session_id"`
	Status       Status             `json:"status"`
	StartedAt    time.Time          `json:"started_at"`
	FinishedAt   time.Time          `json:"finished_at"`
	EvidenceDir  string             `json:"evidence_dir"`
	Checks       []CheckResult      `json:"checks,omitempty"`
	Dependencies []DependencyResult `json:"dependencies,omitempty"`
	Findings     []Finding          `json:"findings,omitempty"`
	FixTasks     []FixTask          `json:"fix_tasks,omitempty"`
	SummaryPath  string             `json:"summary_path"`
	ResultPath   string             `json:"result_path"`
	Error        string             `json:"error,omitempty"`
}

type CommandSpec struct {
	ID             string    `json:"id"`
	Kind           CheckKind `json:"kind"`
	Name           string    `json:"name,omitempty"`
	Command        string    `json:"cmd,omitempty"`
	CWD            string    `json:"cwd,omitempty"`
	Required       bool      `json:"required"`
	TimeoutSeconds int       `json:"timeout_seconds,omitempty"`
}

type CheckResult struct {
	ID         string    `json:"id"`
	Kind       CheckKind `json:"kind"`
	Name       string    `json:"name,omitempty"`
	Command    string    `json:"cmd,omitempty"`
	CWD        string    `json:"cwd,omitempty"`
	Status     Status    `json:"status"`
	ExitCode   int       `json:"exit_code,omitempty"`
	DurationMs int64     `json:"duration_ms"`
	LogPath    string    `json:"log_path,omitempty"`
	ReportPath string    `json:"report_path,omitempty"`
	Artifacts  []string  `json:"artifacts,omitempty"`
	Error      string    `json:"error,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

type DependencyResult struct {
	Name        string           `json:"name"`
	Required    bool             `json:"required"`
	Status      DependencyStatus `json:"status"`
	Version     string           `json:"version,omitempty"`
	InstallHint string           `json:"install_hint,omitempty"`
	Detail      string           `json:"detail,omitempty"`
}

type Finding struct {
	Severity    string   `json:"severity"`
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Detail      string   `json:"detail,omitempty"`
	Evidence    []string `json:"evidence,omitempty"`
	FixTaskPath string   `json:"fix_task_path,omitempty"`
}

type FixTask struct {
	Version             int      `json:"version"`
	Source              string   `json:"source"`
	QASessionID         string   `json:"qa_session_id"`
	CheckID             string   `json:"check_id"`
	Category            string   `json:"category"`
	Title               string   `json:"title"`
	Evidence            []string `json:"evidence"`
	SuggestedEntrypoint string   `json:"suggested_entrypoint,omitempty"`
	Path                string   `json:"path,omitempty"`
}

type FrameworkComponent struct {
	Path       string `json:"path"`
	Lang       string `json:"lang"`
	Framework  string `json:"framework"`
	Config     string `json:"config,omitempty"`
	RunCommand string `json:"run_cmd,omitempty"`
}

type FrameworkResult struct {
	OK          bool                 `json:"ok"`
	Path        string               `json:"path"`
	Lang        string               `json:"lang"`
	Framework   string               `json:"framework"`
	Config      string               `json:"config,omitempty"`
	RunCommand  string               `json:"run_cmd,omitempty"`
	Components  []FrameworkComponent `json:"components,omitempty"`
	RunCommands []string             `json:"run_cmds,omitempty"`
}
