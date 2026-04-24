package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/manifest"
)

// excludedDirs are skipped when scanning for sub-repos.
var excludedDirs = map[string]bool{
	".claude": true, ".git": true, "node_modules": true, ".next": true,
	"target": true, "build": true, ".gradle": true, ".venv": true, "venv": true,
	"dist": true, "docs": true, "scripts": true, "vendor": true, ".cache": true,
	".idea": true, ".vscode": true, "__pycache__": true, ".pytest_cache": true,
	"coverage": true, ".DS_Store": true,
}

// RepoEntry is one row of REPO_MAP.
type RepoEntry struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Lang          string `json:"lang"`
	Framework     string `json:"framework,omitempty"`
	Stack         string `json:"stack"`
	Specialist    string `json:"specialist"`
	DomainPlanner string `json:"domain_planner"`
	Manifest      string `json:"manifest,omitempty"`
	HasSource     bool   `json:"has_source"`
}

func RepoCmd() *cobra.Command {
	c := &cobra.Command{Use: "repo", Short: "Discover and resolve repos in a project"}
	c.AddCommand(repoDiscoverCmd())
	c.AddCommand(repoResolveCmd())
	c.AddCommand(repoCheckTestAuthCmd())
	return c
}

func repoDiscoverCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "discover",
		Short: "Scan project for repos (single/multi/bootstrap auto-detect)",
		RunE: func(c *cobra.Command, _ []string) error {
			root, _ := c.Flags().GetString("root")
			if root == "" {
				root, _ = os.Getwd()
			}
			writeCM, _ := c.Flags().GetBool("write-claude-md")
			persist, _ := c.Flags().GetBool("persist")

			result, err := discover(root)
			if err != nil {
				return EmitErr(err.Error(), "")
			}

			// Optionally upsert CLAUDE.md.
			if writeCM && len(result.RepoMap) > 0 {
				if err := upsertRepositoriesTable(filepath.Join(root, "CLAUDE.md"), result.RepoMap); err != nil {
					// Non-fatal: log + continue.
					fmt.Fprintln(os.Stderr, "[WARN] CLAUDE.md upsert failed:", err)
				}
			}

			// Optionally persist to DB (if a run exists).
			if persist {
				s, err := OpenDB(c)
				if err == nil {
					defer s.Close()
					run, _ := s.CurrentRun()
					if run != nil {
						b, _ := json.Marshal(result.RepoMap)
						_ = s.SetRunJSON(run.ID, "repo_map", b)
						_ = s.UpdateRunConfig(run.ID, "project_mode", result.ProjectMode)
					}
				}
			}
			return EmitJSON(result)
		},
	}
	c.Flags().String("root", "", "project root (default: cwd)")
	c.Flags().Bool("write-claude-md", false, "upsert '## Repositories' in CLAUDE.md at root")
	c.Flags().Bool("persist", true, "persist REPO_MAP into the active run (if any)")
	return c
}

// DiscoverResult is the final JSON of `codedungeon repo discover`.
type DiscoverResult struct {
	OK          bool        `json:"ok"`
	Root        string      `json:"root"`
	ProjectMode string      `json:"project_mode"` // SINGLE | MULTI | BOOTSTRAP
	RepoMap     []RepoEntry `json:"repo_map"`
}

func discover(root string) (*DiscoverResult, error) {
	rootInfo, err := manifest.Detect(root)
	if err != nil {
		return nil, err
	}

	// Scan subdirs for sub-repos.
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var subRepos []RepoEntry
	for _, e := range entries {
		if !e.IsDir() || excludedDirs[e.Name()] || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		subPath := filepath.Join(root, e.Name())
		info, err := manifest.Detect(subPath)
		if err != nil {
			continue
		}
		if info.Lang == "unknown" || !info.HasSource {
			continue
		}
		subRepos = append(subRepos, makeEntry(e.Name(), subPath, info))
	}

	// Classify: bootstrap / single / multi.
	if rootInfo.Lang == "unknown" && len(subRepos) == 0 {
		return &DiscoverResult{OK: true, Root: root, ProjectMode: "BOOTSTRAP", RepoMap: []RepoEntry{}}, nil
	}
	if rootInfo.Lang != "unknown" && len(subRepos) == 0 {
		return &DiscoverResult{
			OK: true, Root: root, ProjectMode: "SINGLE",
			RepoMap: []RepoEntry{makeEntry(".", root, rootInfo)},
		}, nil
	}
	// Multi: prefer subRepos; if root also has a manifest, include it too.
	if rootInfo.Lang != "unknown" && rootInfo.HasSource {
		// Prepend root as "." entry.
		subRepos = append([]RepoEntry{makeEntry(".", root, rootInfo)}, subRepos...)
	}
	// Stable order by name.
	sort.SliceStable(subRepos, func(i, j int) bool { return subRepos[i].Name < subRepos[j].Name })
	return &DiscoverResult{OK: true, Root: root, ProjectMode: "MULTI", RepoMap: subRepos}, nil
}

func makeEntry(name, path string, info manifest.Info) RepoEntry {
	return RepoEntry{
		Name:          name,
		Path:          path,
		Lang:          info.Lang,
		Framework:     info.Framework,
		Stack:         info.Stack,
		Specialist:    info.Lang + "-specialist",
		DomainPlanner: pickDomainPlanner(name, info.Lang),
		Manifest:      info.Manifest,
		HasSource:     info.HasSource,
	}
}

// pickDomainPlanner applies the repo-naming convention.
func pickDomainPlanner(repoName, lang string) string {
	switch repoName {
	case ".":
		return "planner"
	case "backend", "api", "server":
		return "backend-planner"
	case "frontend", "portal", "web":
		return "frontend-planner"
	case "app", "mobile":
		return "app-planner"
	}
	// Fallback: use the repo name itself.
	return repoName + "-planner"
}

func repoResolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <name>",
		Short: "Lookup a repo by name from the active run's REPO_MAP",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "run `codedungeon db init` + `codedungeon phase init` first")
			}
			defer s.Close()
			run, err := s.CurrentRun()
			if err != nil || run == nil {
				return EmitErr("no active run", "`codedungeon phase init` first")
			}
			var entries []RepoEntry
			if len(run.RepoMap) > 0 {
				_ = json.Unmarshal(run.RepoMap, &entries)
			}
			for _, e := range entries {
				if e.Name == name {
					return EmitJSON(e)
				}
			}
			return EmitErr("repo not found: "+name, "known: "+joinNames(entries))
		},
	}
}

func joinNames(entries []RepoEntry) string {
	var out []string
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return strings.Join(out, ", ")
}

// repoCheckTestAuthCmd: grep `## Test Auth` in each repo's CLAUDE.md; report missing.
func repoCheckTestAuthCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "check-test-auth",
		Short: "Check each repo's CLAUDE.md for '## Test Auth' section; return missing list",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			run, err := s.CurrentRun()
			if err != nil || run == nil {
				return EmitErr("no active run", "")
			}
			var entries []RepoEntry
			_ = json.Unmarshal(run.RepoMap, &entries)
			var missing []string
			var present []string
			for _, e := range entries {
				// Skip langs that don't have test auth (mobile/app etc.).
				if e.Lang == "kotlin" || e.Lang == "swift" {
					continue
				}
				cm := filepath.Join(e.Path, "CLAUDE.md")
				b, err := os.ReadFile(cm)
				if err != nil {
					missing = append(missing, e.Name)
					continue
				}
				if strings.Contains(string(b), "## Test Auth") {
					present = append(present, e.Name)
				} else {
					missing = append(missing, e.Name)
				}
			}
			return EmitJSON(map[string]any{
				"ok":      true,
				"missing": missing,
				"present": present,
				"spec":    "run `codedungeon prompts get test-auth-spec` for the full TEST_AUTH_SPEC",
			})
		},
	}
	return c
}

// upsertRepositoriesTable writes or replaces the `## Repositories` section
// in the given CLAUDE.md path. If the file doesn't exist, it's created.
// Preserves all other sections.
func upsertRepositoriesTable(path string, repos []RepoEntry) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	table := renderRepoTable(repos)

	// If file absent → seed.
	if len(existing) == 0 {
		body := "# Project CLAUDE.md\n\n" + table + "\n"
		return os.WriteFile(path, []byte(body), 0o644)
	}

	// Replace existing `## Repositories` section (until next `## ` or EOF).
	content := string(existing)
	idx := strings.Index(content, "\n## Repositories")
	if idx < 0 {
		// Append to end.
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n" + table + "\n"
	} else {
		before := content[:idx+1]
		rest := content[idx+1:]
		// Find next "## " after this section header.
		next := strings.Index(rest[len("## Repositories"):], "\n## ")
		var after string
		if next < 0 {
			after = ""
		} else {
			after = rest[len("## Repositories")+next:]
		}
		content = before + table + after
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func renderRepoTable(repos []RepoEntry) string {
	var b strings.Builder
	b.WriteString("## Repositories\n\n")
	b.WriteString("| Repo | Stack | Lang | Specialist | Domain Planner |\n")
	b.WriteString("|------|-------|------|------------|----------------|\n")
	for _, r := range repos {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			r.Name, r.Stack, r.Lang, r.Specialist, r.DomainPlanner))
	}
	return b.String()
}
