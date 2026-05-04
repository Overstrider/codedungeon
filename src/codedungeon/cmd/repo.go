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
	"github.com/loldinis/codedungeon/internal/provider"
)

// excludedDirs are skipped when scanning for sub-repos.
var excludedDirs = map[string]bool{
	provider.Detect().ConfigDir(): true, ".git": true, "node_modules": true, ".next": true,
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

type agentConfigInstruction struct {
	Path    string `json:"path"`
	Section string `json:"section"`
	Mode    string `json:"mode"`
	Content string `json:"content"`
}

func codedungeonAgentConfigInstruction() agentConfigInstruction {
	return agentConfigInstruction{
		Path:    provider.Detect().AgentConfigFile(),
		Section: "## codedungeon",
		Mode:    "append_or_replace_section",
		Content: renderCodedungeonSection(),
	}
}

func repositoriesAgentConfigInstruction(repos []RepoEntry) agentConfigInstruction {
	return agentConfigInstruction{
		Path:    provider.Detect().AgentConfigFile(),
		Section: "## Repositories",
		Mode:    "append_or_replace_section",
		Content: renderRepoTable(repos),
	}
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
			writeAgentConfig, _ := c.Flags().GetBool("write-agent-config")
			writeClaudeMD, _ := c.Flags().GetBool("write-claude-md")
			persist, _ := c.Flags().GetBool("persist")

			result, err := discover(root)
			if err != nil {
				return EmitErr(err.Error(), "")
			}

			// Optionally return provider instruction guidance. CodeDungeon does
			// not mutate AGENTS.md/CLAUDE.md during install/discovery.
			if (writeAgentConfig || writeClaudeMD) && len(result.RepoMap) > 0 {
				instruction := repositoriesAgentConfigInstruction(result.RepoMap)
				result.AgentConfigInstruction = &instruction
			}

			// Optionally persist to DB (if a run exists).
			if persist {
				s, err := OpenDB(c)
				if err != nil && isMigrationRequired(err) {
					return EmitErr(err.Error(), "run: codedungeon migrate")
				}
				if err != nil {
					return EmitErr(err.Error(), "")
				}
				defer s.Close()
				run, _ := s.CurrentRun()
				if run != nil {
					if err := requireAutonomousCustody(s, run.ID, "repo map persistence"); err != nil {
						return err
					}
					b, _ := json.Marshal(result.RepoMap)
					_ = s.SetRunJSON(run.ID, "repo_map", b)
					_ = s.UpdateRunConfig(run.ID, "project_mode", result.ProjectMode)
				}
			}
			return EmitJSON(result)
		},
	}
	c.Flags().String("root", "", "project root (default: cwd)")
	c.Flags().Bool("write-agent-config", false, "include '## Repositories' provider instruction guidance in JSON output")
	c.Flags().Bool("write-claude-md", false, "deprecated alias for --write-agent-config")
	c.Flags().Bool("persist", true, "persist REPO_MAP into the active run (if any)")
	return c
}

// DiscoverResult is the final JSON of `codedungeon repo discover`.
type DiscoverResult struct {
	OK                     bool                    `json:"ok"`
	Root                   string                  `json:"root"`
	ProjectMode            string                  `json:"project_mode"` // SINGLE | MULTI | BOOTSTRAP
	RepoMap                []RepoEntry             `json:"repo_map"`
	AgentConfigInstruction *agentConfigInstruction `json:"agent_config_instruction,omitempty"`
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
	if rootInfo.Lang == "unknown" && len(subRepos) > 0 && isGitRoot(root) {
		return &DiscoverResult{
			OK: true, Root: root, ProjectMode: "SINGLE",
			RepoMap: []RepoEntry{makeMonorepoEntry(root, subRepos)},
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

func isGitRoot(root string) bool {
	if st, err := os.Stat(filepath.Join(root, ".git")); err == nil && st.IsDir() {
		return true
	}
	out, _, err := run(root, "git", "rev-parse", "--show-toplevel")
	return err == nil && samePath(strings.TrimSpace(out), root)
}

func makeMonorepoEntry(root string, subRepos []RepoEntry) RepoEntry {
	var stacks []string
	var manifests []string
	seenStack := map[string]bool{}
	for _, repo := range subRepos {
		if repo.Stack != "" && !seenStack[repo.Stack] {
			stacks = append(stacks, repo.Stack)
			seenStack[repo.Stack] = true
		}
		if repo.Manifest != "" {
			manifests = append(manifests, filepath.ToSlash(filepath.Join(repo.Name, repo.Manifest)))
		}
	}
	return RepoEntry{
		Name:          inferGitRepoName(root),
		Path:          ".",
		Lang:          "monorepo",
		Framework:     "monorepo",
		Stack:         strings.Join(stacks, " + "),
		Specialist:    "fullstack-specialist",
		DomainPlanner: "fullstack-planner",
		Manifest:      strings.Join(manifests, " + "),
		HasSource:     true,
	}
}

func inferGitRepoName(root string) string {
	if out, _, err := run(root, "git", "config", "--get", "remote.origin.url"); err == nil {
		if name := repoNameFromRemote(strings.TrimSpace(out)); name != "" {
			return name
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return filepath.Base(root)
	}
	return filepath.Base(abs)
}

func repoNameFromRemote(remote string) string {
	remote = strings.TrimSuffix(strings.TrimSpace(remote), ".git")
	remote = strings.TrimRight(remote, "/")
	if remote == "" {
		return ""
	}
	idx := strings.LastIndexAny(remote, "/:")
	if idx < 0 || idx == len(remote)-1 {
		return remote
	}
	return remote[idx+1:]
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
				cm := filepath.Join(e.Path, provider.Detect().AgentConfigFile())
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

// upsertRepositoriesTable is retained for old tests/compatibility helpers.
// Install and discovery flows now return agent_config_instruction instead of
// mutating AGENTS.md/CLAUDE.md.
func upsertRepositoriesTable(path string, repos []RepoEntry) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	table := renderRepoTable(repos)

	// If file absent → seed.
	if len(existing) == 0 {
		body := "# Project " + provider.Detect().AgentConfigFile() + "\n\n" + table + "\n"
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

// upsertCodedungeonSection is retained for old tests/compatibility helpers.
// Install and bootstrap flows now return agent_config_instruction instead of
// mutating AGENTS.md/CLAUDE.md.
func upsertCodedungeonSection(path string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	section := renderCodedungeonSection()

	if len(existing) == 0 {
		body := "# Project " + provider.Detect().AgentConfigFile() + "\n\n" + section + "\n"
		return os.WriteFile(path, []byte(body), 0o644)
	}

	content := string(existing)
	const header = "\n## codedungeon"
	idx := strings.Index(content, header)
	if idx < 0 {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n" + section + "\n"
	} else {
		before := content[:idx+1]
		rest := content[idx+1:]
		next := strings.Index(rest[len("## codedungeon"):], "\n## ")
		var after string
		if next < 0 {
			after = ""
		} else {
			after = rest[len("## codedungeon")+next:]
		}
		content = before + section + after
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func renderCodedungeonSection() string {
	var b strings.Builder
	b.WriteString("## codedungeon\n\n")
	p := provider.Detect()
	if p.Name() == "codex" {
		b.WriteString("Codex CLI pipeline available. Editable command playbooks live in `.codedungeon/commands/`.\n\n")
		b.WriteString("Promoted workflow surface: `$codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>`. Without a flag, `$codedungeon` selects automatically and prints `CODEDUNGEON_MODE_SELECTED: <mode> - <reason>` before dispatch. Run `$codedungeon --rules` before first real task to discover and approve project rules.\n\n")
		b.WriteString("| Skill | Use when |\n")
		b.WriteString("|-------|----------|\n")
		b.WriteString("| `$codedungeon --oneshot` | Small tasks: plan, code, PR, review; no task split. |\n")
		b.WriteString("| `$codedungeon --lite` | Simple planned tasks, single-repo. Requires `.codedungeon/plans/*.md`. |\n")
		b.WriteString("| `$codedungeon --full` | Complex features, multi-repo, full phase pipeline. |\n")
		b.WriteString("| `$codedungeon --rules` | Deep-read this repo, draft `.codedungeon/project-rules.md`, wait for user confirmation, then approve/compact rules. |\n")
		b.WriteString("| `code-review` | Standalone adversarial review on current branch. |\n")
		b.WriteString("\nCompatibility aliases remain installed: `$one-shot`, `$side-quest`, and `$main-quest`.\n")
		b.WriteString("\nProject Rules: workflows read `.codedungeon/project-rules.compact.md` when approved and include `PROJECT_RULES_STATUS`, `PROJECT_RULES_DIGEST`, and `PROJECT_RULES_READ` in handoffs.\n")
		b.WriteString(fmt.Sprintf("\nAgents in `%s/`, skills in `%s/`, commands/phases/mutable state in `.codedungeon/`. CLI binary at `%s/codedungeon`.\n", p.AgentsDir(), p.SkillsDir(), p.BinDir()))
		return b.String()
	}
	b.WriteString("CLI pipeline available. Commands:\n\n")
	b.WriteString("| Command | Use when |\n")
	b.WriteString("|---------|----------|\n")
	b.WriteString("| `/codedungeon --oneshot` | Small tasks: plan, code, PR, review; no task split. |\n")
	b.WriteString("| `/codedungeon --lite` | Simple planned tasks, single-repo. Requires `.codedungeon/plans/*.md`. |\n")
	b.WriteString("| `/codedungeon --full` | Complex features, multi-repo. Full 10-phase pipeline with architect, QA, tests, formal report. |\n")
	b.WriteString("| `/codedungeon --rules` | Deep-read this repo, draft `.codedungeon/project-rules.md`, wait for user confirmation, then approve/compact rules. |\n")
	b.WriteString("| `/code-review` | Standalone adversarial review on current branch. |\n")
	b.WriteString("\nWithout a flag, `/codedungeon` selects automatically and prints `CODEDUNGEON_MODE_SELECTED: <mode> - <reason>` before dispatch. Run `/codedungeon --rules` before first real task to discover and approve project rules. Compatibility aliases remain installed: `/one-shot`, `/side-quest`, and `/main-quest`.\n")
	b.WriteString("\nProject Rules: workflows read `.codedungeon/project-rules.compact.md` when approved and include `PROJECT_RULES_STATUS`, `PROJECT_RULES_DIGEST`, and `PROJECT_RULES_READ` in handoffs.\n")
	b.WriteString(fmt.Sprintf("\nSubagents and skills installed in `%s/`; editable commands, phases, and mutable state live in `.codedungeon/`. CLI binary at `%s/codedungeon`.\n", p.ConfigDir(), p.BinDir()))
	return b.String()
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
