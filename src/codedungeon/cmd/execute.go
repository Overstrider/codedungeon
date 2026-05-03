package cmd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	artifactreg "github.com/loldinis/codedungeon/internal/artifacts"
	"github.com/loldinis/codedungeon/internal/recovery"
	"github.com/loldinis/codedungeon/internal/taskexec"
	"github.com/loldinis/codedungeon/internal/taskplanning"
)

func ExecuteCmd() *cobra.Command {
	c := &cobra.Command{Use: "execute", Short: "Run one implementation task through the CodeDungeon executor"}
	c.AddCommand(executeRunCmd())
	c.AddCommand(executeTaskCmd())
	c.AddCommand(executePlanCmd())
	c.AddCommand(executeStatusCmd())
	c.AddCommand(executeRollbackCmd())
	return c
}

type executeRunInput struct {
	Kind  string
	Value string
}

type executeRunResult struct {
	OK             bool                              `json:"ok"`
	Status         string                            `json:"status"`
	ExecutionID    string                            `json:"execution_id,omitempty"`
	Input          string                            `json:"input"`
	Tasks          int                               `json:"tasks"`
	Planning       *taskplanning.PromptPlannerOutput `json:"planning,omitempty"`
	TaskGraphPath  string                            `json:"task_graph_path,omitempty"`
	ContractsDir   string                            `json:"contracts_dir,omitempty"`
	Contracts      []string                          `json:"contracts,omitempty"`
	Results        []taskexec.Result                 `json:"results,omitempty"`
	TouchedFiles   []string                          `json:"touched_files,omitempty"`
	Risks          []string                          `json:"risks,omitempty"`
	RepairActions  []taskplanning.RepairAction       `json:"repair_actions,omitempty"`
	Notes          []string                          `json:"notes,omitempty"`
	FailureMessage string                            `json:"failure_message,omitempty"`
}

func executeRunCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "run",
		Short: "Plan from prompt when needed, then execute task contracts",
		RunE: func(c *cobra.Command, _ []string) error {
			executionID := newExecutionArtifactID()
			emitRunErr := func(msg, hint string) error {
				return emitExecuteRunErr(executionID, msg, hint)
			}
			input, err := executeRunInputFromFlags(c)
			if err != nil {
				return emitRunErr(err.Error(), "")
			}
			root := currentProjectRoot()
			projectContextArg, _ := c.Flags().GetString("project-context")
			workspacePolicyArg, _ := c.Flags().GetString("workspace-policy")
			projectContext, err := readOptionalContextArg(projectContextArg)
			if err != nil {
				return emitRunErr("project-context: "+err.Error(), "")
			}
			if strings.TrimSpace(projectContext) == "" {
				projectContext, err = defaultExecuteProjectContext(root)
				if err != nil {
					return emitRunErr(err.Error(), "")
				}
			}
			workspacePolicy, err := readOptionalContextArg(workspacePolicyArg)
			if err != nil {
				return emitRunErr("workspace-policy: "+err.Error(), "")
			}
			cfg, err := taskexec.LoadConfig(root)
			if err != nil {
				return emitRunErr(err.Error(), "")
			}
			runnerName, _ := c.Flags().GetString("runner")
			inputDir, _ := c.Flags().GetString("input-dir")
			if runnerName != "" {
				cfg.Runner = runnerName
			}
			execRunner, err := executionRunner(cfg.Runner, inputDir, root)
			if err != nil {
				return emitRunErr(err.Error(), "")
			}
			runID, err := currentRunID(c)
			if err != nil {
				return emitRunErr(err.Error(), "")
			}
			var planner taskplanning.PromptPlanner
			if input.Kind == "prompt" || input.Kind == "prompt-file" {
				plannerRunnerName, _ := c.Flags().GetString("planner-runner")
				plannerInputDir, _ := c.Flags().GetString("planner-input-dir")
				planner, err = promptPlannerRunner(plannerRunnerName, plannerInputDir, root, cfg.PromptPlanner)
				if err != nil {
					return emitRunErr(err.Error(), "")
				}
			}
			dryRun, _ := c.Flags().GetBool("dry-run")
			verbose, _ := c.Flags().GetBool("verbose")
			result, runErr := executeRunFlow(c.Context(), executeRunFlowRequest{
				Root:            root,
				RunID:           runID,
				Input:           input,
				ProjectContext:  projectContext,
				WorkspacePolicy: workspacePolicy,
				DryRun:          dryRun,
				Verbose:         verbose,
				Config:          cfg,
				Executor:        execRunner,
				Planner:         planner,
				ExecutionID:     executionID,
			})
			if runErr != nil {
				if result.Status != "" {
					_ = EmitJSON(result)
					return runErr
				}
				return emitRunErr(runErr.Error(), "")
			}
			if err := registerExecuteRunArtifacts(c, root, runID, executionID, result); err != nil {
				return emitRunErr(err.Error(), "")
			}
			return EmitJSON(result)
		},
	}
	c.Flags().String("task", "", "single task contract JSON path")
	c.Flags().String("tasks", "", "directory containing task contract JSON files")
	c.Flags().String("task-graph", "", "task graph JSON")
	c.Flags().String("prompt", "", "prompt to plan and execute")
	c.Flags().String("prompt-file", "", "file containing prompt to plan and execute")
	c.Flags().String("project-context", "", "project context text or path")
	c.Flags().String("workspace-policy", "", "workspace policy text or path")
	c.Flags().Bool("dry-run", false, "render execution prompts/sessions without invoking provider or git mutation")
	c.Flags().Bool("verbose", false, "emit verbose metadata")
	c.Flags().String("runner", "codex", "execution runner: codex or files")
	c.Flags().String("input-dir", "", "input fixture dir for --runner files")
	c.Flags().String("planner-runner", "codex", "prompt planner runner: codex or files")
	c.Flags().String("planner-input-dir", "", "input fixture dir for --planner-runner files")
	return c
}

type executeRunFlowRequest struct {
	Root            string
	RunID           int64
	Input           executeRunInput
	ProjectContext  string
	WorkspacePolicy string
	DryRun          bool
	Verbose         bool
	Config          taskexec.Config
	Executor        taskexec.Runner
	Planner         taskplanning.PromptPlanner
	ExecutionID     string
}

func executeRunFlow(ctx context.Context, req executeRunFlowRequest) (executeRunResult, error) {
	executionID := strings.TrimSpace(req.ExecutionID)
	if executionID == "" {
		executionID = newExecutionArtifactID()
	}
	runLabel := runIDLabel(req.RunID)
	result := executeRunResult{OK: true, Status: taskexec.StatusRunning, ExecutionID: executionID, Input: req.Input.Kind}
	var taskPaths []string
	var risks []string
	var repairActions []taskplanning.RepairAction
	var err error
	switch req.Input.Kind {
	case "task":
		taskPaths = []string{req.Input.Value}
	case "tasks":
		taskPaths, err = taskContractPaths(req.Input.Value)
	case "task-graph":
		var graph taskplanning.TaskGraph
		graph, err = readTaskGraphFile(req.Input.Value)
		if err == nil {
			graph, repairActions, err = validateOrRepairTaskGraph(graph)
		}
		if err == nil {
			var contractsDir, graphPath string
			contractsDir, graphPath, taskPaths, err = renderExecutionTaskContracts(req.Root, runLabel, executionID, graph)
			result.ContractsDir = contractsDir
			result.TaskGraphPath = graphPath
			result.Contracts = append([]string(nil), taskPaths...)
		}
	case "prompt", "prompt-file":
		if req.Planner == nil {
			err = fmt.Errorf("prompt planner is required")
			break
		}
		if req.Input.Kind == "prompt-file" {
			body, readErr := os.ReadFile(req.Input.Value)
			if readErr != nil {
				err = readErr
				break
			}
			req.Input.Value = string(body)
		}
		var planningDir string
		planningDir, err = createExecutionArtifactDir(req.Root, "prompt-planning", runLabel, executionID)
		if err != nil {
			break
		}
		var planning taskplanning.PromptPlannerOutput
		planning, err = req.Planner.Plan(ctx, taskplanning.PromptPlannerRequest{
			Prompt:          req.Input.Value,
			ProjectContext:  req.ProjectContext,
			WorkspacePolicy: firstNonEmptyString(req.WorkspacePolicy, "Planner runs read-only and must only produce a task graph."),
			OutputDir:       planningDir,
			ProjectRules:    executeRunProjectRules(req.Root),
		})
		result.Planning = &planning
		risks = append(risks, riskStrings(planning.Risks)...)
		if err != nil {
			break
		}
		if planning.NeedsUserInput {
			result.Status = taskplanning.StatusNeedsUserInput
			result.Tasks = 0
			result.Risks = risks
			result.Notes = append(result.Notes, questionStrings(planning.Questions)...)
			return result, nil
		}
		if planning.TaskGraph == nil {
			err = fmt.Errorf("prompt planner did not return task_graph")
			break
		}
		graph := *planning.TaskGraph
		graph, repairActions, err = validateOrRepairTaskGraph(graph)
		if err == nil {
			var contractsDir, graphPath string
			contractsDir, graphPath, taskPaths, err = renderExecutionTaskContracts(req.Root, runLabel, executionID, graph)
			result.ContractsDir = contractsDir
			result.TaskGraphPath = graphPath
			result.Contracts = append([]string(nil), taskPaths...)
		}
	default:
		err = fmt.Errorf("unsupported execute run input %q", req.Input.Kind)
	}
	if err != nil {
		result.OK = false
		result.Status = taskexec.StatusFailed
		result.FailureMessage = err.Error()
		result.Risks = risks
		result.RepairActions = repairActions
		return result, err
	}
	if len(taskPaths) == 0 {
		err = fmt.Errorf("no task contracts found")
		result.OK = false
		result.Status = taskexec.StatusFailed
		result.FailureMessage = err.Error()
		return result, err
	}
	execResults, execErr := executeTaskPathSequence(ctx, req, taskPaths)
	result.Results = execResults
	result.Tasks = len(execResults)
	result.TouchedFiles = touchedFiles(execResults)
	result.Risks = risks
	result.RepairActions = repairActions
	if execErr != nil {
		result.OK = false
		result.Status = taskexec.StatusFailed
		result.FailureMessage = execErr.Error()
		return result, execErr
	}
	result.Status = taskexec.StatusCompleted
	if req.DryRun {
		result.Status = taskexec.StatusDryRun
	}
	return result, nil
}

func executeTaskPathSequence(ctx context.Context, req executeRunFlowRequest, taskPaths []string) ([]taskexec.Result, error) {
	results := make([]taskexec.Result, 0, len(taskPaths))
	for _, taskPath := range taskPaths {
		result, execErr := taskexec.Execute(ctx, taskexec.Request{
			Root:            req.Root,
			RunID:           req.RunID,
			TaskPath:        taskPath,
			ProjectContext:  req.ProjectContext,
			WorkspacePolicy: req.WorkspacePolicy,
			DryRun:          req.DryRun,
			Verbose:         req.Verbose,
			Config:          req.Config,
			Runner:          req.Executor,
		})
		results = append(results, result)
		if execErr != nil {
			return results, execErr
		}
		if !req.DryRun && result.Status != taskexec.StatusCompleted {
			return results, fmt.Errorf("execution stopped at %s with status %s", result.Task.ID, result.Status)
		}
	}
	return results, nil
}

func registerExecuteRunArtifacts(c *cobra.Command, root string, runID int64, executionID string, result executeRunResult) error {
	store, err := OpenDB(c)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Init(); err != nil {
		return err
	}
	registry := artifactreg.NewRegistry(store, root)
	meta := map[string]any{"status": result.Status, "input": result.Input, "tasks": result.Tasks}
	for _, item := range []struct {
		role string
		path string
	}{
		{"contracts_dir", result.ContractsDir},
		{"task_graph", result.TaskGraphPath},
	} {
		if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
			RunID: runID, Module: "execution", OwnerType: "execute_run", OwnerID: executionID,
			Phase: "5", Role: item.role, Kind: artifactreg.KindForPath(item.path), Path: item.path, Metadata: meta,
		}); err != nil {
			return err
		}
	}
	for _, path := range result.Contracts {
		if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
			RunID: runID, Module: "execution", OwnerType: "execute_run", OwnerID: executionID,
			Phase: "5", Role: "contract", Kind: "json", Path: path, Metadata: meta,
		}); err != nil {
			return err
		}
	}
	for _, execResult := range result.Results {
		if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
			RunID: runID, Module: "execution", OwnerType: "execute_run", OwnerID: executionID,
			Phase: "5", Role: "task_output", Kind: "directory", Path: execResult.OutputDir,
			Metadata: map[string]any{"task_id": execResult.Task.ID, "status": execResult.Status},
		}); err != nil {
			return err
		}
	}
	return nil
}

func executeRunInputFromFlags(c *cobra.Command) (executeRunInput, error) {
	type candidate struct {
		kind  string
		value string
	}
	var candidates []candidate
	for _, item := range []candidate{
		{kind: "task", value: flagString(c, "task")},
		{kind: "tasks", value: flagString(c, "tasks")},
		{kind: "task-graph", value: flagString(c, "task-graph")},
		{kind: "prompt", value: flagString(c, "prompt")},
		{kind: "prompt-file", value: flagString(c, "prompt-file")},
	} {
		if strings.TrimSpace(item.value) != "" {
			candidates = append(candidates, item)
		}
	}
	if len(candidates) != 1 {
		return executeRunInput{}, fmt.Errorf("execute run requires exactly one input: --task, --tasks, --task-graph, --prompt, or --prompt-file")
	}
	return executeRunInput{Kind: candidates[0].kind, Value: candidates[0].value}, nil
}

func flagString(c *cobra.Command, name string) string {
	value, _ := c.Flags().GetString(name)
	return value
}

func promptPlannerRunner(name, inputDir, root string, cfg taskplanning.PromptPlannerConfig) (taskplanning.PromptPlanner, error) {
	switch strings.TrimSpace(name) {
	case "", "codex":
		if !cfg.Enabled {
			return nil, fmt.Errorf("prompt planner is disabled by .ralphrc")
		}
		return taskplanning.NewPlannerSplitterRunner(root, cfg), nil
	case "files":
		return taskplanning.PlannerSplitterFilesRunner{InputDir: inputDir}, nil
	default:
		return nil, fmt.Errorf("unknown prompt planner runner %q", name)
	}
}

func readTaskGraphFile(path string) (taskplanning.TaskGraph, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return taskplanning.TaskGraph{}, err
	}
	var graph taskplanning.TaskGraph
	if err := json.Unmarshal(trimJSONBOM(body), &graph); err != nil {
		return taskplanning.TaskGraph{}, fmt.Errorf("invalid task graph JSON: %w", err)
	}
	return graph, nil
}

func validateOrRepairTaskGraph(graph taskplanning.TaskGraph) (taskplanning.TaskGraph, []taskplanning.RepairAction, error) {
	if err := taskplanning.ValidateTaskGraph(graph); err == nil {
		return graph, nil, nil
	}
	repaired, actions, repairErr := taskplanning.RepairTaskGraph(graph)
	if repairErr != nil {
		return graph, nil, repairErr
	}
	if err := taskplanning.ValidateTaskGraph(repaired); err != nil {
		return graph, actions, err
	}
	return repaired, actions, nil
}

func renderExecutionTaskContracts(root, runLabel, executionID string, graph taskplanning.TaskGraph) (string, string, []string, error) {
	if strings.TrimSpace(executionID) == "" {
		return "", "", nil, fmt.Errorf("execution_id is required")
	}
	ordered := taskplanning.OrderedTasks(graph)
	seenNames := map[string]string{}
	for _, task := range ordered {
		name := safeContractFileName(task.ID)
		if other, exists := seenNames[name]; exists {
			return "", "", nil, fmt.Errorf("contract filename collision: %s and %s both render to %s", other, task.ID, name)
		}
		seenNames[name] = task.ID
	}
	outDir, err := createExecutionArtifactDir(root, "task-contracts", runLabel, executionID)
	if err != nil {
		return "", "", nil, err
	}
	graphPath := filepath.Join(outDir, "task-graph.json")
	body, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return "", "", nil, err
	}
	if err := os.WriteFile(graphPath, append(body, '\n'), 0o644); err != nil {
		return "", "", nil, err
	}
	var paths []string
	for _, task := range ordered {
		path := filepath.Join(outDir, safeContractFileName(task.ID))
		body, err := json.MarshalIndent(task, "", "  ")
		if err != nil {
			return "", "", nil, err
		}
		if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
			return "", "", nil, err
		}
		paths = append(paths, path)
	}
	return outDir, graphPath, paths, nil
}

func createExecutionArtifactDir(root, area, runLabel, executionID string) (string, error) {
	if strings.TrimSpace(runLabel) == "" {
		return "", fmt.Errorf("run label is required")
	}
	if strings.TrimSpace(executionID) == "" {
		return "", fmt.Errorf("execution_id is required")
	}
	parent := filepath.Join(root, ".codedungeon", "execute", area, runLabel)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	outDir := filepath.Join(parent, executionID)
	if err := os.Mkdir(outDir, 0o755); err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("execution artifact directory already exists: %s", outDir)
		}
		return "", err
	}
	return outDir, nil
}

func safeContractFileName(id string) string {
	id = strings.ReplaceAll(id, "\\", "_")
	id = strings.ReplaceAll(id, "/", "_")
	id = strings.TrimSpace(id)
	if id == "" {
		id = "task"
	}
	return id + ".json"
}

func runIDLabel(runID int64) string {
	if runID > 0 {
		return strconv.FormatInt(runID, 10)
	}
	return "run-" + time.Now().Format("20060102150405")
}

func newExecutionArtifactID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "exec-" + hex.EncodeToString(b[:])
	}
	return "exec-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 16)
}

func emitExecuteRunErr(executionID, msg, hint string) error {
	_ = EmitJSON(map[string]any{
		"ok":           false,
		"status":       taskexec.StatusFailed,
		"execution_id": executionID,
		"error":        msg,
		"hint":         hint,
	})
	return fmt.Errorf("%s", msg)
}

func executeRunProjectRules(root string) taskplanning.ProjectRulesEnvelope {
	rules := taskplanning.ProjectRulesEnvelope{Status: "missing", Digest: "none", Read: "yes"}
	if status, err := computeProjectRulesStatus(root); err == nil {
		rules.Status = status.Status
		rules.Digest = status.RulesDigest
	}
	return rules
}

func riskStrings(risks []taskplanning.Risk) []string {
	out := make([]string, 0, len(risks))
	for _, risk := range risks {
		if strings.TrimSpace(risk.Title) == "" && strings.TrimSpace(risk.Impact) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(risk.Title+": "+risk.Impact))
	}
	return out
}

func questionStrings(questions []taskplanning.Question) []string {
	out := make([]string, 0, len(questions))
	for _, question := range questions {
		if strings.TrimSpace(question.Question) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(question.Question+": "+question.Impact))
	}
	return out
}

func touchedFiles(results []taskexec.Result) []string {
	seen := map[string]bool{}
	var out []string
	for _, result := range results {
		for _, file := range result.ChangedFiles {
			file = strings.TrimSpace(file)
			if file == "" || seen[file] {
				continue
			}
			seen[file] = true
			out = append(out, file)
		}
	}
	sort.Strings(out)
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func executeTaskCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "task",
		Short: "Execute one task contract",
		RunE: func(c *cobra.Command, _ []string) error {
			taskPath, _ := c.Flags().GetString("task")
			projectContextArg, _ := c.Flags().GetString("project-context")
			workspacePolicyArg, _ := c.Flags().GetString("workspace-policy")
			sessionID, _ := c.Flags().GetString("session")
			resumeID, _ := c.Flags().GetString("resume")
			resetSession, _ := c.Flags().GetBool("reset-session")
			dryRun, _ := c.Flags().GetBool("dry-run")
			verbose, _ := c.Flags().GetBool("verbose")
			runnerName, _ := c.Flags().GetString("runner")
			inputDir, _ := c.Flags().GetString("input-dir")
			if strings.TrimSpace(taskPath) == "" {
				return EmitErr("--task is required", "")
			}
			projectContext, err := readOptionalContextArg(projectContextArg)
			if err != nil {
				return EmitErr("project-context: "+err.Error(), "")
			}
			if strings.TrimSpace(projectContext) == "" {
				projectContext, err = defaultExecuteProjectContext(currentProjectRoot())
				if err != nil {
					return EmitErr(err.Error(), "")
				}
			}
			workspacePolicy, err := readOptionalContextArg(workspacePolicyArg)
			if err != nil {
				return EmitErr("workspace-policy: "+err.Error(), "")
			}
			root := currentProjectRoot()
			cfg, err := taskexec.LoadConfig(root)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if runnerName != "" {
				cfg.Runner = runnerName
			}
			var runner taskexec.Runner
			switch cfg.Runner {
			case "", "codex":
				runner = taskexec.CodexRunner{WorkDir: root}
			case "files":
				runner = taskexec.FilesRunner{InputDir: inputDir}
			default:
				return EmitErr("unknown execution runner "+cfg.Runner, "")
			}
			runID, err := currentRunID(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			result, err := taskexec.Execute(c.Context(), taskexec.Request{
				Root:            root,
				RunID:           runID,
				TaskPath:        taskPath,
				ProjectContext:  projectContext,
				WorkspacePolicy: workspacePolicy,
				SessionID:       sessionID,
				ResumeID:        resumeID,
				ResetSession:    resetSession,
				DryRun:          dryRun,
				Verbose:         verbose,
				Config:          cfg,
				Runner:          runner,
			})
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(result)
		},
	}
	c.Flags().String("task", "", "task contract JSON path")
	c.Flags().String("project-context", "", "project context text or path")
	c.Flags().String("workspace-policy", "", "workspace policy text or path")
	c.Flags().String("session", "", "existing execution session id")
	c.Flags().String("resume", "", "explicit execution session id to resume")
	c.Flags().Bool("reset-session", false, "reset an expired or failed session before resuming")
	c.Flags().Bool("dry-run", false, "render execution prompt/session without invoking provider or git mutation")
	c.Flags().Bool("verbose", false, "emit verbose metadata")
	c.Flags().String("runner", "codex", "execution runner: codex or files")
	c.Flags().String("input-dir", "", "input fixture dir for --runner files")
	return c
}

func executePlanCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "plan",
		Short: "Execute a directory of task contracts one at a time",
		RunE: func(c *cobra.Command, _ []string) error {
			taskDir, _ := c.Flags().GetString("tasks")
			taskGraphPath, _ := c.Flags().GetString("task-graph")
			projectContextArg, _ := c.Flags().GetString("project-context")
			workspacePolicyArg, _ := c.Flags().GetString("workspace-policy")
			dryRun, _ := c.Flags().GetBool("dry-run")
			verbose, _ := c.Flags().GetBool("verbose")
			runnerName, _ := c.Flags().GetString("runner")
			inputDir, _ := c.Flags().GetString("input-dir")
			if strings.TrimSpace(taskDir) == "" && strings.TrimSpace(taskGraphPath) == "" {
				return EmitErr("--tasks or --task-graph is required", "")
			}
			if strings.TrimSpace(taskDir) != "" && strings.TrimSpace(taskGraphPath) != "" {
				return EmitErr("use either --tasks or --task-graph, not both", "")
			}
			root := currentProjectRoot()
			var taskPaths []string
			var err error
			if taskGraphPath != "" {
				taskPaths, err = taskContractPathsFromGraph(root, taskGraphPath)
			} else {
				taskPaths, err = taskContractPaths(taskDir)
			}
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if len(taskPaths) == 0 {
				return EmitErr("no task contracts found", taskDir)
			}
			projectContext, err := readOptionalContextArg(projectContextArg)
			if err != nil {
				return EmitErr("project-context: "+err.Error(), "")
			}
			if strings.TrimSpace(projectContext) == "" {
				projectContext, err = defaultExecuteProjectContext(root)
				if err != nil {
					return EmitErr(err.Error(), "")
				}
			}
			workspacePolicy, err := readOptionalContextArg(workspacePolicyArg)
			if err != nil {
				return EmitErr("workspace-policy: "+err.Error(), "")
			}
			cfg, err := taskexec.LoadConfig(root)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if runnerName != "" {
				cfg.Runner = runnerName
			}
			runner, err := executionRunner(cfg.Runner, inputDir, root)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			runID, err := currentRunID(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			results := make([]taskexec.Result, 0, len(taskPaths))
			for _, taskPath := range taskPaths {
				result, execErr := taskexec.Execute(c.Context(), taskexec.Request{
					Root:            root,
					RunID:           runID,
					TaskPath:        taskPath,
					ProjectContext:  projectContext,
					WorkspacePolicy: workspacePolicy,
					DryRun:          dryRun,
					Verbose:         verbose,
					Config:          cfg,
					Runner:          runner,
				})
				results = append(results, result)
				if execErr != nil {
					return EmitErr(execErr.Error(), "")
				}
				if !dryRun && result.Status != taskexec.StatusCompleted {
					return EmitErr("execution stopped at "+result.Task.ID+" with status "+result.Status, "")
				}
			}
			return EmitJSON(map[string]any{"ok": true, "tasks": len(results), "results": results})
		},
	}
	c.Flags().String("tasks", "", "directory containing task contract JSON files")
	c.Flags().String("task-graph", "", "task graph JSON produced by codedungeon plan run")
	c.Flags().String("project-context", "", "project context text or path")
	c.Flags().String("workspace-policy", "", "workspace policy text or path")
	c.Flags().Bool("dry-run", false, "render execution prompts/sessions without invoking provider or git mutation")
	c.Flags().Bool("verbose", false, "emit verbose metadata")
	c.Flags().String("runner", "codex", "execution runner: codex or files")
	c.Flags().String("input-dir", "", "input fixture dir for --runner files")
	return c
}

func taskContractPaths(taskDir string) ([]string, error) {
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		paths = append(paths, filepath.Join(taskDir, entry.Name()))
	}
	sort.Strings(paths)
	return paths, nil
}

func taskContractPathsFromGraph(root, graphPath string) ([]string, error) {
	body, err := os.ReadFile(graphPath)
	if err != nil {
		return nil, err
	}
	var graph taskplanning.TaskGraph
	if err := json.Unmarshal(trimJSONBOM(body), &graph); err != nil {
		return nil, fmt.Errorf("invalid task graph JSON: %w", err)
	}
	if err := taskplanning.ValidateTaskGraph(graph); err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(filepath.Base(graphPath), filepath.Ext(graphPath))
	outDir := filepath.Join(root, ".codedungeon", "execute", "task-contracts", name)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	var paths []string
	for _, task := range taskplanning.OrderedTasks(graph) {
		path := filepath.Join(outDir, task.ID+".json")
		body, err := json.MarshalIndent(task, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func trimJSONBOM(body []byte) []byte {
	if len(body) >= 3 && body[0] == 0xEF && body[1] == 0xBB && body[2] == 0xBF {
		return body[3:]
	}
	return body
}

func executionRunner(name, inputDir, root string) (taskexec.Runner, error) {
	switch name {
	case "", "codex":
		return taskexec.CodexRunner{WorkDir: root}, nil
	case "files":
		return taskexec.FilesRunner{InputDir: inputDir}, nil
	default:
		return nil, fmt.Errorf("unknown execution runner %s", name)
	}
}

func currentRunID(c *cobra.Command) (int64, error) {
	store, err := OpenDB(c)
	if err != nil {
		return 0, err
	}
	defer store.Close()
	if err := store.Init(); err != nil {
		return 0, err
	}
	run, err := store.CurrentRun()
	if err != nil {
		return 0, err
	}
	if run == nil {
		return 0, nil
	}
	return run.ID, nil
}

func executeStatusCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "status",
		Short: "Show execution session status",
		RunE: func(c *cobra.Command, _ []string) error {
			sessionID, _ := c.Flags().GetString("session")
			if strings.TrimSpace(sessionID) == "" {
				return EmitErr("--session is required", "")
			}
			store, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer store.Close()
			session, err := store.ExecutionSession(sessionID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if session == nil {
				return EmitErr("execution session not found: "+sessionID, "")
			}
			attempts, _ := store.ExecutionAttempts(sessionID)
			transitions, _ := store.ExecutionTransitions(sessionID)
			rec, err := recovery.InspectExecutionSession(store, sessionID, currentProjectRoot(), time.Now())
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{
				"ok":          true,
				"session":     session,
				"attempts":    attempts,
				"transitions": transitions,
				"recovery":    rec,
			})
		},
	}
	c.Flags().String("session", "", "execution session id")
	return c
}

func executeRollbackCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "rollback",
		Short: "Print the rollback target for a session snapshot",
		RunE: func(c *cobra.Command, _ []string) error {
			sessionID, _ := c.Flags().GetString("session")
			to, _ := c.Flags().GetString("to")
			confirm, _ := c.Flags().GetBool("confirm")
			if strings.TrimSpace(sessionID) == "" {
				return EmitErr("--session is required", "")
			}
			if !confirm {
				return EmitErr("--confirm is required for rollback", "")
			}
			if to == "" {
				to = "before"
			}
			store, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer store.Close()
			plan, err := recovery.BuildRollbackPlan(store, sessionID, to)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{
				"ok":         true,
				"session_id": sessionID,
				"target":     plan.Target,
				"command":    plan.Command,
				"rollback":   plan,
			})
		},
	}
	c.Flags().String("session", "", "execution session id")
	c.Flags().String("to", "before", "before or attempt-N")
	c.Flags().Bool("confirm", false, "confirm rollback target inspection")
	return c
}

func defaultExecuteProjectContext(root string) (string, error) {
	candidates := []string{
		filepath.Join(root, ".codedungeon", "project-context.md"),
		filepath.Join(root, ".codedungeon", "project-rules.compact.md"),
	}
	for _, path := range candidates {
		body, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(body)) != "" {
			return string(body), nil
		}
	}
	return "", fmt.Errorf("project-context is required; pass --project-context or create .codedungeon/project-context.md")
}
