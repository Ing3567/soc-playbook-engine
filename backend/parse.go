package main

import (
    "bytes"
    "encoding/json"
    "errors"
    "net/http"
    "os"
    "regexp"
    "strings"
    "time"
)


type Branch struct {
    Label string `json:"label"`
    Steps []Step `json:"steps"`
}


type Step struct {
    Type     string   `json:"type"` 
    Label    string   `json:"label"`
    Branches []Branch `json:"branches,omitempty"`
}

const prompt = `You are a SOC assistant. Convert the SOP into a nested JSON tree.
Return ONLY JSON: {"steps":[{"type":"lookup|decision|action|end","label":"short title","branches":[{"label":"condition","steps":[...]}]}]}
Rules:
- "branches" is used ONLY by decision. It can have 2, 3 or more branches, and branches may contain nested decisions.
- Indentation shows nesting. "else if" / "else" lines are at the same indent as their "if" and become extra branches of the SAME decision.
- Lines less indented that come after a decision run after the branches merge.
- type "end" = a step that stops that path (e.g. close case). Any branch may stop anywhere.
- lookup = check reputation / search past cases.

Example SOP:
Check IP on VirusTotal
If malicious > 10
  Escalate to Tier 2
Else if malicious 1-10
  If known false positive
    Close case
Else
  Monitor 24h
Write ticket

Example JSON:
{"steps":[{"type":"lookup","label":"Check IP on VirusTotal"},{"type":"decision","label":"Check severity","branches":[{"label":"malicious > 10","steps":[{"type":"action","label":"Escalate to Tier 2"}]},{"label":"malicious 1-10","steps":[{"type":"decision","label":"known false positive?","branches":[{"label":"ใช่","steps":[{"type":"end","label":"Close case"}]},{"label":"ไม่ใช่","steps":[]}]}]},{"label":"else","steps":[{"type":"action","label":"Monitor 24h"}]}]},{"type":"action","label":"Write ticket"}]}

SOP:
`


func textLines(s string) []Line {
    var out []Line
    for _, raw := range strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") {
        w := 0
        for _, ch := range raw {
            if ch == ' ' {
                w++
            } else if ch == '\t' {
                w += 4
            } else {
                break
            }
        }
        text := strings.TrimSpace(bullet.ReplaceAllString(strings.TrimSpace(raw), ""))
        if text != "" {
            out = append(out, Line{Level: w / 2, Text: text})
        }
    }
    return out
}

func toSteps(lines []Line) ([]Step, string) {
    if os.Getenv("NO_LLM") == "" { 
        if steps, err := askOllama(lines); err == nil {
            return steps, "ollama"
        }
    }
    return parse(lines), "fallback"
}

func askOllama(lines []Line) ([]Step, error) {
    var sb strings.Builder
    for _, l := range lines {
        sb.WriteString(strings.Repeat("  ", l.Level) + l.Text + "\n")
    }
    model := os.Getenv("OLLAMA_MODEL")
    if model == "" {
        model = "qwen2.5:3b"
    }
    body, _ := json.Marshal(map[string]any{
        "model": model, "prompt": prompt + sb.String(), "stream": false, "format": "json",
    })
    resp, err := (&http.Client{Timeout: 60 * time.Second}).Post(
        "http://localhost:11434/api/generate", "application/json", bytes.NewReader(body))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var out struct{ Response string `json:"response"` }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return nil, err
    }
    var parsed struct{ Steps []Step `json:"steps"` }
    if json.Unmarshal([]byte(out.Response), &parsed) != nil || len(parsed.Steps) == 0 {
        return nil, errors.New("invalid llm output")
    }
    return parsed.Steps, nil
}


var (
    ifRe   = regexp.MustCompile(`(?i)^(if\b|ถ้า|หาก)\s*`)
    elseRe = regexp.MustCompile(`(?i)^(else\b|otherwise\b|ไม่เช่นนั้น|มิฉะนั้น|ไม่งั้น)[\s:,\-–]*`)
)

func classify(s string) string {
    s = strings.ToLower(s)
    for _, k := range []string{"ปิดเคส", "close case"} {
        if strings.Contains(s, k) {
            return "end"
        }
    }
    for _, k := range []string{"virustotal", "reputation", "jira", "เคสเก่า", "ค้นหา", "ตรวจสอบ"} {
        if strings.Contains(s, k) {
            return "lookup"
        }
    }
    return "action"
}

func plain(text string) Step { return Step{Type: classify(text), Label: text} }

// ดึงบรรทัดที่ลึกกว่า level (= เนื้อหาใน branch)
func take(ls []Line, i *int, level int) []Line {
    start := *i
    for *i < len(ls) && ls[*i].Level > level {
        *i++
    }
    return ls[start:*i]
}

func parse(ls []Line) []Step {
    steps := []Step{}
    for i := 0; i < len(ls); {
        l := ls[i]
        i++
        if ifRe.MatchString(l.Text) {
            steps = append(steps, parseIf(ls, &i, l.Text, l.Level))
            continue
        }
        steps = append(steps, plain(l.Text))
        steps = append(steps, parse(take(ls, &i, l.Level))...) // ลูกของ step ธรรมดา = ทำต่อเนื่อง
    }
    return steps
}


func splitCond(s string) (string, string) {
    s = strings.TrimSpace(s)
    if p := strings.Index(s, "ให้"); p > 0 {
        return strings.TrimSpace(s[:p]), strings.TrimSpace(s[p+len("ให้"):])
    }
    if p := strings.Index(s, " then "); p > 0 {
        return strings.TrimSpace(s[:p]), strings.TrimSpace(s[p+6:])
    }
    return s, ""
}


func readBranch(ls []Line, i *int, level int, text string) Branch {
    cond, then := splitCond(text)
    steps := []Step{}
    if then != "" {
        steps = append(steps, plain(then))
    }
    steps = append(steps, parse(take(ls, i, level))...)
    return Branch{Label: cond, Steps: steps}
}


func parseIf(ls []Line, i *int, text string, level int) Step {
    branches := []Branch{readBranch(ls, i, level, ifRe.ReplaceAllString(text, ""))}
    hasElse := false

    for *i < len(ls) && ls[*i].Level == level && elseRe.MatchString(ls[*i].Text) {
        rest := strings.TrimSpace(elseRe.ReplaceAllString(ls[*i].Text, ""))
        *i++
        if ifRe.MatchString(rest) { 
            branches = append(branches, readBranch(ls, i, level, ifRe.ReplaceAllString(rest, "")))
            continue
        }
        b := Branch{Label: "อื่นๆ", Steps: parse(take(ls, i, level))}
        if rest != "" {
            b.Steps = append([]Step{plain(rest)}, b.Steps...)
        }
        branches = append(branches, b)
        hasElse = true
        break
    }
    return finishIf(branches, hasElse)
}

func finishIf(bs []Branch, hasElse bool) Step {
	
	if len(bs) == 1 || (len(bs) == 2 && hasElse) {
		d := Step{Type: "decision", Label: bs[0].Label + "?"}
		
		bs[0].Label = "ใช่" 
		
		if len(bs) == 1 {
			bs = append(bs, Branch{Label: "ไม่ใช่"}) 
		} else {
			bs[1].Label = "ไม่ใช่" 
		}
		d.Branches = bs
		return d
	}
	
	
	if !hasElse {
		bs = append(bs, Branch{Label: "อื่นๆ"}) 
	}
	return Step{Type: "decision", Label: "ตรวจเงื่อนไข", Branches: bs}
}