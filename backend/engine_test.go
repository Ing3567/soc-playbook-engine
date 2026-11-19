package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenCaseNotConfusedWithCloseCase(t *testing.T) {
	tools := DefaultBuiltinTools()
	if got := inferStepType("เปิดเคสแจ้งเตือนไปยัง Tier 2", tools); got != "action" {
		t.Fatalf("inferStepType(เปิดเคส) = %q, want action", got)
	}
	if got := inferStepType("ปิดเคส", tools); got != "end" {
		t.Fatalf("inferStepType(ปิดเคส) = %q, want end", got)
	}
	if got := MatchToolByKeywords("action", "เปิดเคสแจ้งเตือนไปยัง Tier 2"); got != "jira_ticket" {
		t.Fatalf("MatchToolByKeywords(เปิดเคส) = %q, want jira_ticket", got)
	}
}

func TestSandboxTriggers(t *testing.T) {
	res := RunSandboxStep(ExecutionRequest{Tool: "abuseipdb", Label: "ตรวจสอบ failed login จาก IP"}, nil)
	if res.Status != "success" {
		t.Fatalf("expected normal 'failed login' to succeed, got %q", res.Status)
	}

	failRes := RunSandboxStep(ExecutionRequest{Tool: "abuseipdb", Label: "ตรวจสอบ IP [simulate-fail]"}, nil)
	if failRes.Status != "failed" {
		t.Fatalf("expected [simulate-fail] to fail, got %q", failRes.Status)
	}

	abnormal := RunSandboxStep(ExecutionRequest{Tool: "virustotal", Label: "ตรวจสอบพฤติกรรมผิดปกติ VirusTotal"}, nil)
	if abnormal.Output["verdict"] == "Clean / No Detections" {
		t.Fatalf("expected 'ผิดปกติ' not to be treated as clean, got %v", abnormal.Output["verdict"])
	}
}

func TestSandboxCustomToolMockOutput(t *testing.T) {
	customTool := &ToolDefinition{
		ID:       "crowdstrike_edr",
		Name:     "CrowdStrike Falcon",
		Category: "action",
		MockOutput: map[string]any{
			"isolated": true,
			"host_id":  "host-99",
		},
	}
	res := RunSandboxStep(ExecutionRequest{NodeID: "n2", Type: "action", Label: "แยกเครื่องผู้ใช้ออกจากระบบ", Tool: "crowdstrike_edr"}, customTool)
	if res.Status != "success" {
		t.Fatalf("expected success, got %q", res.Status)
	}
	if res.Output["isolated"] != true || res.Output["tool"] != "CrowdStrike Falcon" {
		t.Fatalf("unexpected custom mock output: %v", res.Output)
	}

	failRes := RunSandboxStep(ExecutionRequest{NodeID: "n2", Type: "action", Label: "แยกเครื่อง [simulate-fail]", Tool: "crowdstrike_edr"}, customTool)
	if failRes.Status != "failed" {
		t.Fatalf("expected [simulate-fail] to override mock output and fail, got %q", failRes.Status)
	}
}

func TestAllPathsEndPrunesOrphanEndNode(t *testing.T) {
	tools := DefaultBuiltinTools()
	steps := ParseLinesByIndentation(ExtractLinesFromText("ถ้า IP อันตราย\n  ปิดเคส\nไม่เช่นนั้น\n  ปิดเคส"), tools)
	nodes, edges := BuildCustomFlowchart(steps, tools)

	xmlBytes, err := BuildBpmnXML("test", nodes, edges)
	if err != nil {
		t.Fatalf("BuildBpmnXML error: %v", err)
	}
	if count := strings.Count(string(xmlBytes), "<bpmn:startEvent"); count != 1 {
		t.Fatalf("expected 1 BPMN startEvent, got %d", count)
	}
}

func TestCacaoExportValidStructureAndCustomTool(t *testing.T) {
	tools := append(DefaultBuiltinTools(), ToolDefinition{
		ID:          "crowdstrike_isolate",
		Name:        "CrowdStrike Isolate",
		Category:    "action",
		Description: "Isolate compromised endpoint",
		Keywords:    []string{"isolate", "แยกเครื่อง"},
		CommandType: "bash",
		Endpoint:    "falconctl isolate --host",
		Enabled:     true,
	})
	toolMap := make(map[string]ToolDefinition, len(tools))
	for _, tl := range tools {
		toolMap[tl.ID] = tl
	}

	steps := ParseLinesByIndentation(ExtractLinesFromText("ตรวจสอบ IP ผ่าน VirusTotal\nถ้าพบอันตราย\n  แยกเครื่องผ่าน CrowdStrike\nไม่เช่นนั้น\n  ปิดเคส"), tools)
	nodes, edges := BuildCustomFlowchart(steps, tools)
	data, err := BuildCacaoJSON("test", nodes, edges, toolMap)
	if err != nil {
		t.Fatalf("BuildCacaoJSON error: %v", err)
	}
	var pb map[string]any
	if err := json.Unmarshal(data, &pb); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	wf := pb["workflow"].(map[string]any)
	startID, _ := pb["workflow_start"].(string)
	if _, ok := wf[startID]; !ok {
		t.Fatalf("workflow_start %q not found in workflow", startID)
	}
	if pb["created_by"] == nil || pb["playbook_types"] == nil {
		t.Fatalf("missing required CACAO 2.0 fields")
	}

	foundBash := false
	for _, rawStep := range wf {
		stepMap := rawStep.(map[string]any)
		if cmds, ok := stepMap["commands"].([]any); ok {
			for _, c := range cmds {
				cmd := c.(map[string]any)
				if cmd["type"] == "bash" && strings.Contains(cmd["command"].(string), "falconctl isolate") {
					foundBash = true
				}
			}
		}
	}
	if !foundBash {
		t.Fatalf("expected custom bash tool command in CACAO workflow")
	}
}

func TestDynamicPromptIncludesEnabledAndOmitsDisabledTools(t *testing.T) {
	tools := []ToolDefinition{
		{
			ID:          "splunk_siem",
			Name:        "Splunk SIEM",
			Category:    "lookup",
			Description: "Query authentication logs in Splunk",
			Keywords:    []string{"splunk", "siem"},
			Enabled:     true,
		},
		{
			ID:          "disabled_tool",
			Name:        "Disabled Tool",
			Category:    "action",
			Description: "Should not appear",
			Enabled:     false,
		},
	}
	prompt := buildSopToTreePrompt([]Line{{Level: 0, Text: "ค้นหา log ใน Splunk"}}, tools)
	if !strings.Contains(prompt, "splunk_siem (Splunk SIEM): Query authentication logs in Splunk") {
		t.Fatalf("prompt missing enabled custom tool: %s", prompt)
	}
	if strings.Contains(prompt, "disabled_tool") {
		t.Fatalf("prompt should omit disabled tool")
	}
}

func TestExtractStepsFromJSONRetainsToolAndResolveStepTool(t *testing.T) {
	raw := `{"steps":[
		{"type":"lookup","label":"Query auth logs","tool":"splunk_siem"},
		{"type":"decision","label":"Suspicious?","tool":"none","branches":[
			{"label":"ใช่","steps":[{"type":"action","label":"Contain host","tool":"CrowdStrike EDR"}]},
			{"label":"ไม่ใช่","steps":[{"type":"action","label":"Unknown step","tool":"hallucinated_tool"}]}
		]}
	]}`
	steps, err := extractStepsFromJSON(raw)
	if err != nil {
		t.Fatalf("extractStepsFromJSON error: %v", err)
	}

	tools := append(DefaultBuiltinTools(),
		ToolDefinition{ID: "splunk_siem", Name: "Splunk SIEM", Category: "lookup", Keywords: []string{"splunk"}, Enabled: true},
		ToolDefinition{ID: "crowdstrike_edr", Name: "CrowdStrike EDR", Category: "action", Keywords: []string{"crowdstrike"}, Enabled: true},
	)

	nodes, _ := BuildCustomFlowchart(steps, tools)
	byLabel := make(map[string]Node, len(nodes))
	for _, n := range nodes {
		byLabel[n.Label] = n
	}

	if byLabel["Query auth logs"].Tool != "splunk_siem" || byLabel["Query auth logs"].ToolSource != "ai" {
		t.Fatalf("unexpected splunk node: %+v", byLabel["Query auth logs"])
	}
	if byLabel["Contain host"].Tool != "crowdstrike_edr" || byLabel["Contain host"].ToolSource != "ai" {
		t.Fatalf("expected display name normalization to crowdstrike_edr, got %+v", byLabel["Contain host"])
	}
	if byLabel["Unknown step"].Tool != "manual_action" || byLabel["Unknown step"].ToolSource != "manual" {
		t.Fatalf("expected hallucinated tool to fall back to manual_action, got %+v", byLabel["Unknown step"])
	}
	if byLabel["Suspicious?"].Tool != "none" {
		t.Fatalf("decision node tool must be none, got %+v", byLabel["Suspicious?"])
	}
}

func TestToolValidationAndSlug(t *testing.T) {
	if got := GenerateToolSlug("CrowdStrike Falcon EDR!"); got != "crowdstrike_falcon_edr" {
		t.Fatalf("GenerateToolSlug = %q", got)
	}
	if _, err := NormalizeAndValidateTool(ToolDefinition{ID: "none", Name: "Bad", Category: "lookup", Description: "desc"}, true); err == nil {
		t.Fatalf("expected reserved ID 'none' to fail validation")
	}
	valid, err := NormalizeAndValidateTool(ToolDefinition{
		Name:        "SIEM Query",
		Category:    "lookup",
		Description: "Query SIEM logs",
		HTTPMethod:  "get",
		PayloadMode: "dynamic",
		Params: []ToolParameter{
			{Key: "ip", In: "path", Mode: "dynamic", Value: "{{alert.src_ip}}", Required: true},
			{Key: "limit", In: "query", Mode: "fixed", Value: "50"},
		},
	}, true)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if valid.HTTPMethod != "GET" || len(valid.Params) != 2 {
		t.Fatalf("unexpected normalized tool: %+v", valid)
	}
}

func TestBuildToolRequestPathQueryFixedAndDynamic(t *testing.T) {
	tool := &ToolDefinition{
		ID:          "threat_lookup",
		Name:        "Threat Lookup",
		Category:    "lookup",
		HTTPMethod:  "GET",
		Endpoint:    "https://intel.example.com/api/v1/ip/{{ip}}?source=soc",
		PayloadMode: "dynamic",
		Params: []ToolParameter{
			{Key: "ip", In: "path", Mode: "dynamic", Value: "{{alert.src_ip}}", Required: true},
			{Key: "days", In: "query", Mode: "fixed", Value: "30"},
		},
	}

	dynRes := BuildToolRequest(tool, ExecutionRequest{
		Tool:        "threat_lookup",
		PayloadMode: "dynamic",
		RuntimeContext: map[string]any{
			"alert": map[string]any{"src_ip": "192.0.2.55"},
		},
	})
	if len(dynRes.MissingRequired) != 0 {
		t.Fatalf("unexpected missing required: %v", dynRes.MissingRequired)
	}
	if !strings.Contains(dynRes.URL, "/api/v1/ip/192.0.2.55?") || !strings.Contains(dynRes.URL, "days=30") || !strings.Contains(dynRes.URL, "source=soc") {
		t.Fatalf("unexpected dynamic resolved URL: %q", dynRes.URL)
	}

	fixedRes := BuildToolRequest(tool, ExecutionRequest{
		Tool:        "threat_lookup",
		PayloadMode: "fixed",
		InputPayload: map[string]string{
			"ip":   "203.0.113.10",
			"days": "7",
		},
	})
	if !strings.Contains(fixedRes.URL, "/api/v1/ip/203.0.113.10?") || !strings.Contains(fixedRes.URL, "days=7") {
		t.Fatalf("unexpected fixed resolved URL: %q", fixedRes.URL)
	}

	missingRes := BuildToolRequest(tool, ExecutionRequest{
		Tool:           "threat_lookup",
		PayloadMode:    "dynamic",
		RuntimeContext: map[string]any{},
	})
	if len(missingRes.MissingRequired) != 1 || missingRes.MissingRequired[0] != "ip" {
		t.Fatalf("expected missing required ['ip'], got %v", missingRes.MissingRequired)
	}
}

func TestLookupNodeInheritsInputPayloadAndSandboxResolvesURL(t *testing.T) {
	tools := DefaultBuiltinTools()
	steps := []Step{{Type: "lookup", Label: "ตรวจสอบ IP ผ่าน VirusTotal", Tool: "virustotal"}}
	nodes, _ := BuildCustomFlowchart(steps, tools)
	var lookupNode *Node
	for i := range nodes {
		if nodes[i].Type == "lookup" {
			lookupNode = &nodes[i]
			break
		}
	}
	if lookupNode == nil {
		t.Fatalf("expected lookup node")
	}
	if lookupNode.PayloadMode != "dynamic" || lookupNode.InputPayload["ip"] != "{{alert.src_ip}}" {
		t.Fatalf("expected lookup node to inherit tool input payload, got mode=%q payload=%v", lookupNode.PayloadMode, lookupNode.InputPayload)
	}

	vt := tools[0]
	res := RunSandboxStep(ExecutionRequest{
		NodeID:       lookupNode.ID,
		Type:         lookupNode.Type,
		Label:        lookupNode.Label,
		Tool:         lookupNode.Tool,
		PayloadMode:  lookupNode.PayloadMode,
		InputPayload: lookupNode.InputPayload,
		RuntimeContext: map[string]any{
			"alert": map[string]any{"src_ip": "198.51.100.42"},
		},
	}, &vt)
	if res.Status != "success" {
		t.Fatalf("expected success, got %q (%v)", res.Status, res.Output)
	}
	if res.Output["resolved_url"] != "https://www.virustotal.com/api/v3/ip_addresses/198.51.100.42" {
		t.Fatalf("unexpected resolved_url: %v", res.Output["resolved_url"])
	}

	failMissing := RunSandboxStep(ExecutionRequest{
		NodeID:       lookupNode.ID,
		Type:         lookupNode.Type,
		Label:        lookupNode.Label,
		Tool:         lookupNode.Tool,
		PayloadMode:  "dynamic",
		InputPayload: lookupNode.InputPayload,
	}, &vt)
	if failMissing.Status != "failed" {
		t.Fatalf("expected missing required dynamic param to fail, got %q", failMissing.Status)
	}
}
