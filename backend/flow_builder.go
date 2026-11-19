package main

import (
	"fmt"
	"regexp"
	"strings"
)

var asciiIdentRegex = regexp.MustCompile(`^[a-z0-9_]+$`)

type graphState struct {
	nodes []Node
	edges []Edge
	tools []ToolDefinition
}

var supportedNodeTypes = map[string]bool{
	"lookup":   true,
	"decision": true,
	"action":   true,
	"end":      true,
}

func matchesToolKeyword(lowerText, keyword string) bool {
	kw := strings.ToLower(strings.TrimSpace(keyword))
	if kw == "" {
		return false
	}
	if asciiIdentRegex.MatchString(kw) {
		rx, err := regexp.Compile(`\b` + regexp.QuoteMeta(kw) + `\b`)
		if err == nil {
			return rx.MatchString(lowerText)
		}
	}
	return strings.Contains(lowerText, kw)
}

func MatchToolByCatalog(nodeType, label string, tools []ToolDefinition) string {
	if nodeType != "lookup" && nodeType != "action" {
		return "none"
	}
	lower := strings.ToLower(label)
	isCloseCase := strings.Contains(lower, "ปิดเคส") && !strings.Contains(lower, "เปิดเคส")

	bestID := ""
	bestScore := 0
	for _, t := range tools {
		if !t.Enabled || t.Category != nodeType {
			continue
		}
		score := 0
		for _, kw := range t.Keywords {
			if (t.ID == "ticket" || t.ID == "jira_ticket") && strings.TrimSpace(kw) == "เคส" && isCloseCase {
				continue
			}
			if matchesToolKeyword(lower, kw) {
				score += len(strings.TrimSpace(kw)) + 1
			}
		}
		if t.Name != "" && matchesToolKeyword(lower, t.Name) {
			score += len(t.Name) + 2
		}
		if score > bestScore {
			bestScore = score
			bestID = t.ID
		}
	}

	if bestID != "" {
		return bestID
	}
	if nodeType == "lookup" {
		return "manual_lookup"
	}
	return "manual_action"
}

func MatchToolByKeywords(nodeType, label string) string {
	return MatchToolByCatalog(nodeType, label, DefaultBuiltinTools())
}

func ResolveStepTool(step Step, tools []ToolDefinition) (string, string) {
	if step.Type != "lookup" && step.Type != "action" {
		return "none", ""
	}

	rawTool := strings.TrimSpace(step.Tool)
	if rawTool != "" {
		norm := strings.ToLower(NormalizeLegacyToolID(rawTool))
		for _, t := range tools {
			if !t.Enabled {
				continue
			}
			if strings.ToLower(t.ID) == norm || (t.Name != "" && strings.ToLower(t.Name) == norm) {
				return t.ID, "ai"
			}
		}
		if norm == "manual_lookup" || norm == "manual_action" {
			kwMatch := MatchToolByCatalog(step.Type, step.Label, tools)
			if kwMatch != "manual_lookup" && kwMatch != "manual_action" {
				return kwMatch, "keyword"
			}
			return norm, "ai"
		}
	}

	kwMatch := MatchToolByCatalog(step.Type, step.Label, tools)
	if kwMatch != "manual_lookup" && kwMatch != "manual_action" {
		return kwMatch, "keyword"
	}
	return kwMatch, "manual"
}

func (g *graphState) findToolByID(toolID string) *ToolDefinition {
	for i := range g.tools {
		if g.tools[i].ID == toolID {
			return &g.tools[i]
		}
	}
	return nil
}

func (g *graphState) appendNode(nodeType, label, tool, toolSource string) string {
	id := fmt.Sprintf("n%d", len(g.nodes)+1)
	node := Node{
		ID:         id,
		Type:       nodeType,
		Label:      label,
		Tool:       tool,
		ToolSource: toolSource,
	}
	if toolDef := g.findToolByID(tool); toolDef != nil && (nodeType == "lookup" || len(toolDef.Params) > 0) {
		if toolDef.PayloadMode != "" {
			node.PayloadMode = toolDef.PayloadMode
		} else if len(toolDef.Params) > 0 {
			node.PayloadMode = "dynamic"
		}
		if len(toolDef.Params) > 0 {
			node.ParamModes = make(map[string]string, len(toolDef.Params))
			node.InputPayload = make(map[string]string, len(toolDef.Params))
			for _, p := range toolDef.Params {
				pMode := p.Mode
				if pMode == "" {
					pMode = node.PayloadMode
				}
				if pMode == "" {
					pMode = "dynamic"
				}
				node.ParamModes[p.Key] = pMode
				val := p.Value
				if val == "" {
					val = p.DefaultValue
				}
				if pMode == "fixed" || (strings.Contains(val, "{{") && strings.Contains(val, "}}")) {
					if val != "" {
						node.InputPayload[p.Key] = val
					}
				}
			}
			if len(node.InputPayload) == 0 {
				node.InputPayload = nil
			}
		}
	}
	g.nodes = append(g.nodes, node)
	return id
}

func (g *graphState) connectNodes(fromID, toID, label string) {
	if toID == "" {
		return
	}
	for i := range g.edges {
		existing := &g.edges[i]
		if existing.Source == fromID && existing.Target == toID {
			if label != "" {
				if existing.Label == "" {
					existing.Label = label
				} else {
					existing.Label += " / " + label
				}
			}
			return
		}
	}
	edgeID := fmt.Sprintf("e%d", len(g.edges)+1)
	g.edges = append(g.edges, Edge{ID: edgeID, Source: fromID, Target: toID, Label: label})
}

func (g *graphState) buildStepSequence(steps []Step, nextID string) string {
	for i, s := range steps {
		if s.Type == "end" {
			steps = steps[:i+1]
			break
		}
	}
	if len(steps) == 0 {
		if nextID != "" {
			return nextID
		}
		return g.appendNode("terminal", "จบ", "none", "")
	}
	entryID := nextID
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		if !supportedNodeTypes[step.Type] {
			step.Type = "action"
		}
		if step.Type == "decision" && len(step.Branches) == 0 {
			step.Type = "action"
		}
		tool, source := ResolveStepTool(step, g.tools)
		id := g.appendNode(step.Type, step.Label, tool, source)
		switch step.Type {
		case "decision":
			for _, br := range step.Branches {
				g.connectNodes(id, g.buildStepSequence(br.Steps, entryID), br.Label)
			}
		case "end":
		default:
			g.connectNodes(id, entryID, "")
		}
		entryID = id
	}
	return entryID
}

func BuildCustomFlowchart(steps []Step, tools []ToolDefinition) ([]Node, []Edge) {
	state := &graphState{tools: tools}
	startID := state.appendNode("terminal", "เริ่ม", "none", "")
	endID := state.appendNode("terminal", "จบ", "none", "")
	state.connectNodes(startID, state.buildStepSequence(steps, endID), "")

	endHasIncoming := false
	for _, e := range state.edges {
		if e.Target == endID {
			endHasIncoming = true
			break
		}
	}
	if !endHasIncoming {
		filtered := make([]Node, 0, len(state.nodes)-1)
		for _, n := range state.nodes {
			if n.ID != endID {
				filtered = append(filtered, n)
			}
		}
		state.nodes = filtered
	}

	return sortNodesTopological(state.nodes, state.edges, startID), state.edges
}

func sortNodesTopological(nodes []Node, edges []Edge, startID string) []Node {
	nodeMap := make(map[string]Node, len(nodes))
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
	}

	visited := make(map[string]bool, len(nodes))
	ordered := make([]Node, 0, len(nodes))
	queue := []string{startID}
	visited[startID] = true

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if n, ok := nodeMap[curr]; ok {
			ordered = append(ordered, n)
		}
		for _, next := range adj[curr] {
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}

	for _, n := range nodes {
		if !visited[n.ID] {
			ordered = append(ordered, n)
		}
	}
	return ordered
}
