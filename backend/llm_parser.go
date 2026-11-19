package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const sopToTreePromptHeader = `You are a SOC assistant. Convert the SOP into a nested JSON tree and map each lookup/action step to the best matching tool.
Return ONLY JSON: {"steps":[{"type":"lookup|decision|action|end","label":"short title","tool":"tool_id_or_none","branches":[{"label":"condition","steps":[...]}]}]}
Rules:
- "branches" is used ONLY by decision. It can have 2, 3 or more branches, and branches may contain nested decisions.
- Indentation shows nesting. "else if" / "else" lines are at the same indent as their "if" and become extra branches of the SAME decision.
- Lines less indented that come after a decision run after the branches merge.
- type "end" = a step that stops that path (e.g. close case). Any branch may stop anywhere.
- lookup = check reputation, query logs, or search past cases.
- Tool mapping:
  - For "lookup" and "action" steps, set "tool" to the single best matching tool ID from Available Tools below based on meaning and intent.
  - Use "manual_lookup" (for lookup) or "manual_action" (for action) only if no registered tool fits.
  - For "decision" and "end" steps, ALWAYS set "tool" to "none".
  - NEVER invent tool IDs outside Available Tools.

Available Tools:
`

var (
	ifKeywordRegex   = regexp.MustCompile(`(?i)^(if\b|ถ้า|หาก)\s*`)
	elseKeywordRegex = regexp.MustCompile(`(?i)^(else\b|otherwise\b|ไม่เช่นนั้น|มิฉะนั้น|ไม่งั้น)[\s:,\-–]*`)
)

func ConvertLinesToSteps(lines []Line, tools []ToolDefinition) ([]Step, string) {
	provider := strings.ToLower(Config.LlmProvider)
	var llmErr error
	switch provider {
	case "fallback", "none":
		return ParseLinesByIndentation(lines, tools), "fallback"
	case "hf_api":
		var steps []Step
		if steps, llmErr = queryHuggingFaceChatAPI(lines, tools); llmErr == nil {
			return steps, "hf_api:" + Config.LlmModel
		}
	case "colab_ollama", "ollama":
		var steps []Step
		if steps, llmErr = queryOllamaGenerateAPI(lines, tools); llmErr == nil {
			return steps, provider + ":" + Config.LlmModel
		}
	}
	if llmErr != nil {
		log.Printf("LLM (%s) failed, using fallback parser: %v", provider, llmErr)
		return ParseLinesByIndentation(lines, tools), fmt.Sprintf("fallback (reason: %v)", llmErr)
	}
	return ParseLinesByIndentation(lines, tools), "fallback"
}

func formatToolCatalogForPrompt(tools []ToolDefinition) string {
	var lookupLines []string
	var actionLines []string
	for _, t := range tools {
		if !t.Enabled {
			continue
		}
		kw := ""
		if len(t.Keywords) > 0 {
			kw = fmt.Sprintf(" (keywords: %s)", strings.Join(t.Keywords, ", "))
		}
		entry := fmt.Sprintf("- %s (%s): %s%s", t.ID, t.Name, t.Description, kw)
		if t.Category == "lookup" {
			lookupLines = append(lookupLines, entry)
		} else {
			actionLines = append(actionLines, entry)
		}
	}
	var sb strings.Builder
	sb.WriteString("[lookup]\n")
	if len(lookupLines) > 0 {
		sb.WriteString(strings.Join(lookupLines, "\n") + "\n")
	} else {
		sb.WriteString("- (none)\n")
	}
	sb.WriteString("[action]\n")
	if len(actionLines) > 0 {
		sb.WriteString(strings.Join(actionLines, "\n") + "\n")
	} else {
		sb.WriteString("- (none)\n")
	}
	return sb.String()
}

func buildSopToTreePrompt(lines []Line, tools []ToolDefinition) string {
	var sb strings.Builder
	sb.WriteString(sopToTreePromptHeader)
	sb.WriteString(formatToolCatalogForPrompt(tools))
	sb.WriteString("\nSOP:\n")
	for _, l := range lines {
		sb.WriteString(strings.Repeat("  ", l.Level) + l.Text + "\n")
	}
	return sb.String()
}

func queryOllamaGenerateAPI(lines []Line, tools []ToolDefinition) ([]Step, error) {
	payload, _ := json.Marshal(map[string]any{
		"model":  Config.LlmModel,
		"prompt": buildSopToTreePrompt(lines, tools),
		"stream": false,
		"format": "json",
	})
	url := strings.TrimRight(Config.LlmEndpoint, "/") + "/api/generate"
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama HTTP %d", resp.StatusCode)
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return extractStepsFromJSON(result.Response)
}

func queryHuggingFaceChatAPI(lines []Line, tools []ToolDefinition) ([]Step, error) {
	endpoint := Config.LlmEndpoint
	if !strings.Contains(endpoint, "huggingface.co") {
		endpoint = "https://router.huggingface.co/hf-inference/models/" + Config.LlmModel + "/v1/chat/completions"
	}
	payload, _ := json.Marshal(map[string]any{
		"model": Config.LlmModel,
		"messages": []map[string]string{
			{"role": "user", "content": buildSopToTreePrompt(lines, tools)},
		},
		"temperature": 0.1,
		"max_tokens":  3072,
	})
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if Config.HfToken != "" {
		req.Header.Set("Authorization", "Bearer "+Config.HfToken)
	}
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("hf_api HTTP %d", resp.StatusCode)
	}

	var chat struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&chat); err != nil || len(chat.Choices) == 0 {
		return nil, errors.New("empty response")
	}
	return extractStepsFromJSON(chat.Choices[0].Message.Content)
}

func extractStepsFromJSON(raw string) ([]Step, error) {
	cleaned := strings.TrimSpace(raw)
	start := strings.Index(cleaned, "{")
	end := strings.LastIndex(cleaned, "}")
	if start >= 0 && end > start {
		cleaned = cleaned[start : end+1]
	}
	var parsed struct {
		Steps []Step `json:"steps"`
	}
	if json.Unmarshal([]byte(cleaned), &parsed) != nil || len(parsed.Steps) == 0 {
		return nil, errors.New("invalid steps")
	}
	return parsed.Steps, nil
}

func inferStepType(text string, tools []ToolDefinition) string {
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "เปิดเคส") {
		for _, kw := range []string{"ปิดเคส", "close case"} {
			if strings.Contains(lower, kw) {
				return "end"
			}
		}
	}
	for _, t := range tools {
		if !t.Enabled || t.Category != "lookup" {
			continue
		}
		for _, kw := range t.Keywords {
			if kw != "" && matchesToolKeyword(lower, kw) {
				return "lookup"
			}
		}
	}
	for _, kw := range []string{"virustotal", "reputation", "jira", "เคสเก่า", "ค้นหา", "ตรวจสอบ"} {
		if strings.Contains(lower, kw) {
			return "lookup"
		}
	}
	return "action"
}

func createSimpleStep(text string, tools []ToolDefinition) Step {
	return Step{Type: inferStepType(text, tools), Label: text}
}

func collectIndentedChildren(lines []Line, idx *int, parentLevel int) []Line {
	start := *idx
	for *idx < len(lines) && lines[*idx].Level > parentLevel {
		*idx++
	}
	return lines[start:*idx]
}

func ParseLinesByIndentation(lines []Line, tools []ToolDefinition) []Step {
	steps := []Step{}
	for i := 0; i < len(lines); {
		line := lines[i]
		i++
		if ifKeywordRegex.MatchString(line.Text) {
			steps = append(steps, parseIfBlock(lines, &i, line.Text, line.Level, tools))
			continue
		}
		steps = append(steps, createSimpleStep(line.Text, tools))
		steps = append(steps, ParseLinesByIndentation(collectIndentedChildren(lines, &i, line.Level), tools)...)
	}
	return steps
}

func splitConditionAndAction(text string) (string, string) {
	text = strings.TrimSpace(text)
	if pos := strings.Index(text, "ให้"); pos > 0 {
		return strings.TrimSpace(text[:pos]), strings.TrimSpace(text[pos+len("ให้"):])
	}
	if pos := strings.Index(text, " then "); pos > 0 {
		return strings.TrimSpace(text[:pos]), strings.TrimSpace(text[pos+6:])
	}
	return text, ""
}

func parseBranchBlock(lines []Line, idx *int, level int, rawText string, tools []ToolDefinition) Branch {
	cond, inlineAct := splitConditionAndAction(rawText)
	steps := []Step{}
	if inlineAct != "" {
		steps = append(steps, createSimpleStep(inlineAct, tools))
	}
	steps = append(steps, ParseLinesByIndentation(collectIndentedChildren(lines, idx, level), tools)...)
	return Branch{Label: cond, Steps: steps}
}

func parseIfBlock(lines []Line, idx *int, text string, level int, tools []ToolDefinition) Step {
	branches := []Branch{parseBranchBlock(lines, idx, level, ifKeywordRegex.ReplaceAllString(text, ""), tools)}
	hasElse := false
	for *idx < len(lines) && lines[*idx].Level == level && elseKeywordRegex.MatchString(lines[*idx].Text) {
		rest := strings.TrimSpace(elseKeywordRegex.ReplaceAllString(lines[*idx].Text, ""))
		*idx++
		if ifKeywordRegex.MatchString(rest) {
			branches = append(branches, parseBranchBlock(lines, idx, level, ifKeywordRegex.ReplaceAllString(rest, ""), tools))
			continue
		}
		elseBranch := Branch{Label: "อื่นๆ", Steps: ParseLinesByIndentation(collectIndentedChildren(lines, idx, level), tools)}
		if rest != "" {
			elseBranch.Steps = append([]Step{createSimpleStep(rest, tools)}, elseBranch.Steps...)
		}
		branches = append(branches, elseBranch)
		hasElse = true
		break
	}
	return normalizeDecisionStep(branches, hasElse)
}

func normalizeDecisionStep(branches []Branch, hasElse bool) Step {
	if len(branches) == 1 || (len(branches) == 2 && hasElse) {
		decision := Step{Type: "decision", Label: branches[0].Label + "?"}
		branches[0].Label = "ใช่"
		if len(branches) == 1 {
			branches = append(branches, Branch{Label: "ไม่ใช่"})
		} else {
			branches[1].Label = "ไม่ใช่"
		}
		decision.Branches = branches
		return decision
	}
	if !hasElse {
		branches = append(branches, Branch{Label: "อื่นๆ"})
	}
	return Step{Type: "decision", Label: "ตรวจเงื่อนไข", Branches: branches}
}
