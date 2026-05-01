package taskplanning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func Execute(ctx context.Context, req Request, runner Runner) (Result, error) {
	if runner == nil {
		return Result{}, fmt.Errorf("task planning runner is required")
	}
	if err := normalizeRequest(&req); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(filepath.Join(req.OutputDir, "agent-outputs"), 0o755); err != nil {
		return Result{}, err
	}
	requestPath := filepath.Join(req.OutputDir, "planning-request.json")
	blackboardPath := filepath.Join(req.OutputDir, "blackboard.jsonl")
	if err := writeJSONFile(requestPath, req); err != nil {
		return Result{}, err
	}
	if err := os.WriteFile(blackboardPath, nil, 0o644); err != nil {
		return Result{}, err
	}

	result := Result{
		OK:             true,
		SessionID:      req.SessionID,
		RunID:          req.RunID,
		Status:         StatusRunning,
		OutputDir:      req.OutputDir,
		RequestPath:    requestPath,
		BlackboardPath: blackboardPath,
		ProjectRules:   req.ProjectRules,
		Artifacts:      []string{requestPath, blackboardPath},
	}

	contextPacket := buildContextPacket(req, nil)
	exploration, err := runRoles(ctx, runner, req, req.Roles, 1, contextPacket, nil, blackboardPath)
	if err != nil {
		return failResult(result, err)
	}
	result.Agents = append(result.Agents, exploration...)

	evaluatorOutput, err := runRole(ctx, runner, req, "planning_evaluator", 2, buildContextPacket(req, exploration), exploration, blackboardPath, true)
	if err != nil {
		return failResult(result, err)
	}
	result.Agents = append(result.Agents, evaluatorOutput)
	evaluation := buildEvaluation(evaluatorOutput, req.HumanGatePolicy)
	result.Evaluation = &evaluation
	evaluationPath := filepath.Join(req.OutputDir, "evaluation.json")
	if err := writeJSONFile(evaluationPath, evaluation); err != nil {
		return failResult(result, err)
	}
	result.EvaluationPath = evaluationPath
	result.Artifacts = append(result.Artifacts, evaluationPath)
	if strings.EqualFold(evaluation.Verdict, "FAIL") {
		return failResult(result, fmt.Errorf("planning evaluator failed: %s", evaluation.Summary))
	}
	if evaluation.NeedsUserInput {
		result.Status = StatusNeedsUserInput
		result.NeedsUserInput = true
		if err := writeJSONFile(filepath.Join(req.OutputDir, "planning-result.json"), result); err != nil {
			return failResult(result, err)
		}
		result.Artifacts = append(result.Artifacts, filepath.Join(req.OutputDir, "planning-result.json"))
		return result, nil
	}

	splitterOutput, err := runRole(ctx, runner, req, "task_splitter", 3, buildContextPacket(req, result.Agents), result.Agents, blackboardPath, true)
	if err != nil {
		return failResult(result, err)
	}
	result.Agents = append(result.Agents, splitterOutput)
	if splitterOutput.TaskGraph == nil {
		return failResult(result, fmt.Errorf("task_splitter did not provide task_graph"))
	}
	graph := *splitterOutput.TaskGraph
	if err := ValidateTaskGraph(graph); err != nil {
		if !req.AutoRepair {
			return failResult(result, err)
		}
		repaired, actions, repairErr := RepairTaskGraph(graph)
		if repairErr != nil {
			return failResult(result, fmt.Errorf("%w; auto repair failed: %v", err, repairErr))
		}
		graph = repaired
		splitterOutput.TaskGraph = &graph
		if result.Metadata == nil {
			result.Metadata = map[string]any{}
		}
		result.Metadata["auto_repair_actions"] = actions
	}
	if err := ValidateTaskGraph(graph); err != nil {
		return failResult(result, err)
	}
	taskGraphPath := filepath.Join(req.OutputDir, "task-graph.json")
	if err := writeJSONFile(taskGraphPath, &graph); err != nil {
		return failResult(result, err)
	}
	rendered, err := RenderArtifacts(req.OutputDir, graph, req.ProjectRules)
	if err != nil {
		return failResult(result, err)
	}
	result.Status = StatusCompleted
	result.TaskGraph = &graph
	result.TaskGraphPath = taskGraphPath
	result.MasterPath = filepath.Join(req.OutputDir, "MASTER.md")
	result.Artifacts = append(result.Artifacts, taskGraphPath)
	result.Artifacts = append(result.Artifacts, rendered...)
	resultPath := filepath.Join(req.OutputDir, "planning-result.json")
	if err := writeJSONFile(resultPath, result); err != nil {
		return failResult(result, err)
	}
	result.Artifacts = append(result.Artifacts, resultPath)
	return result, nil
}

func ValidateRequest(req Request) error {
	return normalizeRequest(&req)
}

func normalizeRequest(req *Request) error {
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	req.Mode = strings.ToLower(strings.TrimSpace(req.Mode))
	if req.Mode == "" {
		req.Mode = "full"
	}
	switch req.Mode {
	case "full", "lite", "oneshot", "one-shot":
	default:
		return fmt.Errorf("unsupported planning mode %q", req.Mode)
	}
	if len(strings.Fields(req.ProjectContext)) < 8 {
		return fmt.Errorf("project_context is required and must be substantive")
	}
	if strings.TrimSpace(req.OutputDir) == "" {
		return fmt.Errorf("output_dir is required")
	}
	req.OutputDir = filepath.Clean(req.OutputDir)
	if req.SessionID == "" {
		req.SessionID = newSessionID(req.Prompt)
	}
	if len(req.Roles) == 0 {
		req.Roles = append([]string{}, DefaultExplorationRoles...)
	}
	req.Roles = normalizeRoles(req.Roles)
	if req.HumanGatePolicy == "" {
		req.HumanGatePolicy = HumanGateMaterialAmbiguity
	}
	switch req.HumanGatePolicy {
	case HumanGateMaterialAmbiguity, HumanGateAlwaysBeforeSplit, HumanGateNever:
	default:
		return fmt.Errorf("unsupported human_gate_policy %q", req.HumanGatePolicy)
	}
	if req.ProjectRules.Status == "" {
		req.ProjectRules.Status = "missing"
	}
	if req.ProjectRules.Digest == "" {
		req.ProjectRules.Digest = "none"
	}
	if req.ProjectRules.Read == "" {
		req.ProjectRules.Read = "yes"
	}
	return nil
}

func runRoles(ctx context.Context, runner Runner, req Request, roles []string, round int, contextPacket string, previous []AgentOutput, blackboardPath string) ([]AgentOutput, error) {
	type roleResult struct {
		index  int
		output AgentOutput
		err    error
	}
	ch := make(chan roleResult, len(roles))
	var wg sync.WaitGroup
	for i, role := range roles {
		wg.Add(1)
		go func(index int, role string) {
			defer wg.Done()
			output, err := runRole(ctx, runner, req, role, round, contextPacket, previous, blackboardPath, false)
			ch <- roleResult{index: index, output: output, err: err}
		}(i, role)
	}
	wg.Wait()
	close(ch)
	results := make([]AgentOutput, len(roles))
	for item := range ch {
		if item.err != nil {
			return nil, item.err
		}
		results[item.index] = item.output
	}
	for _, output := range results {
		if err := appendBlackboard(blackboardPath, output); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func runRole(ctx context.Context, runner Runner, req Request, role string, round int, contextPacket string, previous []AgentOutput, blackboardPath string, recordBlackboard bool) (AgentOutput, error) {
	outPath := filepath.Join(req.OutputDir, "agent-outputs", role+".json")
	agentReq := AgentRequest{
		SessionID:       req.SessionID,
		RunID:           req.RunID,
		Role:            role,
		Round:           round,
		ContextPacket:   contextPacket,
		OutputDir:       req.OutputDir,
		OutputPath:      outPath,
		BlackboardPath:  blackboardPath,
		PreviousOutputs: previous,
		ProjectRules:    req.ProjectRules,
	}
	if err := runner.RunPlanningAgent(ctx, agentReq); err != nil {
		return AgentOutput{}, err
	}
	output, err := readJSONFile[AgentOutput](outPath)
	if err != nil {
		return AgentOutput{}, fmt.Errorf("read planning output for %s: %w", role, err)
	}
	if err := validateAgentOutput(output, role); err != nil {
		return AgentOutput{}, err
	}
	if recordBlackboard {
		if err := appendBlackboard(blackboardPath, output); err != nil {
			return AgentOutput{}, err
		}
	}
	return output, nil
}

func validateAgentOutput(output AgentOutput, expectedRole string) error {
	if output.Role != expectedRole {
		return fmt.Errorf("agent output role %q does not match expected %q", output.Role, expectedRole)
	}
	if strings.TrimSpace(output.Provider) == "" || strings.TrimSpace(output.Model) == "" || strings.TrimSpace(output.SessionID) == "" {
		return fmt.Errorf("agent %s missing provider/model/session_id", expectedRole)
	}
	if output.Confidence <= 0 || output.Confidence > 1 {
		return fmt.Errorf("agent %s confidence must be between 0 and 1", expectedRole)
	}
	if len(strings.Fields(output.Summary)) < 3 {
		return fmt.Errorf("agent %s summary is required and must be concrete", expectedRole)
	}
	if expectedRole == "planning_evaluator" {
		switch strings.ToUpper(output.Verdict) {
		case "PASS", "NEEDS_USER_INPUT", "FAIL":
		default:
			return fmt.Errorf("planning_evaluator invalid verdict %q", output.Verdict)
		}
	}
	return nil
}

func buildEvaluation(output AgentOutput, policy string) Evaluation {
	needsInput := false
	if strings.EqualFold(output.Verdict, "NEEDS_USER_INPUT") {
		needsInput = true
	}
	if policy == HumanGateAlwaysBeforeSplit {
		needsInput = true
	}
	if policy == HumanGateNever {
		needsInput = false
	}
	if policy == HumanGateMaterialAmbiguity {
		for _, q := range output.Questions {
			if q.Material {
				needsInput = true
				break
			}
		}
	}
	issues := []string{}
	if strings.EqualFold(output.Verdict, "FAIL") {
		issues = append(issues, output.Summary)
	}
	return Evaluation{
		Verdict:        strings.ToUpper(output.Verdict),
		Score:          output.Score,
		NeedsUserInput: needsInput,
		Questions:      output.Questions,
		Issues:         issues,
		Summary:        output.Summary,
		Raw:            output,
	}
}

func appendBlackboard(path string, output AgentOutput) error {
	entries := []map[string]any{{
		"role": output.Role, "kind": "agent_summary", "title": output.Role, "summary": output.Summary, "full": output,
	}}
	for _, proposal := range output.Proposals {
		entries = append(entries, map[string]any{"role": output.Role, "kind": "proposal", "title": proposal.Title, "summary": proposal.Summary, "full": proposal})
	}
	for _, risk := range output.Risks {
		entries = append(entries, map[string]any{"role": output.Role, "kind": "risk", "title": risk.Title, "summary": risk.Impact, "full": risk})
	}
	for _, question := range output.Questions {
		entries = append(entries, map[string]any{"role": output.Role, "kind": "question", "title": question.Question, "summary": question.Impact, "full": question})
	}
	for _, claim := range output.Claims {
		entries = append(entries, map[string]any{"role": output.Role, "kind": claim.Kind, "title": claim.Title, "summary": claim.Summary, "full": claim})
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			return err
		}
	}
	return nil
}

func buildContextPacket(req Request, previous []AgentOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Prompt:\n%s\n\n", req.Prompt)
	fmt.Fprintf(&b, "Mode: %s\n", req.Mode)
	fmt.Fprintf(&b, "Human gate policy: %s\n", req.HumanGatePolicy)
	fmt.Fprintf(&b, "PROJECT_RULES_STATUS: %s\nPROJECT_RULES_DIGEST: %s\nPROJECT_RULES_READ: %s\n\n",
		req.ProjectRules.Status, req.ProjectRules.Digest, req.ProjectRules.Read)
	fmt.Fprintf(&b, "Project context:\n%s\n", req.ProjectContext)
	if len(previous) > 0 {
		fmt.Fprintln(&b, "\nPrevious planning outputs:")
		for _, output := range previous {
			fmt.Fprintf(&b, "- %s: %s\n", output.Role, output.Summary)
		}
	}
	return b.String()
}

func failResult(result Result, err error) (Result, error) {
	result.OK = false
	result.Status = StatusFailed
	if result.OutputDir != "" {
		resultPath := filepath.Join(result.OutputDir, "planning-result.json")
		_ = writeJSONFile(resultPath, result)
	}
	return result, err
}

func normalizeRoles(roles []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(role)
		if role == "" || seen[role] || role == "planning_evaluator" || role == "task_splitter" {
			continue
		}
		seen[role] = true
		out = append(out, role)
	}
	if len(out) == 0 {
		return append([]string{}, DefaultExplorationRoles...)
	}
	return out
}

func newSessionID(prompt string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", prompt, time.Now().UnixNano())))
	return "plan-" + hex.EncodeToString(sum[:])[:16]
}
