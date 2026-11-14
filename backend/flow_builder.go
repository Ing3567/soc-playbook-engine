package main

import (
	"fmt"
	"strings"
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
		if strings.Contains(lower, "virustotal") || strings.Contains(lower, "vt") {
			return "virustotal"
		}
		if strings.Contains(lower, "jira") || strings.Contains(lower, "เคสเก่า") {
			return "jira_search"
		}
		return "abuseipdb"
	case "action":
		if strings.Contains(lower, "บล็อก") || strings.Contains(lower, "ระงับ") || strings.Contains(lower, "block") || strings.Contains(lower, "firewall") {
			return "firewall_block"
		}
		if strings.Contains(lower, "ticket") || strings.Contains(lower, "เคส") || strings.Contains(lower, "tier") {
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
	state.connectNodes(startID, state.buildStepSequence(steps, ""), "")
	return state.nodes, state.edges
}