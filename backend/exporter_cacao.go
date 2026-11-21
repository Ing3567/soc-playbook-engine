package main

import (
	"encoding/json"
	"fmt"
	"time"
)

type CacaoPlaybook struct {
	Type          string                   `json:"type"`
	SpecVersion   string                   `json:"spec_version"`
	ID            string                   `json:"id"`
	Name          string                   `json:"name"`
	Created       string                   `json:"created"`
	Modified      string                   `json:"modified"`
	WorkflowStart string                   `json:"workflow_start"`
	Workflow      map[string]CacaoStepItem `json:"workflow"`
}

type CacaoExternalReference struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type CacaoStepItem struct {
	Type               string                   `json:"type"`
	Name               string                   `json:"name"`
	OnCompletion       string                   `json:"on_completion,omitempty"`
	Condition          string                   `json:"condition,omitempty"`
	Cases              map[string]string        `json:"cases,omitempty"`
	Commands           []CacaoCommand           `json:"commands,omitempty"`
	ExternalReferences []CacaoExternalReference `json:"external_references,omitempty"`
}

type CacaoCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func BuildCacaoJSON(title string, nodes []Node, edges []Edge) ([]byte, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	workflow := make(map[string]CacaoStepItem)
	outgoing := make(map[string][]Edge)

	for _, e := range edges {
		outgoing[e.Source] = append(outgoing[e.Source], e)
	}

	startID := ""
	for i, n := range nodes {
		stepID := "step--" + n.ID
		if i == 0 {
			startID = stepID
		}
		outs := outgoing[n.ID]
		var item CacaoStepItem

		if n.Type == "terminal" && i == 0 {
			item = CacaoStepItem{Type: "start", Name: n.Label}
			if len(outs) > 0 {
				item.OnCompletion = "step--" + outs[0].Target
			}
		} else if n.Type == "terminal" || n.Type == "end" {
			item = CacaoStepItem{Type: "end", Name: n.Label}
		} else if n.Type == "decision" {
			cases := make(map[string]string)
			for idx, br := range outs {
				key := br.Label
				if key == "" {
					key = fmt.Sprintf("branch_%d", idx+1)
				}
				cases[key] = "step--" + br.Target
			}
			item = CacaoStepItem{Type: "switch-condition", Name: n.Label, Condition: n.Label, Cases: cases}
		} else {
			item = CacaoStepItem{
				Type:     "action",
				Name:     n.Label,
				Commands: []CacaoCommand{{Type: n.Tool, Command: n.Label}},
			}
			if len(outs) > 0 {
				item.OnCompletion = "step--" + outs[0].Target
			}
		}

		for _, l := range n.Links {
			item.ExternalReferences = append(item.ExternalReferences, CacaoExternalReference{
				Name:        l.Label,
				URL:         l.URL,
				Description: l.SplunkQuery,
			})
		}
		for _, img := range n.Images {
			name := img.Caption
			if name == "" {
				name = img.Filename
			}
			item.ExternalReferences = append(item.ExternalReferences, CacaoExternalReference{
				Name:        name,
				URL:         img.URL,
				Description: img.ContentType,
			})
		}

		workflow[stepID] = item
	}

	pb := CacaoPlaybook{
		Type:          "playbook",
		SpecVersion:   "cacao-2.0",
		ID:            fmt.Sprintf("playbook--%d", time.Now().UnixNano()),
		Name:          title,
		Created:       now,
		Modified:      now,
		WorkflowStart: startID,
		Workflow:      workflow,
	}
	return json.MarshalIndent(pb, "", "  ")
}