package main

import (
	"fmt"
	"strings"
)

func BuildMarkdownMermaid(title string, nodes []Node, edges []Edge) []byte {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n```mermaid\ngraph TD\n", title))
	for _, n := range nodes {
		open, close := "(", ")"
		if n.Type == "decision" {
			open, close = "{", "}"
		} else if n.Type == "terminal" || n.Type == "end" {
			open, close = "([", "])"
		}
		cleanLabel := strings.ReplaceAll(n.Label, `"`, `'`)
		sb.WriteString(fmt.Sprintf("  %s%s\"%s\"%s\n", n.ID, open, cleanLabel, close))
	}
	for _, e := range edges {
		if e.Label != "" {
			cleanEdge := strings.ReplaceAll(e.Label, `"`, `'`)
			sb.WriteString(fmt.Sprintf("  %s -->|\"%s\"| %s\n", e.Source, cleanEdge, e.Target))
		} else {
			sb.WriteString(fmt.Sprintf("  %s --> %s\n", e.Source, e.Target))
		}
	}
	sb.WriteString("```\n")

	hasAssets := false
	for _, n := range nodes {
		if len(n.Links) > 0 || len(n.Images) > 0 {
			hasAssets = true
			break
		}
	}
	if hasAssets {
		sb.WriteString("\n## รายละเอียดลิงก์และรูปภาพประกอบ (Node References & Attachments)\n\n")
		for _, n := range nodes {
			if len(n.Links) == 0 && len(n.Images) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("### %s: %s\n", n.ID, n.Label))
			for _, l := range n.Links {
				if l.SplunkQuery != "" {
					sb.WriteString(fmt.Sprintf("- 🔗 [%s](%s) — Splunk Query: `%s`\n", l.Label, l.URL, l.SplunkQuery))
				} else {
					sb.WriteString(fmt.Sprintf("- 🔗 [%s](%s)\n", l.Label, l.URL))
				}
			}
			for _, img := range n.Images {
				caption := img.Caption
				if caption == "" {
					caption = img.Filename
				}
				sb.WriteString(fmt.Sprintf("- 🖼️ ![%s](%s)\n", caption, img.URL))
			}
			sb.WriteString("\n")
		}
	}

	return []byte(sb.String())
}