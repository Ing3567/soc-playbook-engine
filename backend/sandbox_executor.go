package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var cleanWordRegex = regexp.MustCompile(`\b(clean|benign)\b`)

func RunSandboxStep(req ExecutionRequest, toolDef *ToolDefinition) ExecutionResult {
	return RunSandboxStepWithScenario(req, toolDef, nil)
}

func extractScenarioLookupTargets(req ExecutionRequest, resolved *ResolvedToolRequest) []string {
	seen := make(map[string]bool)
	var targets []string
	add := func(v string) {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" || trimmed == "***" || seen[trimmed] {
			return
		}
		seen[trimmed] = true
		targets = append(targets, trimmed)
	}

	preferredKeys := []string{"ip", "ipAddress", "src_ip", "domain", "host", "url", "hash", "file_hash", "target"}
	if resolved != nil {
		for _, k := range preferredKeys {
			if v, ok := resolved.Params[k]; ok {
				add(v)
			}
		}
		for _, v := range resolved.Params {
			add(v)
		}
	}
	for _, k := range preferredKeys {
		if v, ok := req.RuntimeInputs[k]; ok {
			add(v)
		}
	}
	if alertMap, ok := req.RuntimeContext["alert"].(map[string]any); ok {
		for _, k := range preferredKeys {
			if raw, exists := alertMap[k]; exists && raw != nil {
				add(fmt.Sprintf("%v", raw))
			}
		}
	}
	return targets
}

func RunSandboxStepWithScenario(req ExecutionRequest, toolDef *ToolDefinition, scenario *Scenario) ExecutionResult {
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
		var resolved *ResolvedToolRequest
		if toolDef != nil && (toolDef.Endpoint != "" || len(toolDef.Params) > 0 || len(req.InputPayload) > 0) {
			r := BuildToolRequest(toolDef, req)
			resolved = &r
			if len(r.MissingRequired) > 0 {
				status = "failed"
				output = map[string]any{
					"tool":           toolDef.Name,
					"tool_id":        toolDef.ID,
					"step":           req.Label,
					"http_method":    r.Method,
					"resolved_url":   r.URL,
					"payload_mode":   r.PayloadMode,
					"missing_params": r.MissingRequired,
					"error":          fmt.Sprintf("Missing required input parameters: %s", strings.Join(r.MissingRequired, ", ")),
				}
			}
		}

		if status != "failed" {
			// 1. Prefer scenario-driven mock output when a training scenario is active.
			if scenario != nil {
				targets := extractScenarioLookupTargets(req, resolved)
				if mockOut, matchedKey, ok := ResolveScenarioMockOutput(scenario, req.Tool, targets); ok {
					output = mockOut
					if _, hasTool := output["tool"]; !hasTool {
						if toolDef != nil {
							output["tool"] = toolDef.Name
						} else {
							output["tool"] = req.Tool
						}
					}
					if toolDef != nil {
						output["tool_id"] = toolDef.ID
					}
					output["step"] = req.Label
					output["scenario_id"] = scenario.ID
					output["scenario_match"] = matchedKey
				}
			}

			// 2. Fallback to existing keyword/tool sandbox mock when no scenario mock matched.
			if output == nil {
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
				case "ticket", "jira_ticket":
					output = map[string]any{
						"tool":      "Ticket",
						"step":      req.Label,
						"ticket_id": fmt.Sprintf("SOC-%d", 2000+time.Now().Unix()%1000),
						"status":    "Created",
					}
				default:
					if toolDef != nil {
						output = make(map[string]any, len(toolDef.MockOutput)+6)
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

			if resolved != nil {
				if resolved.Method != "" {
					output["http_method"] = resolved.Method
				}
				if resolved.URL != "" {
					output["resolved_url"] = resolved.URL
				}
				output["payload_mode"] = resolved.PayloadMode
				if len(resolved.Params) > 0 {
					output["input_payload"] = resolved.Params
				}
				if len(resolved.Body) > 0 {
					output["body_payload"] = resolved.Body
				}
				if resolved.Auth != nil && resolved.Auth.Type != "none" {
					switch resolved.Auth.Source {
					case "env":
						output["auth_status"] = fmt.Sprintf("configured (env: %s)", resolved.Auth.EnvVar)
					case "runtime":
						output["auth_status"] = "configured (session key)"
					default:
						output["auth_status"] = "not configured (sandbox mode)"
					}
					if resolved.Auth.MaskedPreview != "" {
						output["auth_preview"] = resolved.Auth.MaskedPreview
					}
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
