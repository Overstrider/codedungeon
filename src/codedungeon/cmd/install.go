package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/prompts"
)

// InstallCmd: walk embedded tree → write to <project>/.claude/<relpath>.
// Flags: --force (overwrite user-modified files), --dry-run (list only).
// Records each write in `installed_artifacts`. Sprint 7 Stage 4.
func InstallCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "install",
		Short: "Install embedded agents/skills/commands/phases into <project>/.claude/",
		RunE: func(c *cobra.Command, _ []string) error {
			return runInstall(c)
		},
	}
	c.Flags().Bool("force", false, "overwrite user-modified files")
	c.Flags().Bool("dry-run", false, "list what would change, don't write")
	return c
}

// MigrateCmd: compare binary.Version vs meta.cd_version; if different,
// re-install + update cd_version. Preserves user-modified (warn).
func MigrateCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "migrate",
		Short: "Compare cd_version; re-install artifacts if binary is newer",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			if err := s.Migrate(); err != nil {
				return EmitErr(err.Error(), "")
			}
			cur, _ := s.GetMeta("cd_version")
			target := versionString()
			if cur == target {
				return EmitJSON(map[string]any{
					"ok":      true,
					"message": "already at " + target,
					"from":    cur,
					"to":      target,
				})
			}
			// Re-run install (force off — user-modified stays).
			if err := runInstallWith(c, s, false, false); err != nil {
				return err
			}
			_ = s.SetMeta("cd_version", target)
			return EmitJSON(map[string]any{
				"ok":   true,
				"from": cur,
				"to":   target,
			})
		},
	}
	return c
}

// StatusCmd: lists installed artifacts + their status (synced | user-modified | stale).
func StatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report installed artifacts + drift vs embedded",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			arts, err := s.ListArtifacts()
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			embedded, _ := prompts.Artifacts()
			embSHA := map[string]string{}
			for _, a := range embedded {
				embSHA[a.RelPath] = sha256Hex(a.Content)
			}

			rows := []map[string]any{}
			cwd, _ := os.Getwd()
			root := ResolveProjectRoot(cwd)
			for _, a := range arts {
				status := "synced"
				disk := filepath.Join(root, ".claude", a.RelPath)
				if data, err := os.ReadFile(disk); err == nil {
					diskSHA := sha256Hex(data)
					if diskSHA != a.SHA256 {
						status = "user-modified"
					} else if ebS, ok := embSHA[a.RelPath]; ok && ebS != a.SHA256 {
						status = "stale"
					}
				} else {
					status = "missing"
				}
				rows = append(rows, map[string]any{
					"path":           a.RelPath,
					"status":         status,
					"binary_version": a.BinaryVersion,
				})
			}
			return EmitJSON(map[string]any{
				"ok":       true,
				"count":    len(rows),
				"artifacts": rows,
			})
		},
	}
}

// ---- helpers ----

func runInstall(c *cobra.Command) error {
	force, _ := c.Flags().GetBool("force")
	dry, _ := c.Flags().GetBool("dry-run")
	s, err := OpenDB(c)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	defer s.Close()
	return runInstallWith(c, s, force, dry)
}

func runInstallWith(c *cobra.Command, s *db.Store, force, dry bool) error {
	cwd, _ := os.Getwd()
	root := ResolveProjectRoot(cwd)
	embedded, err := prompts.Artifacts()
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	var wrote, skipped, forced []string
	for _, a := range embedded {
		disk := filepath.Join(root, ".claude", a.RelPath)
		embSHA := sha256Hex(a.Content)

		// Detect user modification: disk SHA != DB-recorded SHA.
		userModified := false
		if existing, _ := s.GetArtifact(a.RelPath); existing != nil {
			if data, err := os.ReadFile(disk); err == nil {
				if sha256Hex(data) != existing.SHA256 {
					userModified = true
				}
			}
		}
		if userModified && !force {
			skipped = append(skipped, a.RelPath)
			continue
		}
		if userModified && force {
			forced = append(forced, a.RelPath)
		}
		if dry {
			wrote = append(wrote, "DRY:"+a.RelPath)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(disk), 0o755); err != nil {
			return EmitErr(err.Error(), "")
		}
		if err := os.WriteFile(disk, a.Content, 0o644); err != nil {
			return EmitErr(err.Error(), "")
		}
		_ = s.UpsertArtifact(db.InstalledArtifact{
			RelPath:       a.RelPath,
			SHA256:        embSHA,
			BinaryVersion: versionString(),
			UserModified:  false,
			InstalledAt:   time.Now().Unix(),
		})
		wrote = append(wrote, a.RelPath)
	}
	return EmitJSON(map[string]any{
		"ok":      true,
		"mode":    modeLbl(dry),
		"wrote":   len(wrote),
		"skipped": len(skipped),
		"forced":  len(forced),
		"skipped_paths": skipped,
	})
}

func modeLbl(dry bool) string {
	if dry {
		return "dry-run"
	}
	return "install"
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Used by main.go to wire SetVersion flow through.
var _ = fmt.Sprintf
