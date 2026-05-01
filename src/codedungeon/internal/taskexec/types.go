package taskexec

import (
	"context"

	"github.com/loldinis/codedungeon/internal/taskplanning"
)

const (
	StatusRunning   = "RUNNING"
	StatusCompleted = "COMPLETED"
	StatusFailed    = "FAILED"
	StatusBlocked   = "BLOCKED"
	StatusDryRun    = "DRY_RUN"

	WorkerPassed           = "PASS"
	WorkerChangesRequested = "CHANGES_REQUESTED"
	WorkerBlocked          = "BLOCKED"
)

type Request struct {
	Root            string
	RunID           int64
	TaskPath        string
	ProjectContext  string
	WorkspacePolicy string
	SessionID       string
	ResumeID        string
	ResetSession    bool
	DryRun          bool
	Verbose         bool
	Config          Config
	Runner          Runner
	Git             GitClient
	Verifier        Verifier
}

type Result struct {
	OK                  bool                  `json:"ok"`
	SessionID           string                `json:"session_id"`
	Status              string                `json:"status"`
	Task                taskplanning.TaskSpec `json:"task"`
	OutputDir           string                `json:"output_dir"`
	Attempts            []AttemptResult       `json:"attempts"`
	ChangedFiles        []string              `json:"changed_files,omitempty"`
	VerificationResults []VerificationResult  `json:"verification_results,omitempty"`
	Artifacts           []string              `json:"artifacts,omitempty"`
	Metadata            map[string]any        `json:"metadata,omitempty"`
	FailureMessage      string                `json:"failure_message,omitempty"`
}

type AttemptResult struct {
	Attempt             int                  `json:"attempt"`
	ProviderSessionID   string               `json:"provider_session_id,omitempty"`
	HeadBefore          string               `json:"head_before,omitempty"`
	HeadAfter           string               `json:"head_after,omitempty"`
	BackupRef           string               `json:"backup_ref,omitempty"`
	DiffPath            string               `json:"diff_path,omitempty"`
	ChangedFiles        []string             `json:"changed_files,omitempty"`
	WorkerStatus        string               `json:"worker_status"`
	VerificationStatus  string               `json:"verification_status,omitempty"`
	Summary             string               `json:"summary,omitempty"`
	VerificationResults []VerificationResult `json:"verification_results,omitempty"`
	ErrorMessage        string               `json:"error_message,omitempty"`
}

type Runner interface {
	RunTask(ctx context.Context, req AgentRequest) (AgentResult, error)
}

type AgentRequest struct {
	SessionID       string                `json:"session_id"`
	Attempt         int                   `json:"attempt"`
	Root            string                `json:"root"`
	OutputDir       string                `json:"output_dir"`
	ResultPath      string                `json:"result_path"`
	ProjectContext  string                `json:"project_context"`
	WorkspacePolicy string                `json:"workspace_policy,omitempty"`
	Task            taskplanning.TaskSpec `json:"task"`
	Config          Config                `json:"config"`
}

type AgentResult struct {
	Status       string   `json:"status"`
	Summary      string   `json:"summary"`
	SessionID    string   `json:"session_id,omitempty"`
	ChangedFiles []string `json:"changed_files,omitempty"`
	Risks        []string `json:"risks,omitempty"`
}

type GitClient interface {
	Head(ctx context.Context, repo string) (string, error)
	CreateBackupRef(ctx context.Context, repo, name, head string) error
	ChangedFiles(ctx context.Context, repo string) ([]string, error)
	Diff(ctx context.Context, repo string) (string, error)
	Commit(ctx context.Context, repo, message string) error
	Push(ctx context.Context, repo string) error
	LatestSemverTag(ctx context.Context, repo string) (string, error)
	Tag(ctx context.Context, repo, tag, message string) error
}

type Verifier interface {
	Verify(ctx context.Context, req VerifyRequest) ([]VerificationResult, error)
}

type VerifyRequest struct {
	Root     string
	Repo     string
	Phase    string
	Commands []string
	Policy   SafetyPolicy
}

type VerificationResult struct {
	Command string `json:"command"`
	Status  string `json:"status"`
	LogPath string `json:"log_path,omitempty"`
	Error   string `json:"error,omitempty"`
}
