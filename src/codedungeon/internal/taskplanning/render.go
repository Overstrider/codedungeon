package taskplanning

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func RenderArtifacts(outputDir string, graph TaskGraph, rules ProjectRulesEnvelope) ([]string, error) {
	if err := ValidateTaskGraph(graph); err != nil {
		return nil, err
	}
	var artifacts []string
	masterPath := filepath.Join(outputDir, "MASTER.md")
	if err := os.WriteFile(masterPath, []byte(renderMaster(graph, rules)), 0o644); err != nil {
		return nil, err
	}
	artifacts = append(artifacts, masterPath)

	byRepo := map[string][]TaskSpec{}
	for _, task := range OrderedTasks(graph) {
		byRepo[task.Repo] = append(byRepo[task.Repo], task)
	}
	repos := make([]string, 0, len(byRepo))
	for repo := range byRepo {
		repos = append(repos, repo)
	}
	sort.Strings(repos)
	for _, repo := range repos {
		repoDir := filepath.Join(outputDir, "tasks", filepath.Clean(repo))
		if err := os.MkdirAll(repoDir, 0o755); err != nil {
			return nil, err
		}
		planPath := filepath.Join(repoDir, "PLAN.md")
		if err := os.WriteFile(planPath, []byte(renderRepoPlan(repo, byRepo[repo], rules)), 0o644); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, planPath)
		for _, task := range byRepo[repo] {
			taskPath := filepath.Join(repoDir, task.ID+".md")
			if err := os.WriteFile(taskPath, []byte(renderTask(task, rules)), 0o644); err != nil {
				return nil, err
			}
			artifacts = append(artifacts, taskPath)
			contractPath := filepath.Join(repoDir, task.ID+".json")
			if err := writeJSONFile(contractPath, task); err != nil {
				return nil, err
			}
			artifacts = append(artifacts, contractPath)
		}
	}
	return artifacts, nil
}

func renderMaster(graph TaskGraph, rules ProjectRulesEnvelope) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Task Planning Master")
	fmt.Fprintln(&b)
	renderRules(&b, rules)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| Wave | Task | Repo | Parallel Group | Depends On | Title |")
	fmt.Fprintln(&b, "|------|------|------|----------------|------------|-------|")
	for _, task := range OrderedTasks(graph) {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s |\n",
			task.Wave, task.ID, task.Repo, fallback(task.ParallelGroup, "-"), joinOrDash(task.DependsOn), task.Title)
	}
	return b.String()
}

func renderRepoPlan(repo string, tasks []TaskSpec, rules ProjectRulesEnvelope) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Repo: %s\n\n", repo)
	renderRules(&b, rules)
	fmt.Fprintln(&b)
	for _, task := range tasks {
		fmt.Fprintf(&b, "- [ ] %s wave %d: %s\n", task.ID, task.Wave, task.Title)
	}
	return b.String()
}

func renderTask(task TaskSpec, rules ProjectRulesEnvelope) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: %s\n\n", task.ID, task.Title)
	renderRules(&b, rules)
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Repo: %s\n", task.Repo)
	fmt.Fprintf(&b, "Kind: %s\n", fallback(task.Kind, "dev"))
	fmt.Fprintf(&b, "Wave: %d\n", task.Wave)
	fmt.Fprintf(&b, "Parallel Group: %s\n", fallback(task.ParallelGroup, "-"))
	fmt.Fprintf(&b, "Owner Role: %s\n", fallback(task.OwnerRole, "-"))
	fmt.Fprintf(&b, "Depends On: %s\n\n", joinOrDash(task.DependsOn))
	fmt.Fprintln(&b, "## Objective")
	fmt.Fprintf(&b, "%s\n\n", task.Objective)
	renderList(&b, "Context", task.Context)
	renderList(&b, "Write Scope", task.WriteScope)
	renderList(&b, "Acceptance Criteria", task.AcceptanceCriteria)
	renderList(&b, "Verification Commands", task.VerificationCommands)
	renderList(&b, "Risk Notes", task.RiskNotes)
	return b.String()
}

func renderRules(b *strings.Builder, rules ProjectRulesEnvelope) {
	fmt.Fprintf(b, "PROJECT_RULES_STATUS: %s\n", fallback(rules.Status, "missing"))
	fmt.Fprintf(b, "PROJECT_RULES_DIGEST: %s\n", fallback(rules.Digest, "none"))
	fmt.Fprintf(b, "PROJECT_RULES_READ: %s\n", fallback(rules.Read, "yes"))
}

func renderList(b *strings.Builder, title string, values []string) {
	values = nonEmpty(values)
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "## %s\n", title)
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
	fmt.Fprintln(b)
}

func joinOrDash(values []string) string {
	values = nonEmpty(values)
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ", ")
}

func fallback(value, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}
