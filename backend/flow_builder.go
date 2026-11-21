package main

import (
	"fmt"
	"strings"
)

type graphState struct {
	nodes    []Node
	edges    []Edge
	registry *URLRegistry
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

func (g *graphState) activeRegistry() *URLRegistry {
	if g.registry != nil {
		return g.registry
	}
	return NewURLRegistry()
}

func (g *graphState) appendNode(nodeType, label string) string {
	return g.appendStepNode(Step{Type: nodeType, Label: label})
}

func (g *graphState) appendStepNode(step Step) string {
	reg := g.activeRegistry()
	links := reg.ResolveNodeLinks(step.Links, step.Label)
	images := reg.ResolveNodeImages(step.Images, step.Label)
	cleanLabel := reg.CleanNodeLabel(step.Label, links, images)

	toolHint := cleanLabel
	for _, l := range links {
		toolHint += " " + l.URL + " " + l.Label
	}

	id := fmt.Sprintf("n%d", len(g.nodes)+1)
	g.nodes = append(g.nodes, Node{
		ID:     id,
		Type:   step.Type,
		Label:  cleanLabel,
		Tool:   MatchToolByKeywords(step.Type, toolHint),
		Links:  links,
		Images: images,
	})
	return id
}

func (g *graphState) connectNodes(fromID, toID, label string) {
	if toID == "" {
		return
	}
	cleanEdgeLabel := g.activeRegistry().StripPlaceholders(label)
	for i := range g.edges {
		existing := &g.edges[i]
		if existing.Source == fromID && existing.Target == toID {
			if cleanEdgeLabel != "" {
				if existing.Label == "" {
					existing.Label = cleanEdgeLabel
				} else {
					existing.Label += " / " + cleanEdgeLabel
				}
			}
			return
		}
	}
	edgeID := fmt.Sprintf("e%d", len(g.edges)+1)
	g.edges = append(g.edges, Edge{ID: edgeID, Source: fromID, Target: toID, Label: cleanEdgeLabel})
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
		if step.Type == "decision" {
			for bIdx := range step.Branches {
				br := &step.Branches[bIdx]
				urlMatches := placeholderPattern.FindAllString(br.Label, -1)
				imgMatches := imageTokenPattern.FindAllString(br.Label, -1)
				if len(urlMatches) > 0 || len(imgMatches) > 0 {
					if len(br.Steps) > 0 {
						br.Steps[0].Links = append(br.Steps[0].Links, urlMatches...)
						br.Steps[0].Images = append(br.Steps[0].Images, imgMatches...)
					} else {
						step.Links = append(step.Links, urlMatches...)
						step.Images = append(step.Images, imgMatches...)
					}
				}
			}
		}
		id := g.appendStepNode(step)
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
	return BuildCustomFlowchartWithRegistry(steps, nil)
}

func BuildCustomFlowchartWithRegistry(steps []Step, registry *URLRegistry) ([]Node, []Edge) {
	if registry != nil {
		steps = registry.ReconcileSteps(steps)
	}
	state := &graphState{registry: registry}
	startID := state.appendNode("terminal", "เริ่ม")
	state.connectNodes(startID, state.buildStepSequence(steps, ""), "")
	return state.nodes, state.edges
}