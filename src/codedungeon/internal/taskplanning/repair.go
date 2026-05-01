package taskplanning

import (
	"fmt"
	"sort"
	"strings"
)

type RepairAction struct {
	TaskID string `json:"task_id"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

func RepairTaskGraph(graph TaskGraph) (TaskGraph, []RepairAction, error) {
	repaired := copyTaskGraph(graph)
	if len(repaired.Tasks) == 0 {
		return repaired, nil, ValidateTaskGraph(repaired)
	}
	var actions []RepairAction
	for i := 0; i < len(repaired.Tasks)*len(repaired.Tasks)+1; i++ {
		changed, passActions := serializeWriteScopeConflicts(&repaired)
		actions = append(actions, passActions...)
		if !changed {
			break
		}
		normalizeDependencyWaves(&repaired, &actions)
	}
	if err := ValidateTaskGraph(repaired); err != nil {
		return repaired, actions, err
	}
	sort.SliceStable(repaired.Tasks, func(i, j int) bool {
		if repaired.Tasks[i].Wave != repaired.Tasks[j].Wave {
			return repaired.Tasks[i].Wave < repaired.Tasks[j].Wave
		}
		if repaired.Tasks[i].Repo != repaired.Tasks[j].Repo {
			return repaired.Tasks[i].Repo < repaired.Tasks[j].Repo
		}
		return repaired.Tasks[i].ID < repaired.Tasks[j].ID
	})
	return repaired, actions, nil
}

func copyTaskGraph(graph TaskGraph) TaskGraph {
	out := TaskGraph{Version: graph.Version, Tasks: make([]TaskSpec, len(graph.Tasks))}
	for i, task := range graph.Tasks {
		out.Tasks[i] = task
		out.Tasks[i].Context = append([]string(nil), task.Context...)
		out.Tasks[i].WriteScope = append([]string(nil), task.WriteScope...)
		out.Tasks[i].DependsOn = append([]string(nil), task.DependsOn...)
		out.Tasks[i].AcceptanceCriteria = append([]string(nil), task.AcceptanceCriteria...)
		out.Tasks[i].VerificationCommands = append([]string(nil), task.VerificationCommands...)
		out.Tasks[i].RiskNotes = append([]string(nil), task.RiskNotes...)
	}
	return out
}

func serializeWriteScopeConflicts(graph *TaskGraph) (bool, []RepairAction) {
	type waveKey struct {
		repo string
		wave int
	}
	ordered := OrderedTasks(*graph)
	indexByID := map[string]int{}
	for i, task := range graph.Tasks {
		indexByID[task.ID] = i
	}
	seen := map[waveKey]map[string]string{}
	var actions []RepairAction
	for _, task := range ordered {
		key := waveKey{repo: task.Repo, wave: task.Wave}
		if seen[key] == nil {
			seen[key] = map[string]string{}
		}
		for _, scope := range nonEmpty(task.WriteScope) {
			scope = normalizeScope(scope)
			other := seen[key][scope]
			if other == "" {
				seen[key][scope] = task.ID
				continue
			}
			idx := indexByID[task.ID]
			depIdx := indexByID[other]
			graph.Tasks[idx].Wave = maxInt(graph.Tasks[idx].Wave, graph.Tasks[depIdx].Wave+1)
			if !stringSliceContains(graph.Tasks[idx].DependsOn, other) {
				graph.Tasks[idx].DependsOn = append(graph.Tasks[idx].DependsOn, other)
			}
			actions = append(actions, RepairAction{
				TaskID: task.ID,
				Kind:   "serialize_write_scope",
				Detail: fmt.Sprintf("%s moved after %s because both touch %s", task.ID, other, scope),
			})
			return true, actions
		}
	}
	return false, actions
}

func normalizeDependencyWaves(graph *TaskGraph, actions *[]RepairAction) {
	for i := 0; i < len(graph.Tasks)*len(graph.Tasks)+1; i++ {
		changed := false
		byID := map[string]TaskSpec{}
		for _, task := range graph.Tasks {
			byID[task.ID] = task
		}
		for idx, task := range graph.Tasks {
			for _, dep := range task.DependsOn {
				dep = strings.TrimSpace(dep)
				if dep == "" {
					continue
				}
				depTask, ok := byID[dep]
				if !ok || depTask.Wave < graph.Tasks[idx].Wave {
					continue
				}
				graph.Tasks[idx].Wave = depTask.Wave + 1
				changed = true
				*actions = append(*actions, RepairAction{
					TaskID: task.ID,
					Kind:   "bump_dependency_wave",
					Detail: fmt.Sprintf("%s moved to wave %d after dependency %s", task.ID, graph.Tasks[idx].Wave, dep),
				})
			}
		}
		if !changed {
			return
		}
	}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
