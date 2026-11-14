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

const sopToTreePrompt = `You are a SOC assistant. Convert the SOP into a nested JSON tree.
Return ONLY JSON: {"steps":[{"type":"lookup|decision|action|end","label":"short title","branches":[{"label":"condition","steps":[...]}]}]}
Rules:
- "branches" is used ONLY by decision. It can have 2, 3 or more branches, and branches may contain nested decisions.
- Indentation shows nesting. "else if" / "else" lines are at the same indent as their "if" and become extra branches of the SAME decision.
- Lines less indented that come after a decision run after the branches merge.
- type "end" = a step that stops that path (e.g. close case). Any branch may stop anywhere.
- lookup = check reputation / search past cases.

SOP:
`

var (
	ifKeywordRegex   = regexp.MustCompile(`(?i)^(if\b|ถ้า|หาก)\s*`)
	elseKeywordRegex = regexp.MustCompile(`(?i)^(else\b|otherwise\b|ไม่เช่นนั้น|มิฉะนั้น|ไม่งั้น)[\s:,\-–]*`)
)

func ConvertLinesToSteps(lines []Line) ([]Step, string) {
	provider := strings.ToLower(Config.LlmProvider)
	var llmErr error
	switch provider {
	case "fallback", "none":
		return ParseLinesByIndentation(lines), "fallback"
	case "hf_api":
		var steps []Step
		if steps, llmErr = queryHuggingFaceChatAPI(lines); llmErr == nil {
			return steps, "hf_api:" + Config.LlmModel
		}
	case "colab_ollama", "ollama":
		var steps []Step
		if steps, llmErr = queryOllamaGenerateAPI(lines); llmErr == nil {
			return steps, provider + ":" + Config.LlmModel
		}
	}
	if llmErr != nil {
		log.Printf("LLM (%s) failed, using fallback parser: %v", provider, llmErr)
		return ParseLinesByIndentation(lines), fmt.Sprintf("fallback (reason: %v)", llmErr)
	}
	return ParseLinesByIndentation(lines), "fallback"
}

func formatIndentedPrompt(lines []Line) string {
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(strings.Repeat("  ", l.Level) + l.Text + "\n")
	}
	return sopToTreePrompt + sb.String()
}

func queryOllamaGenerateAPI(lines []Line) ([]Step, error) {
	payload, _ := json.Marshal(map[string]any{
		"model":  Config.LlmModel,
		"prompt": formatIndentedPrompt(lines),
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

func queryHuggingFaceChatAPI(lines []Line) ([]Step, error) {
	endpoint := Config.LlmEndpoint
	if !strings.Contains(endpoint, "huggingface.co") {
		endpoint = "https://router.huggingface.co/hf-inference/models/" + Config.LlmModel + "/v1/chat/completions"
	}
	payload, _ := json.Marshal(map[string]any{
		"model": Config.LlmModel,
		"messages": []map[string]string{
			{"role": "user", "content": formatIndentedPrompt(lines)},
		},
		"temperature": 0.1,
		"max_tokens":  2048,
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

func inferStepType(text string) string {
	lower := strings.ToLower(text)
	if !strings.Contains(lower, "เปิดเคส") {
		for _, kw := range []string{"ปิดเคส", "close case"} {
			if strings.Contains(lower, kw) {
				return "end"
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

func createSimpleStep(text string) Step {
	return Step{Type: inferStepType(text), Label: text}
}

func collectIndentedChildren(lines []Line, idx *int, parentLevel int) []Line {
	start := *idx
	for *idx < len(lines) && lines[*idx].Level > parentLevel {
		*idx++
	}
	return lines[start:*idx]
}

func ParseLinesByIndentation(lines []Line) []Step {
	steps := []Step{}
	for i := 0; i < len(lines); {
		line := lines[i]
		i++
		if ifKeywordRegex.MatchString(line.Text) {
			steps = append(steps, parseIfBlock(lines, &i, line.Text, line.Level))
			continue
		}
		steps = append(steps, createSimpleStep(line.Text))
		steps = append(steps, ParseLinesByIndentation(collectIndentedChildren(lines, &i, line.Level))...)
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

func parseBranchBlock(lines []Line, idx *int, level int, rawText string) Branch {
	cond, inlineAct := splitConditionAndAction(rawText)
	steps := []Step{}
	if inlineAct != "" {
		steps = append(steps, createSimpleStep(inlineAct))
	}
	steps = append(steps, ParseLinesByIndentation(collectIndentedChildren(lines, idx, level))...)
	return Branch{Label: cond, Steps: steps}
}

func parseIfBlock(lines []Line, idx *int, text string, level int) Step {
	branches := []Branch{parseBranchBlock(lines, idx, level, ifKeywordRegex.ReplaceAllString(text, ""))}
	hasElse := false
	for *idx < len(lines) && lines[*idx].Level == level && elseKeywordRegex.MatchString(lines[*idx].Text) {
		rest := strings.TrimSpace(elseKeywordRegex.ReplaceAllString(lines[*idx].Text, ""))
		*idx++
		if ifKeywordRegex.MatchString(rest) {
			branches = append(branches, parseBranchBlock(lines, idx, level, ifKeywordRegex.ReplaceAllString(rest, "")))
			continue
		}
		elseBranch := Branch{Label: "อื่นๆ", Steps: ParseLinesByIndentation(collectIndentedChildren(lines, idx, level))}
		if rest != "" {
			elseBranch.Steps = append([]Step{createSimpleStep(rest)}, elseBranch.Steps...)
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