package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var cleanWordRegex = regexp.MustCompile(`\b(clean|benign)\b`)

func RunSandboxStep(req ExecutionRequest, toolDef *ToolDefinition) ExecutionResult {
	start := time.Now()
	time.Sleep(250 * time.Millisecond)

	lower := strings.ToLower(req.Label)
	status := "success"
	var output map[string]any

	if strings.Contains(lower, "[simulate-fail]") {
		status = "failed"
		output = map[string]any{
			"tool":  req.Tool,
			"step":  req.Label,
			"error": "Simulated execution failure",
		}
	} else {
		isAbnormal := strings.Contains(lower, "ผิดปกติ")
		isClean := !isAbnormal && (cleanWordRegex.MatchString(lower) ||
			strings.Contains(lower, "ปกติ") || strings.Contains(lower, "ปลอดภัย"))

		switch req.Tool {
		case "virustotal":
			if isClean {
				output = map[string]any{
					"tool":            "VirusTotal API Sandbox",
					"step":            req.Label,
					"malicious_count": 0,
					"verdict":         "Clean / No Detections",
				}
			} else {
				output = map[string]any{
					"tool":            "VirusTotal API Sandbox",
					"step":            req.Label,
					"malicious_count": 12,
					"verdict":         "Malicious IP Confirmed",
				}
			}
		case "abuseipdb":
			if isClean {
				output = map[string]any{
					"tool":             "AbuseIPDB Sandbox",
					"step":             req.Label,
					"confidence_score": 5,
					"verdict":          "Low Risk",
				}
			} else {
				output = map[string]any{
					"tool":             "AbuseIPDB Sandbox",
					"step":             req.Label,
					"confidence_score": 85,
					"verdict":          "Suspicious Source",
				}
			}
		case "jira_search":
			if isClean {
				output = map[string]any{
					"tool":          "Jira Case Lookup",
					"step":          req.Label,
					"matched_cases": []string{},
				}
			} else {
				output = map[string]any{
					"tool":          "Jira Case Lookup",
					"step":          req.Label,
					"matched_cases": []string{"SOC-1024", "SOC-1089"},
				}
			}
		case "firewall_block":
			output = map[string]any{
				"tool":    "Firewall Controller",
				"step":    req.Label,
				"rule_id": fmt.Sprintf("FW-%d", time.Now().Unix()%10000),
				"result":  "Blocked Successfully",
			}
		case "jira_ticket":
			output = map[string]any{
				"tool":      "Jira Ticketing System",
				"step":      req.Label,
				"ticket_id": fmt.Sprintf("SOC-%d", 2000+time.Now().Unix()%1000),
				"status":    "Created",
			}
		default:
			if toolDef != nil {
				output = make(map[string]any, len(toolDef.MockOutput)+4)
				for k, v := range toolDef.MockOutput {
					output[k] = v
				}
				if _, ok := output["tool"]; !ok {
					output["tool"] = toolDef.Name
				}
				output["tool_id"] = toolDef.ID
				output["step"] = req.Label
				if len(toolDef.MockOutput) == 0 {
					output["category"] = toolDef.Category
					if toolDef.Endpoint != "" {
						output["endpoint"] = toolDef.Endpoint
					}
					output["result"] = "Simulated Execution Completed"
				}
			} else {
				output = map[string]any{
					"tool":   "Manual SOC Procedure",
					"step":   req.Label,
					"result": "Completed",
				}
			}
		}
	}

	return ExecutionResult{
		NodeID:     req.NodeID,
		Status:     status,
		DurationMs: time.Since(start).Milliseconds(),
		Timestamp:  time.Now().Format("15:04:05"),
		Output:     output,
	}
}
