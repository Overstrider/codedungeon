package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
)

func TraceCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "trace",
		Short: "Record CodeDungeon agent telemetry",
	}
	c.AddCommand(traceAgentStartCmd())
	c.AddCommand(traceAgentEndCmd())
	return c
}

func traceAgentStartCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "agent-start",
		Short: "Record the start of a phase, worker, or review agent",
		RunE: func(c *cobra.Command, _ []string) error {
			s, run, err := openTraceStore(c, "trace agent-start")
			if err != nil {
				return err
			}
			defer s.Close()
			phase, _ := c.Flags().GetString("phase")
			role, _ := c.Flags().GetString("role")
			agentType, _ := c.Flags().GetString("agent-type")
			agentName, _ := c.Flags().GetString("agent-name")
			model, _ := c.Flags().GetString("model")
			effort, _ := c.Flags().GetString("reasoning-effort")
			taskPath, _ := c.Flags().GetString("task")
			inputSummary, _ := c.Flags().GetString("input-summary")
			role = strings.TrimSpace(role)
			if role == "" {
				return EmitErr("--role is required", "")
			}
			id, err := s.StartAgentRun(db.AgentRun{
				RunID:           run.ID,
				SessionID:       os.Getenv(envSessionID),
				Phase:           phase,
				Role:            role,
				AgentType:       agentType,
				AgentName:       agentName,
				Model:           model,
				ReasoningEffort: effort,
				TaskPath:        taskPath,
				InputSummary:    inputSummary,
			})
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			_, _ = s.InsertAgentEvent(db.AgentEvent{
				RunID:      run.ID,
				AgentRunID: id,
				SessionID:  os.Getenv(envSessionID),
				Phase:      phase,
				Event:      "agent_started",
				Detail:     fmt.Sprintf("%s %s", role, agentType),
			})
			return EmitJSON(map[string]any{"ok": true, "agent_run_id": id})
		},
	}
	c.Flags().String("phase", "", "CodeDungeon phase number, such as 5 or 5.5")
	c.Flags().String("role", "", "agent role, such as phase-agent, dev-worker, reviewer, validator")
	c.Flags().String("agent-type", "", "Codex agent_type or provider persona type")
	c.Flags().String("agent-name", "", "human-friendly agent name")
	c.Flags().String("model", "", "model used for this agent")
	c.Flags().String("reasoning-effort", "", "Codex reasoning effort")
	c.Flags().String("task", "", "task or artifact path associated with this agent")
	c.Flags().String("input-summary", "", "short summary of the work delegated to this agent")
	return c
}

func traceAgentEndCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "agent-end",
		Short: "Record the terminal status of a phase, worker, or review agent",
		RunE: func(c *cobra.Command, _ []string) error {
			s, run, err := openTraceStore(c, "trace agent-end")
			if err != nil {
				return err
			}
			defer s.Close()
			idRaw, _ := c.Flags().GetString("id")
			id, err := strconv.ParseInt(strings.TrimSpace(idRaw), 10, 64)
			if err != nil || id <= 0 {
				return EmitErr("--id must be a positive integer", "")
			}
			status, _ := c.Flags().GetString("status")
			status = strings.ToUpper(strings.TrimSpace(status))
			if !validAgentStatus(status) || status == "RUNNING" {
				return EmitErr("--status must be COMPLETED, FAILED, or ABORTED", "")
			}
			summary, _ := c.Flags().GetString("summary")
			artifact, _ := c.Flags().GetString("artifact")
			errorMessage, _ := c.Flags().GetString("error")
			if err := s.FinishAgentRun(id, status, summary, artifact, errorMessage); err != nil {
				return EmitErr(err.Error(), "")
			}
			_, _ = s.InsertAgentEvent(db.AgentEvent{
				RunID:      run.ID,
				AgentRunID: id,
				SessionID:  os.Getenv(envSessionID),
				Event:      "agent_" + strings.ToLower(status),
				Detail:     firstNonEmpty(errorMessage, summary, artifact),
			})
			return EmitJSON(map[string]any{"ok": true, "agent_run_id": id, "status": status})
		},
	}
	c.Flags().String("id", "", "agent run id returned by trace agent-start")
	c.Flags().String("status", "COMPLETED", "COMPLETED | FAILED | ABORTED")
	c.Flags().String("summary", "", "short result summary")
	c.Flags().String("artifact", "", "primary artifact path")
	c.Flags().String("error", "", "error or blocker message for failed/aborted agents")
	return c
}

func openTraceStore(c *cobra.Command, action string) (*db.Store, *db.Run, error) {
	s, err := OpenDB(c)
	if err != nil {
		return nil, nil, EmitErr(err.Error(), "")
	}
	if err := s.Init(); err != nil {
		s.Close()
		return nil, nil, EmitErr(err.Error(), "")
	}
	run, err := s.CurrentRun()
	if err != nil {
		s.Close()
		return nil, nil, EmitErr(err.Error(), "")
	}
	if run == nil {
		s.Close()
		return nil, nil, EmitErr("no active run", "")
	}
	if err := requireAutonomousCustody(s, run.ID, action); err != nil {
		s.Close()
		return nil, nil, err
	}
	return s, run, nil
}

func validAgentStatus(status string) bool {
	switch status {
	case "RUNNING", "COMPLETED", "FAILED", "ABORTED":
		return true
	default:
		return false
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
