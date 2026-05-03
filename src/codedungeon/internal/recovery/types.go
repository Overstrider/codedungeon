package recovery

import "github.com/loldinis/codedungeon/internal/db"

type AbortOptions struct {
	IncludeAutonomousRunner bool
	ExcludeAgentID          int64
	Summary                 string
}

type AgentAbortOptions struct {
	SessionID               string
	ExcludeAgentID          int64
	IncludeAutonomousRunner bool
	Summary                 string
	ErrorMessage            string
}

type RunRecoveryReport struct {
	Status                 string         `json:"status"`
	EffectiveStatus        string         `json:"effective_status,omitempty"`
	SessionID              string         `json:"session_id,omitempty"`
	RecoveredFromSessionID string         `json:"recovered_from_session_id,omitempty"`
	Session                *db.RunSession `json:"session,omitempty"`
	Active                 bool           `json:"active"`
	Stale                  bool           `json:"stale"`
	Recovered              bool           `json:"recovered,omitempty"`
	FailureKind            string         `json:"failure_kind,omitempty"`
	BlockedReason          string         `json:"blocked_reason,omitempty"`
	ResumeCommand          string         `json:"resume_command,omitempty"`
	AgeSeconds             int64          `json:"age_seconds,omitempty"`
	OpenAgents             int            `json:"open_agents,omitempty"`
	AbortedAgents          int            `json:"aborted_agents,omitempty"`
	NextCommands           []string       `json:"next_commands,omitempty"`
	CleanupHints           []CleanupHint  `json:"cleanup_hints,omitempty"`
	FailureMessages        []string       `json:"failure_messages,omitempty"`
	RecoveredFailures      []string       `json:"recovered_failures,omitempty"`
}

type ExecutionRecoveryReport struct {
	SessionID        string               `json:"session_id"`
	Status           string               `json:"status"`
	Session          *db.ExecutionSession `json:"session,omitempty"`
	Expired          bool                 `json:"expired"`
	Stale            bool                 `json:"stale"`
	AgeSeconds       int64                `json:"age_seconds,omitempty"`
	ExpiresInSeconds int64                `json:"expires_in_seconds,omitempty"`
	RollbackHints    []RollbackHint       `json:"rollback_hints,omitempty"`
	CleanupHints     []CleanupHint        `json:"cleanup_hints,omitempty"`
	NextCommands     []string             `json:"next_commands,omitempty"`
}

type RollbackHint struct {
	Name            string `json:"name"`
	SessionID       string `json:"session_id"`
	Attempt         int    `json:"attempt,omitempty"`
	Target          string `json:"target"`
	BackupRef       string `json:"backup_ref,omitempty"`
	Command         string `json:"command"`
	RequiresConfirm bool   `json:"requires_confirm"`
	Destructive     bool   `json:"destructive"`
}

type RollbackPlan = RollbackHint

type CleanupHint struct {
	Kind         string `json:"kind"`
	SessionID    string `json:"session_id,omitempty"`
	Status       string `json:"status,omitempty"`
	Path         string `json:"path"`
	SafeToDelete bool   `json:"safe_to_delete"`
	Reason       string `json:"reason"`
	Command      string `json:"command,omitempty"`
}
