package main

import (
	"fmt"
	"time"
)

func RunSandboxStep(req ExecutionRequest) ExecutionResult {
	start := time.Now()
	time.Sleep(250 * time.Millisecond)

	var output map[string]any
	switch req.Tool {
	case "virustotal":
		output = map[string]any{
			"tool":            "VirusTotal API Sandbox",
			"step":            req.Label,
			"malicious_count": 12,
			"verdict":         "Malicious IP Confirmed",
		}
	case "abuseipdb":
		output = map[string]any{
			"tool":             "AbuseIPDB Sandbox",
			"step":             req.Label,
			"confidence_score": 85,
			"verdict":          "Suspicious Source",
		}
	case "jira_search":
		output = map[string]any{
			"tool":          "Jira Case Lookup",
			"step":          req.Label,
			"matched_cases": []string{"SOC-1024", "SOC-1089"},
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
		output = map[string]any{
			"tool":   "Manual SOC Procedure",
			"step":   req.Label,
			"result": "Completed",
		}
	}

	return ExecutionResult{
		NodeID:     req.NodeID,
		Status:     "success",
		DurationMs: time.Since(start).Milliseconds(),
		Timestamp:  time.Now().Format("15:04:05"),
		Output:     output,
	}
}