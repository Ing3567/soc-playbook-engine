package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"
)

type CacaoPlaybook struct {
	Type          string                   `json:"type"`
	SpecVersion   string                   `json:"spec_version"`
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	PlaybookTypes []string                 `json:"playbook_types"`
	CreatedBy     string                   `json:"created_by"`
	Created       string                   `json:"created"`
	Modified      string                   `json:"modified"`
	WorkflowStart string                   `json:"workflow_start"`
	Workflow      map[string]CacaoStepItem `json:"workflow"`
}

type CacaoStepItem struct {
	Type         string            `json:"type"`
	Name         string            `json:"name"`
	OnCompletion string            `json:"on_completion,omitempty"`
	Condition    string            `json:"condition,omitempty"`
	OnTrue       string            `json:"on_true,omitempty"`
	OnFalse      string            `json:"on_false,omitempty"`
	Switch       string            `json:"switch,omitempty"`
	Cases        map[string]string `json:"cases,omitempty"`
	Commands     []CacaoCommand    `json:"commands,omitempty"`
}

type CacaoCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func newUUIDv4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func cacaoStepType(n Node, isStart bool) string {
	if isStart {
		return "start"
	}
	if n.Type == "terminal" || n.Type == "end" {
		return "end"
	}
	if n.Type == "decision" {
		return "if-condition"
	}
	return "action"
}

func cacaoCommandType(tool string, toolMap map[string]ToolDefinition) string {
	if def, ok := toolMap[tool]; ok && def.CommandType != "" {
		return def.CommandType
	}
	switch tool {
	case "virustotal", "abuseipdb", "jira_search", "firewall_block", "jira_ticket":
		return "http-api"
	default:
		return "manual"
	}
}

func BuildCacaoJSON(title string, nodes []Node, edges []Edge, toolMaps ...map[string]ToolDefinition) ([]byte, error) {
	var toolMap map[string]ToolDefinition
	if len(toolMaps) > 0 && toolMaps[0] != nil {
		toolMap = toolMaps[0]
	} else {
		toolMap = make(map[string]ToolDefinition)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	workflow := make(map[string]CacaoStepItem, len(nodes))
	outgoing := make(map[string][]Edge)
	inDegree := make(map[string]int, len(nodes))

	for _, e := range edges {
		outgoing[e.Source] = append(outgoing[e.Source], e)
		inDegree[e.Target]++
	}

	startNodeID := ""
	for _, n := range nodes {
		if inDegree[n.ID] == 0 {
			startNodeID = n.ID
			break
		}
	}
	if startNodeID == "" && len(nodes) > 0 {
		startNodeID = nodes[0].ID
	}

	stepIDs := make(map[string]string, len(nodes))
	for _, n := range nodes {
		outs := outgoing[n.ID]
		st := cacaoStepType(n, n.ID == startNodeID)
		if n.Type == "decision" && len(outs) > 2 {
			st = "switch-condition"
		}
		stepIDs[n.ID] = st + "--" + newUUIDv4()
	}

	for _, n := range nodes {
		stepID := stepIDs[n.ID]
		outs := outgoing[n.ID]
		var item CacaoStepItem

		if n.ID == startNodeID {
			item = CacaoStepItem{Type: "start", Name: n.Label}
			if len(outs) > 0 {
				item.OnCompletion = stepIDs[outs[0].Target]
			}
		} else if n.Type == "terminal" || n.Type == "end" {
			item = CacaoStepItem{Type: "end", Name: n.Label}
		} else if n.Type == "decision" && len(outs) <= 2 {
			item = CacaoStepItem{
				Type:      "if-condition",
				Name:      n.Label,
				Condition: n.Label,
			}
			if len(outs) >= 1 {
				item.OnTrue = stepIDs[outs[0].Target]
			}
			if len(outs) >= 2 {
				item.OnFalse = stepIDs[outs[1].Target]
			}
		} else if n.Type == "decision" {
			cases := make(map[string]string, len(outs))
			for idx, br := range outs {
				key := br.Label
				if key == "" {
					key = fmt.Sprintf("branch_%d", idx+1)
				}
				cases[key] = stepIDs[br.Target]
			}
			item = CacaoStepItem{
				Type:   "switch-condition",
				Name:   n.Label,
				Switch: n.Label,
				Cases:  cases,
			}
		} else {
			cmdText := n.Label
			if def, ok := toolMap[n.Tool]; ok && def.Endpoint != "" {
				resolved := BuildToolRequest(&def, ExecutionRequest{
					NodeID:       n.ID,
					Type:         n.Type,
					Label:        n.Label,
					Tool:         n.Tool,
					PayloadMode:  n.PayloadMode,
					InputPayload: n.InputPayload,
				})
				targetURL := def.Endpoint
				if n.PayloadMode == "fixed" && resolved.URL != "" {
					targetURL = resolved.URL
				}
				cmdText = fmt.Sprintf("[%s] %s %s (%s)", def.Name, resolved.Method, targetURL, n.Label)
			}
			item = CacaoStepItem{
				Type:     "action",
				Name:     n.Label,
				Commands: []CacaoCommand{{Type: cacaoCommandType(n.Tool, toolMap), Command: cmdText}},
			}
			if len(outs) > 0 {
				item.OnCompletion = stepIDs[outs[0].Target]
			}
		}
		workflow[stepID] = item
	}

	pb := CacaoPlaybook{
		Type:          "playbook",
		SpecVersion:   "cacao-2.0",
		ID:            "playbook--" + newUUIDv4(),
		Name:          title,
		PlaybookTypes: []string{"investigation"},
		CreatedBy:     "identity--" + newUUIDv4(),
		Created:       now,
		Modified:      now,
		WorkflowStart: stepIDs[startNodeID],
		Workflow:      workflow,
	}
	return json.MarshalIndent(pb, "", "  ")
}
