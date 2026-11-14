package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	vtWordRegex    = regexp.MustCompile(`\b(virustotal|vt)\b`)
	blockWordRegex = regexp.MustCompile(`\b(block|firewall)\b`)
	ticketWordRegex = regexp.MustCompile(`\b(ticket|tier)\b`)
)

type graphState struct {
	nodes []Node
	edges []Edge
}

var supportedNodeTypes = map[string]bool{
	"lookup":   true,
	"decision": true,
	"action":   true,
	"end":      true,
}

func MatchToolByKeywords(nodeType, label string) string {
	lower := strings.ToLower(label)
	switch nodeType {
	case "lookup":
		if vtWordRegex.MatchString(lower) {
			return "virustotal"
		}
		if strings.Contains(lower, "abuseipdb") {
			return "abuseipdb"
		}
		if strings.Contains(lower, "jira") || strings.Contains(lower, "เคสเก่า") {
			return "jira_search"
		}
		return "manual_lookup"
	case "action":
		if strings.Contains(lower, "บล็อก") || strings.Contains(lower, "ระงับ") || blockWordRegex.MatchString(lower) {
			return "firewall_block"
		}
		isCloseCase := strings.Contains(lower, "ปิดเคส") && !strings.Contains(lower, "เปิดเคส")
		if ticketWordRegex.MatchString(lower) || strings.Contains(lower, "เปิดเคส") || (strings.Contains(lower, "เคส") && !isCloseCase) {
			return "jira_ticket"
		}
		return "manual_action"
	default:
		return "none"
	}
}

func (g *graphState) appendNode(nodeType, label string) string {
	id := fmt.Sprintf("n%d", len(g.nodes)+1)
	g.nodes = append(g.nodes, Node{
		ID:    id,
		Type:  nodeType,
		Label: label,
		Tool:  MatchToolByKeywords(nodeType, label),
	})
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
		return g.appendNode("terminal", "จบ")
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
		id := g.appendNode(step.Type, step.Label)
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

func BuildCustomFlowchart(steps []Step) ([]Node, []Edge) {
	state := &graphState{}
	startID := state.appendNode("terminal", "เริ่ม")
	endID := state.appendNode("terminal", "จบ")
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