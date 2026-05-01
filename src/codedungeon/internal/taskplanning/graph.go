package taskplanning

import (
	"fmt"
	"sort"
	"strings"
)

func ValidateTaskGraph(graph TaskGraph) error {
	if graph.Version <= 0 {
		return fmt.Errorf("task graph version must be > 0")
	}
	if len(graph.Tasks) == 0 {
		return fmt.Errorf("task graph must contain at least one task")
	}
	byID := map[string]TaskSpec{}
	for i, task := range graph.Tasks {
		if strings.TrimSpace(task.ID) == "" {
			return fmt.Errorf("task %d missing id", i+1)
		}
		if _, exists := byID[task.ID]; exists {
			return fmt.Errorf("duplicate task id: %s", task.ID)
		}
		if strings.TrimSpace(task.Repo) == "" {
			return fmt.Errorf("task %s missing repo", task.ID)
		}
		if strings.TrimSpace(task.Title) == "" || strings.TrimSpace(task.Objective) == "" {
			return fmt.Errorf("task %s missing title or objective", task.ID)
		}
		if task.Wave <= 0 {
			return fmt.Errorf("task %s wave must be > 0", task.ID)
		}
		if len(nonEmpty(task.WriteScope)) == 0 {
			return fmt.Errorf("task %s write_scope is required", task.ID)
		}
		if len(nonEmpty(task.AcceptanceCriteria)) == 0 {
			return fmt.Errorf("task %s acceptance_criteria is required", task.ID)
		}
		if len(nonEmpty(task.VerificationCommands)) == 0 {
			return fmt.Errorf("task %s verification_commands is required", task.ID)
		}
		byID[task.ID] = task
	}
	for _, task := range graph.Tasks {
		for _, dep := range task.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			if dep == task.ID {
				return fmt.Errorf("task graph cycle: %s depends on itself", task.ID)
			}
			if _, exists := byID[dep]; !exists {
				return fmt.Errorf("task %s depends on unknown task %s", task.ID, dep)
			}
		}
	}
	if err := rejectCycles(byID); err != nil {
		return err
	}
	for _, task := range graph.Tasks {
		for _, dep := range task.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			depTask := byID[dep]
			if depTask.Wave >= task.Wave {
				return fmt.Errorf("task %s dependency %s must be in an earlier wave", task.ID, dep)
			}
		}
	}
	if err := rejectParallelWriteScopeConflicts(graph.Tasks); err != nil {
		return err
	}
	return nil
}

func OrderedTasks(graph TaskGraph) []TaskSpec {
	out := append([]TaskSpec{}, graph.Tasks...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Wave != out[j].Wave {
			return out[i].Wave < out[j].Wave
		}
		if out[i].Repo != out[j].Repo {
			return out[i].Repo < out[j].Repo
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func rejectCycles(tasks map[string]TaskSpec) error {
	const (
		unseen = 0
		active = 1
		done   = 2
	)
	state := map[string]int{}
	var visit func(string, []string) error
	visit = func(id string, stack []string) error {
		switch state[id] {
		case active:
			return fmt.Errorf("task graph cycle detected: %s -> %s", strings.Join(stack, " -> "), id)
		case done:
			return nil
		}
		state[id] = active
		task := tasks[id]
		for _, dep := range task.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			if err := visit(dep, append(stack, id)); err != nil {
				return err
			}
		}
		state[id] = done
		return nil
	}
	for id := range tasks {
		if state[id] == unseen {
			if err := visit(id, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectParallelWriteScopeConflicts(tasks []TaskSpec) error {
	type waveKey struct {
		repo string
		wave int
	}
	seen := map[waveKey]map[string]string{}
	for _, task := range tasks {
		key := waveKey{repo: task.Repo, wave: task.Wave}
		if seen[key] == nil {
			seen[key] = map[string]string{}
		}
		for _, scope := range nonEmpty(task.WriteScope) {
			scope = normalizeScope(scope)
			if other := seen[key][scope]; other != "" {
				return fmt.Errorf("parallel write scope conflict in repo %s wave %d: %s and %s both touch %s", task.Repo, task.Wave, other, task.ID, scope)
			}
			seen[key][scope] = task.ID
		}
	}
	return nil
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func normalizeScope(scope string) string {
	scope = strings.ReplaceAll(scope, "\\", "/")
	scope = strings.Trim(scope, "/ ")
	return strings.ToLower(scope)
}
