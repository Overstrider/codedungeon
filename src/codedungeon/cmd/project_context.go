package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/projectcontext"
)

func ProjectContextCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "project-context",
		Short: "Manage compact Project Context memory and audit ledger",
	}
	c.AddCommand(projectContextStatusCmd())
	c.AddCommand(projectContextInitCmd())
	c.AddCommand(projectContextApproveCmd())
	c.AddCommand(projectContextRejectCmd())
	c.AddCommand(projectContextAuditCmd())
	c.AddCommand(projectContextEnvelopeCmd())
	return c
}

func projectContextStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print Project Context status",
		RunE: func(c *cobra.Command, _ []string) error {
			root := currentProjectRoot()
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			if err := s.Init(); err != nil {
				return EmitErr(err.Error(), "")
			}
			status, err := projectcontext.Status(root, projectcontext.NewSQLiteStore(s))
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(status)
		},
	}
}

func projectContextInitCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "init",
		Short: "Create an initial Project Context proposal",
		RunE: func(c *cobra.Command, _ []string) error {
			root := currentProjectRoot()
			mode, _ := c.Flags().GetString("mode")
			firstPrompt, _ := c.Flags().GetString("first-prompt")
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			if err := s.Init(); err != nil {
				return EmitErr(err.Error(), "")
			}
			result, err := projectcontext.Init(root, projectcontext.NewSQLiteStore(s), projectcontext.InitOptions{
				Mode:        mode,
				FirstPrompt: firstPrompt,
			})
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(result)
		},
	}
	c.Flags().String("mode", projectcontext.ModeAuto, "auto, existing, or empty")
	c.Flags().String("first-prompt", "", "first user prompt for empty-project context")
	return c
}

func projectContextApproveCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "approve",
		Short: "Approve a pending Project Context proposal",
		RunE: func(c *cobra.Command, _ []string) error {
			root := currentProjectRoot()
			proposalID, _ := c.Flags().GetInt64("proposal")
			by, _ := c.Flags().GetString("by")
			if proposalID == 0 {
				return EmitErr("--proposal is required", "")
			}
			if by == "" {
				by = os.Getenv("USER")
				if by == "" {
					by = os.Getenv("USERNAME")
				}
				if by == "" {
					by = "unknown"
				}
			}
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			if err := s.Init(); err != nil {
				return EmitErr(err.Error(), "")
			}
			version, err := projectcontext.ApproveProposal(root, projectcontext.NewSQLiteStore(s), proposalID, by)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(version)
		},
	}
	c.Flags().Int64("proposal", 0, "proposal id to approve")
	c.Flags().String("by", "", "approver name")
	return c
}

func projectContextRejectCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "reject",
		Short: "Reject a pending Project Context proposal",
		RunE: func(c *cobra.Command, _ []string) error {
			root := currentProjectRoot()
			proposalID, _ := c.Flags().GetInt64("proposal")
			if proposalID == 0 {
				return EmitErr("--proposal is required", "")
			}
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			if err := s.Init(); err != nil {
				return EmitErr(err.Error(), "")
			}
			if err := projectcontext.RejectProposal(root, projectcontext.NewSQLiteStore(s), proposalID); err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{"ok": true, "proposal": proposalID, "status": projectcontext.StatusRejected})
		},
	}
	c.Flags().Int64("proposal", 0, "proposal id to reject")
	return c
}

func projectContextAuditCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "audit",
		Short: "Search Project Context audit records",
		RunE: func(c *cobra.Command, _ []string) error {
			query, _ := c.Flags().GetString("query")
			limit, _ := c.Flags().GetInt("limit")
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			if err := s.Init(); err != nil {
				return EmitErr(err.Error(), "")
			}
			records, err := projectcontext.NewSQLiteStore(s).AuditRecords(query, limit)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{"ok": true, "records": records})
		},
	}
	c.Flags().String("query", "", "audit query")
	c.Flags().Int("limit", 20, "maximum records")
	return c
}

func projectContextEnvelopeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "envelope",
		Short: "Print Project Rules and Project Context envelope",
		RunE: func(c *cobra.Command, _ []string) error {
			root := currentProjectRoot()
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			if err := s.Init(); err != nil {
				return EmitErr(err.Error(), "")
			}
			rulesStatus := ""
			rulesDigest := ""
			if st, err := computeProjectRulesStatus(root); err == nil {
				rulesStatus = st.Status
				rulesDigest = st.RulesDigest
			}
			envelope, err := projectcontext.BuildEnvelope(root, projectcontext.NewSQLiteStore(s), rulesStatus, rulesDigest)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(envelope)
		},
	}
}
