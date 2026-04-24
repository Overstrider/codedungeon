package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/prompts"
)

func PromptsCmd() *cobra.Command {
	c := &cobra.Command{Use: "prompts", Short: "Read, list, version embedded prompts"}
	c.AddCommand(promptsGetCmd())
	c.AddCommand(promptsListCmd())
	c.AddCommand(promptsSetCmd())
	c.AddCommand(promptsDiffCmd())
	return c
}

func promptsGetCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "get <name>",
		Short: "Print prompt contents to stdout (DB-first, embedded fallback)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			version, _ := c.Flags().GetInt("version")
			s, err := OpenDB(c)
			if err == nil {
				defer s.Close()
				if version > 0 {
					p, err := s.GetPrompt(name, version)
					if err != nil {
						return EmitErr(err.Error(), "")
					}
					if p == nil {
						return EmitErr(fmt.Sprintf("prompt %s v%d not found", name, version), "")
					}
					_, _ = os.Stdout.WriteString(p.Content)
					return nil
				}
				p, err := s.LatestPrompt(name)
				if err == nil && p != nil {
					_, _ = os.Stdout.WriteString(p.Content)
					return nil
				}
			}
			// DB miss or error → embedded fallback.
			body, err := prompts.Get(name)
			if err != nil {
				return EmitErr(err.Error(), "run `codedungeon prompts list` for known names")
			}
			_, _ = os.Stdout.WriteString(body)
			return nil
		},
	}
	c.Flags().Int("version", 0, "specific version (default: latest)")
	return c
}

func promptsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List known prompts (DB versions + embedded names)",
		RunE: func(c *cobra.Command, _ []string) error {
			embedded, _ := prompts.List()
			out := map[string]any{"embedded": embedded}
			s, err := OpenDB(c)
			if err == nil {
				defer s.Close()
				dbList, err := s.ListPrompts()
				if err == nil {
					out["db_latest_versions"] = dbList
				}
			}
			return EmitJSON(out)
		},
	}
}

func promptsSetCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "set <name>",
		Short: "Write a new prompt version (from --file or stdin)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			file, _ := c.Flags().GetString("file")
			var content []byte
			var err error
			if file == "-" || file == "" {
				content, err = io.ReadAll(os.Stdin)
			} else {
				content, err = os.ReadFile(file)
			}
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "run `codedungeon db init` first")
			}
			defer s.Close()
			v, err := s.InsertPrompt(name, string(content), "user")
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{"ok": true, "name": name, "version": v})
		},
	}
	c.Flags().String("file", "", "file path, or '-' / empty for stdin")
	return c
}

func promptsDiffCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "diff <name>",
		Short: "Show line counts + SHA diff between two prompt versions",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			from, _ := c.Flags().GetInt("from")
			to, _ := c.Flags().GetInt("to")
			if from <= 0 || to <= 0 {
				return EmitErr("--from and --to required (>0)", "")
			}
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			a, err := s.GetPrompt(name, from)
			if err != nil || a == nil {
				return EmitErr(fmt.Sprintf("version %d not found", from), "")
			}
			b, err := s.GetPrompt(name, to)
			if err != nil || b == nil {
				return EmitErr(fmt.Sprintf("version %d not found", to), "")
			}
			return EmitJSON(map[string]any{
				"ok":         true,
				"name":       name,
				"from":       map[string]any{"version": a.Version, "sha256": a.SHA256, "bytes": len(a.Content)},
				"to":         map[string]any{"version": b.Version, "sha256": b.SHA256, "bytes": len(b.Content)},
				"equal":      a.SHA256 == b.SHA256,
				"delta_bytes": len(b.Content) - len(a.Content),
			})
		},
	}
	c.Flags().Int("from", 0, "from version")
	c.Flags().Int("to", 0, "to version")
	return c
}
