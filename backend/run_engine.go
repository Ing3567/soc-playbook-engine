package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrRunNotFound       = errors.New("simulation run not found")
	ErrRunAlreadyClosed  = errors.New("simulation run is already completed")
	ErrRunNodeNotFound   = errors.New("node not found in run playbook")
	ErrRunNodeNotReady   = errors.New("node is locked or skipped")
	ErrRunInvalidRequest = errors.New("invalid simulation run request")
)

type RunNodeGraphState struct {
	Availability   string         `json:"availability"`
	UpstreamOutput map[string]any `json:"upstream_output,omitempty"`
	LockedReason   string         `json:"locked_reason,omitempty"`
}

type RunGraphState struct {
	NodeStates     map[string]RunNodeGraphState `json:"node_states"`
	TraversedEdges []string                     `json:"traversed_edges"`
	SkippedEdges   []string                     `json:"skipped_edges"`
}

func getDownstreamNodeIDs(startID string, edges []Edge) []string {
	outgoing := make(map[string][]string)
	for _, e := range edges {
		outgoing[e.Source] = append(outgoing[e.Source], e.Target)
	}
	visited := make(map[string]bool)
	queue := append([]string(nil), outgoing[startID]...)
	var out []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == startID || visited[cur] {
			continue
		}
		visited[cur] = true
		out = append(out, cur)
		for _, nxt := range outgoing[cur] {
			if !visited[nxt] {
				queue = append(queue, nxt)
			}
		}
	}
	return out
}

// ComputeRunGraphState computes which nodes are ready, locked, or skipped on the server,
// matching frontend/src/pipeline.ts semantics and using structured EdgeCondition rules.
func ComputeRunGraphState(
	nodes []Node,
	edges []Edge,
	statuses map[string]string,
	nodeOutputs map[string]map[string]any,
	alert map[string]any,
) RunGraphState {
	nodeStates := make(map[string]RunNodeGraphState, len(nodes))
	traversedSet := make(map[string]bool)
	skippedSet := make(map[string]bool)
	var traversed []string
	var skipped []string

	if len(nodes) == 0 {
		return RunGraphState{NodeStates: nodeStates, TraversedEdges: traversed, SkippedEdges: skipped}
	}

	nodeMap := make(map[string]Node, len(nodes))
	outgoing := make(map[string][]Edge)
	inDegree := make(map[string]int, len(nodes))
	for _, n := range nodes {
		nodeMap[n.ID] = n
		inDegree[n.ID] = 0
	}
	for _, e := range edges {
		outgoing[e.Source] = append(outgoing[e.Source], e)
		inDegree[e.Target]++
	}

	startNode := nodes[0]
	for _, n := range nodes {
		if inDegree[n.ID] == 0 {
			startNode = n
			break
		}
	}

	nodesCtx := make(map[string]any, len(nodeOutputs))
	for k, v := range nodeOutputs {
		nodesCtx[k] = v
	}
	fullCtx := map[string]any{
		"alert": alert,
		"nodes": nodesCtx,
	}

	type queueItem struct {
		id      string
		lastOut map[string]any
	}

	visitedActive := make(map[string]bool)
	blockingLabels := make(map[string]string)
	queue := []queueItem{{id: startNode.ID, lastOut: nil}}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		current, exists := nodeMap[item.id]
		if !exists || visitedActive[item.id] {
			continue
		}
		visitedActive[item.id] = true
		outEdges := outgoing[item.id]

		switch current.Type {
		case "terminal":
			nodeStates[item.id] = RunNodeGraphState{Availability: "ready", UpstreamOutput: item.lastOut}
			for _, e := range outEdges {
				if !traversedSet[e.ID] {
					traversedSet[e.ID] = true
					traversed = append(traversed, e.ID)
				}
				queue = append(queue, queueItem{id: e.Target, lastOut: item.lastOut})
			}
		case "end":
			nodeStates[item.id] = RunNodeGraphState{Availability: "ready", UpstreamOutput: item.lastOut}
		case "decision":
			nodeStates[item.id] = RunNodeGraphState{Availability: "ready", UpstreamOutput: item.lastOut}
			chosen := PickDecisionEdge(outEdges, item.lastOut, fullCtx)
			for _, e := range outEdges {
				if chosen != nil && e.ID == chosen.ID {
					if !traversedSet[e.ID] {
						traversedSet[e.ID] = true
						traversed = append(traversed, e.ID)
					}
					queue = append(queue, queueItem{id: e.Target, lastOut: item.lastOut})
				} else if !skippedSet[e.ID] {
					skippedSet[e.ID] = true
					skipped = append(skipped, e.ID)
				}
			}
		default:
			nodeStates[item.id] = RunNodeGraphState{Availability: "ready", UpstreamOutput: item.lastOut}
			if statuses[item.id] == "success" {
				myOut := nodeOutputs[item.id]
				if myOut == nil {
					myOut = item.lastOut
				}
				for _, e := range outEdges {
					if !traversedSet[e.ID] {
						traversedSet[e.ID] = true
						traversed = append(traversed, e.ID)
					}
					queue = append(queue, queueItem{id: e.Target, lastOut: myOut})
				}
			} else {
				for _, dID := range getDownstreamNodeIDs(item.id, edges) {
					if _, exists := blockingLabels[dID]; !exists {
						blockingLabels[dID] = current.Label
					}
				}
			}
		}
	}

	reachable := map[string]bool{startNode.ID: true}
	reachQ := []string{startNode.ID}
	for len(reachQ) > 0 {
		cur := reachQ[0]
		reachQ = reachQ[1:]
		for _, e := range outgoing[cur] {
			if skippedSet[e.ID] {
				continue
			}
			if !reachable[e.Target] {
				reachable[e.Target] = true
				reachQ = append(reachQ, e.Target)
			}
		}
	}

	for _, n := range nodes {
		if _, done := nodeStates[n.ID]; done {
			continue
		}
		if !reachable[n.ID] {
			nodeStates[n.ID] = RunNodeGraphState{
				Availability: "skipped",
				LockedReason: "กิ่งนี้ไม่ถูกเลือกตามผลลัพธ์ของโหนดก่อนหน้า",
			}
		} else {
			reason := "ต้องรันโหนดด้านบนให้สำเร็จก่อน"
			if blocker, ok := blockingLabels[n.ID]; ok && blocker != "" {
				reason = fmt.Sprintf(`ต้องรัน "%s" ให้สำเร็จก่อน`, blocker)
			}
			nodeStates[n.ID] = RunNodeGraphState{
				Availability: "locked",
				LockedReason: reason,
			}
		}
	}

	return RunGraphState{
		NodeStates:     nodeStates,
		TraversedEdges: traversed,
		SkippedEdges:   skipped,
	}
}

func ExecuteRunNode(
	run *SimulationRun,
	nodeID string,
	stepReq RunStepRequest,
	toolDef *ToolDefinition,
	scenario *Scenario,
	allowLive bool,
) (RunEvent, RunGraphState, error) {
	if run == nil {
		return RunEvent{}, RunGraphState{}, ErrRunNotFound
	}
	if run.Status != "in_progress" {
		return RunEvent{}, RunGraphState{}, ErrRunAlreadyClosed
	}

	var target *Node
	for i := range run.Nodes {
		if run.Nodes[i].ID == nodeID {
			target = &run.Nodes[i]
			break
		}
	}
	if target == nil {
		return RunEvent{}, RunGraphState{}, ErrRunNodeNotFound
	}
	if target.Type != "lookup" && target.Type != "action" {
		return RunEvent{}, RunGraphState{}, fmt.Errorf("%w: node type %q cannot be executed", ErrRunInvalidRequest, target.Type)
	}

	if run.NodeStatuses == nil {
		run.NodeStatuses = make(map[string]string)
	}
	if run.NodeOutputs == nil {
		run.NodeOutputs = make(map[string]map[string]any)
	}

	beforeState := ComputeRunGraphState(run.Nodes, run.Edges, run.NodeStatuses, run.NodeOutputs, run.Alert)
	nodeState, ok := beforeState.NodeStates[nodeID]
	if !ok || nodeState.Availability != "ready" {
		return RunEvent{}, beforeState, fmt.Errorf("%w: %s", ErrRunNodeNotReady, nodeState.LockedReason)
	}

	// Re-running an already completed node invalidates downstream states.
	if run.NodeStatuses[nodeID] == "success" {
		for _, dID := range getDownstreamNodeIDs(nodeID, run.Edges) {
			delete(run.NodeStatuses, dID)
			delete(run.NodeOutputs, dID)
		}
	}

	canonicalTool := NormalizeLegacyToolID(target.Tool)
	isManual := strings.TrimSpace(stepReq.ManualVerdict) != "" ||
		(toolDef != nil && toolDef.CommandType == "manual") ||
		canonicalTool == "manual_lookup" ||
		canonicalTool == "manual_action"

	now := time.Now()
	var event RunEvent
	if isManual {
		verdict := strings.ToLower(strings.TrimSpace(stepReq.ManualVerdict))
		if verdict == "" {
			return RunEvent{}, beforeState, fmt.Errorf("%w: manual node requires manual_verdict (clean, suspicious, malicious, or done)", ErrRunInvalidRequest)
		}
		if verdict != "clean" && verdict != "suspicious" && verdict != "malicious" && verdict != "done" {
			return RunEvent{}, beforeState, fmt.Errorf("%w: invalid manual_verdict %q", ErrRunInvalidRequest, verdict)
		}

		toolDisplay := canonicalTool
		if toolDef != nil && toolDef.Name != "" {
			toolDisplay = toolDef.Name
		}
		verdictLabel := "Completed (Manual Action)"
		isMalicious := false
		switch verdict {
		case "clean":
			verdictLabel = "Clean / Benign (Analyst Confirmed)"
		case "suspicious", "malicious":
			verdictLabel = "Malicious / Suspicious (Analyst Confirmed)"
			isMalicious = true
		}

		out := map[string]any{
			"mode":         "manual",
			"tool":         toolDisplay,
			"step":         target.Label,
			"verdict":      verdictLabel,
			"is_malicious": isMalicious,
			"completed":    true,
		}
		if notes := strings.TrimSpace(stepReq.Notes); notes != "" {
			out["notes"] = notes
		}

		run.NodeStatuses[nodeID] = "success"
		run.NodeOutputs[nodeID] = out
		event = RunEvent{
			ID:        GeneratePrefixedID("evt"),
			RunID:     run.ID,
			NodeID:    target.ID,
			NodeLabel: target.Label,
			Action:    "manual_verdict",
			Status:    "success",
			Verdict:   verdict,
			Notes:     strings.TrimSpace(stepReq.Notes),
			Output:    out,
			Timestamp: now,
		}
	} else {
		nodesCtx := make(map[string]any, len(run.NodeOutputs))
		for k, v := range run.NodeOutputs {
			nodesCtx[k] = v
		}
		prevOut := nodeState.UpstreamOutput
		if prevOut == nil {
			prevOut = map[string]any{}
		}
		execReq := ExecutionRequest{
			NodeID:        target.ID,
			Type:          target.Type,
			Label:         target.Label,
			Tool:          canonicalTool,
			PayloadMode:   target.PayloadMode,
			ParamModes:    target.ParamModes,
			InputPayload:  target.InputPayload,
			RuntimeInputs: stepReq.RuntimeInputs,
			RuntimeContext: map[string]any{
				"alert": run.Alert,
				"prev":  prevOut,
				"nodes": nodesCtx,
			},
			RuntimeAuth:   stepReq.RuntimeAuth,
			ExecutionMode: run.Mode,
			ScenarioID:    run.ScenarioID,
		}

		var res ExecutionResult
		if strings.EqualFold(run.Mode, "live") && allowLive {
			res = RunLiveHTTPStep(execReq, toolDef)
		} else {
			res = RunSandboxStepWithScenario(execReq, toolDef, scenario)
		}

		nextStatus := "success"
		if res.Status == "failed" {
			nextStatus = "failed"
		}
		run.NodeStatuses[nodeID] = nextStatus
		if nextStatus == "success" {
			run.NodeOutputs[nodeID] = res.Output
		}

		// Store sanitized non-secret runtime inputs in audit log.
		sanitizedInputs := sanitizeAuditInputs(stepReq.RuntimeInputs, toolDef)
		event = RunEvent{
			ID:         GeneratePrefixedID("evt"),
			RunID:      run.ID,
			NodeID:     target.ID,
			NodeLabel:  target.Label,
			Action:     "step_executed",
			Status:     nextStatus,
			Input:      sanitizedInputs,
			Output:     res.Output,
			DurationMs: res.DurationMs,
			Timestamp:  now,
		}
	}

	afterState := ComputeRunGraphState(run.Nodes, run.Edges, run.NodeStatuses, run.NodeOutputs, run.Alert)
	run.TraversedEdges = afterState.TraversedEdges
	run.UpdatedAt = now
	return event, afterState, nil
}

func sanitizeAuditInputs(inputs map[string]string, toolDef *ToolDefinition) map[string]any {
	if len(inputs) == 0 {
		return nil
	}
	secretKeys := make(map[string]bool)
	authKey := ""
	if toolDef != nil {
		if toolDef.Auth != nil {
			authKey = strings.ToLower(strings.TrimSpace(toolDef.Auth.KeyName))
		}
		for _, p := range toolDef.Params {
			if p.IsSecret {
				secretKeys[p.Key] = true
			}
		}
	}
	out := make(map[string]any, len(inputs))
	for k, v := range inputs {
		if secretKeys[k] || (authKey != "" && strings.EqualFold(k, authKey)) {
			out[k] = "***"
		} else {
			out[k] = v
		}
	}
	return out
}
