package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	if got := MatchToolByKeywords("action", "เปิดเคสแจ้งเตือนไปยัง Tier 2"); got != "ticket" {
		t.Fatalf("MatchToolByKeywords(เปิดเคส) = %q, want ticket", got)
	}
	if got := NormalizeLegacyToolID("jira_ticket"); got != "ticket" {
		t.Fatalf("NormalizeLegacyToolID(jira_ticket) = %q, want ticket", got)
	}
	res := RunSandboxStep(ExecutionRequest{Tool: "ticket", Label: "เปิดเคสแจ้งเตือนไปยัง Tier 2"}, nil)
	if res.Output["tool"] != "Ticket" {
		t.Fatalf("RunSandboxStep(ticket) tool = %v, want Ticket", res.Output["tool"])
	}
	for _, td := range tools {
		if td.ID == "ticket" && td.Name != "Ticket" {
			t.Fatalf("builtin ticket tool Name = %q, want Ticket", td.Name)
		}
		if strings.EqualFold(td.Name, "Jira Ticket") {
			t.Fatalf("unexpected builtin tool named %q", td.Name)
		}
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

func TestToolAutoDescriptionAndDryRun(t *testing.T) {
	norm, err := NormalizeAndValidateTool(ToolDefinition{
		Name:       "Shodan Host",
		Category:   "lookup",
		HTTPMethod: "GET",
		Endpoint:   "https://api.shodan.io/shodan/host/{{ip}}",
		Params: []ToolParameter{
			{Key: "ip", In: "path", Mode: "dynamic", Value: "{{alert.src_ip}}", Required: true},
			{Key: "minify", In: "query", Mode: "fixed", Value: "true"},
		},
		MockOutput: map[string]any{"ports": []int{22, 80}},
	}, true)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if norm.Description == "" {
		t.Fatalf("expected auto-generated description when omitted")
	}

	execReq := ExecutionRequest{
		NodeID:         "dry_run",
		Type:           norm.Category,
		Label:          "Dry Run: " + norm.Name,
		Tool:           norm.ID,
		RuntimeContext: map[string]any{"alert": map[string]any{"src_ip": "1.2.3.4"}},
	}
	res := RunSandboxStep(execReq, &norm)
	if res.Status != "success" {
		t.Fatalf("expected dry run success, got %q (%v)", res.Status, res.Output)
	}
	if res.Output["resolved_url"] != "https://api.shodan.io/shodan/host/1.2.3.4?minify=true" {
		t.Fatalf("unexpected dry run resolved_url: %v", res.Output["resolved_url"])
	}
}

func TestBuildToolRequestPerParamModesAndRuntimeInputs(t *testing.T) {
	tool := ToolDefinition{
		ID:          "abuseipdb",
		Name:        "AbuseIPDB",
		Category:    "lookup",
		HTTPMethod:  "GET",
		Endpoint:    "https://api.abuseipdb.com/api/v2/check",
		PayloadMode: "dynamic",
		Params: []ToolParameter{
			{Key: "ipAddress", In: "query", Mode: "dynamic", Value: "{{alert.src_ip}}", Required: true},
			{Key: "maxAgeInDays", In: "query", Mode: "fixed", Value: "90"},
		},
	}

	built := BuildToolRequest(&tool, ExecutionRequest{
		Tool: "abuseipdb",
		ParamModes: map[string]string{
			"ipAddress":    "dynamic",
			"maxAgeInDays": "fixed",
		},
		InputPayload: map[string]string{
			"maxAgeInDays": "30",
		},
		RuntimeInputs: map[string]string{
			"ipAddress": "203.0.113.77",
		},
	})
	if len(built.MissingRequired) != 0 {
		t.Fatalf("expected no missing params, got %v", built.MissingRequired)
	}
	if built.URL != "https://api.abuseipdb.com/api/v2/check?ipAddress=203.0.113.77&maxAgeInDays=30" {
		t.Fatalf("unexpected URL: %q", built.URL)
	}
}

func TestSanitizePlaybookForSaveStripsRuntimeValues(t *testing.T) {
	toolMap := map[string]ToolDefinition{
		"jira_search": {
			ID:          "jira_search",
			PayloadMode: "dynamic",
			Params: []ToolParameter{
				{Key: "jql", In: "query", Mode: "dynamic", Value: `text ~ "{{alert.src_ip}}"`},
				{Key: "maxResults", In: "query", Mode: "fixed", Value: "10"},
			},
		},
		"virustotal": {
			ID:          "virustotal",
			PayloadMode: "dynamic",
			Params: []ToolParameter{
				{Key: "ip", In: "path", Mode: "dynamic", Value: "{{alert.src_ip}}"},
			},
		},
	}

	doc := PlaybookDocument{
		ID:    "pb1",
		Title: "Customer SOP",
		Nodes: []Node{
			{
				ID:         "n1",
				Type:       "lookup",
				Tool:       "virustotal",
				ParamModes: map[string]string{"ip": "dynamic"},
				InputPayload: map[string]string{
					"ip": "198.51.100.42", // raw customer IP must be stripped
				},
			},
			{
				ID:         "n2",
				Type:       "lookup",
				Tool:       "jira_search",
				ParamModes: map[string]string{"jql": "dynamic", "maxResults": "fixed"},
				InputPayload: map[string]string{
					"jql":        `project = "SOC" AND text ~ "{{alert.src_ip}}"`, // template query must be kept
					"maxResults": "25",                                            // fixed param must be kept
				},
			},
		},
	}

	sanitized := SanitizePlaybookForSave(doc, toolMap)
	if len(sanitized.Nodes[0].InputPayload) != 0 {
		t.Fatalf("expected raw runtime IP to be stripped, got %v", sanitized.Nodes[0].InputPayload)
	}
	if sanitized.Nodes[1].InputPayload["jql"] != `project = "SOC" AND text ~ "{{alert.src_ip}}"` {
		t.Fatalf("expected template query to be preserved, got %q", sanitized.Nodes[1].InputPayload["jql"])
	}
	if sanitized.Nodes[1].InputPayload["maxResults"] != "25" {
		t.Fatalf("expected fixed param to be preserved, got %q", sanitized.Nodes[1].InputPayload["maxResults"])
	}
}

func TestToolAuthValidationAndNormalization(t *testing.T) {
	if _, err := NormalizeAndValidateToolAuth(&ToolAuth{Type: "oauth99"}); err == nil {
		t.Fatalf("expected error for invalid auth type")
	}
	if _, err := NormalizeAndValidateToolAuth(&ToolAuth{Type: "api_key", EnvVar: "bad-env!"}); err == nil {
		t.Fatalf("expected error for invalid env_var format")
	}
	if _, err := NormalizeAndValidateToolAuth(&ToolAuth{Type: "bearer"}); err == nil {
		t.Fatalf("expected error when neither env_var nor allow_runtime is set")
	}

	norm, err := NormalizeAndValidateToolAuth(&ToolAuth{
		Type:         "bearer",
		EnvVar:       "jira_api_token",
		AllowRuntime: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if norm.In != "header" || norm.KeyName != "Authorization" || norm.Prefix != "Bearer " || norm.EnvVar != "JIRA_API_TOKEN" {
		t.Fatalf("unexpected normalized bearer auth: %+v", norm)
	}
}

func TestResolveToolAuthAndSecretMasking(t *testing.T) {
	t.Setenv("TEST_VT_SECRET_KEY", "super-secret-env-key")

	tool := &ToolDefinition{
		ID:         "vt_test",
		Name:       "VT Test",
		Category:   "lookup",
		HTTPMethod: "GET",
		Endpoint:   "https://example.test/api/v3/ip/{{ip}}",
		Auth: &ToolAuth{
			Type:         "api_key",
			In:           "header",
			KeyName:      "x-apikey",
			EnvVar:       "TEST_VT_SECRET_KEY",
			AllowRuntime: true,
		},
		Params: []ToolParameter{
			{Key: "ip", In: "path", Mode: "dynamic", Value: "{{alert.src_ip}}", Required: true},
			{Key: "token", In: "query", Mode: "dynamic", IsSecret: true},
		},
	}

	// Env var takes precedence over runtime auth
	resolvedAuth := ResolveToolAuth(tool, "runtime-fallback")
	if resolvedAuth.Summary.Source != "env" || !resolvedAuth.Summary.Configured {
		t.Fatalf("expected env auth source, got %+v", resolvedAuth.Summary)
	}
	if strings.Contains(resolvedAuth.Summary.MaskedPreview, "super-secret-env-key") {
		t.Fatalf("masked preview leaked secret: %q", resolvedAuth.Summary.MaskedPreview)
	}

	built := BuildToolRequest(tool, ExecutionRequest{
		RuntimeContext: map[string]any{"alert": map[string]any{"src_ip": "1.2.3.4"}},
		RuntimeInputs:  map[string]string{"token": "raw-secret-token-value"},
	})
	if built.Params["token"] != "***" {
		t.Fatalf("expected secret param to be masked in Params, got %q", built.Params["token"])
	}
	if strings.Contains(built.URL, "raw-secret-token-value") {
		t.Fatalf("expected secret query param to be masked in URL, got %q", built.URL)
	}
	if built.Auth == nil || built.Auth.Source != "env" {
		t.Fatalf("expected AuthSummary on ResolvedToolRequest, got %+v", built.Auth)
	}
}

func TestSanitizePlaybookStripsSecretParamsAndAuthHeaders(t *testing.T) {
	toolMap := map[string]ToolDefinition{
		"custom_api": {
			ID: "custom_api",
			Auth: &ToolAuth{
				Type:         "api_key",
				In:           "header",
				KeyName:      "x-apikey",
				AllowRuntime: true,
			},
			Params: []ToolParameter{
				{Key: "api_secret", In: "header", Mode: "fixed", IsSecret: true},
				{Key: "window", In: "query", Mode: "fixed"},
			},
		},
	}
	doc := PlaybookDocument{
		ID: "pb-secret",
		Nodes: []Node{
			{
				ID:         "n1",
				Type:       "lookup",
				Tool:       "custom_api",
				ParamModes: map[string]string{"api_secret": "fixed", "x-apikey": "fixed", "window": "fixed"},
				InputPayload: map[string]string{
					"api_secret": "leak-me-not",
					"x-apikey":   "also-leak-me-not",
					"window":     "24h",
				},
			},
		},
	}
	sanitized := SanitizePlaybookForSave(doc, toolMap)
	if _, ok := sanitized.Nodes[0].InputPayload["api_secret"]; ok {
		t.Fatalf("expected is_secret param to be stripped on save")
	}
	if _, ok := sanitized.Nodes[0].InputPayload["x-apikey"]; ok {
		t.Fatalf("expected auth KeyName param to be stripped on save")
	}
	if sanitized.Nodes[0].InputPayload["window"] != "24h" {
		t.Fatalf("expected normal fixed param to remain, got %v", sanitized.Nodes[0].InputPayload)
	}
}

func TestRunLiveHTTPStepAuthInjectionAndMasking(t *testing.T) {
	var gotHeader string
	var gotQueryKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-apikey")
		gotQueryKey = r.URL.Query().Get("apikey")
		if gotHeader == "bad-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid key"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"verdict":"malicious","score":95}`))
	}))
	defer srv.Close()

	prevLive := Config.AllowLiveExecution
	defer func() { Config.AllowLiveExecution = prevLive }()

	tool := &ToolDefinition{
		ID:          "vt_live",
		Name:        "VT Live",
		Category:    "lookup",
		CommandType: "http-api",
		HTTPMethod:  "GET",
		Endpoint:    srv.URL + "/ip/{{ip}}",
		Auth: &ToolAuth{
			Type:         "api_key",
			In:           "header",
			KeyName:      "x-apikey",
			EnvVar:       "UNSET_TEST_KEY_FOR_LIVE",
			AllowRuntime: true,
		},
		Params: []ToolParameter{
			{Key: "ip", In: "path", Mode: "dynamic", Value: "{{alert.src_ip}}", Required: true},
		},
	}

	// 1. Disabled by server config
	Config.AllowLiveExecution = false
	resDisabled := RunLiveHTTPStep(ExecutionRequest{NodeID: "n1", Label: "check", Tool: "vt_live"}, tool)
	if resDisabled.Status != "failed" {
		t.Fatalf("expected failure when AllowLiveExecution=false")
	}

	// 2. Enabled, missing credential
	Config.AllowLiveExecution = true
	resMissing := RunLiveHTTPStep(ExecutionRequest{
		NodeID:         "n1",
		Label:          "check",
		Tool:           "vt_live",
		RuntimeContext: map[string]any{"alert": map[string]any{"src_ip": "8.8.8.8"}},
	}, tool)
	if resMissing.Status != "failed" || resMissing.Output["auth_status"] != "missing" {
		t.Fatalf("expected missing auth failure, got %+v", resMissing)
	}

	// 3. Valid runtime session key injected into header, not leaked in Output
	resOK := RunLiveHTTPStep(ExecutionRequest{
		NodeID:         "n1",
		Label:          "check",
		Tool:           "vt_live",
		RuntimeAuth:    "secret-session-123",
		RuntimeContext: map[string]any{"alert": map[string]any{"src_ip": "8.8.8.8"}},
	}, tool)
	if resOK.Status != "success" {
		t.Fatalf("expected live success, got %+v", resOK)
	}
	if gotHeader != "secret-session-123" {
		t.Fatalf("expected header x-apikey=secret-session-123, got %q", gotHeader)
	}
	rawOut, _ := json.Marshal(resOK.Output)
	if strings.Contains(string(rawOut), "secret-session-123") {
		t.Fatalf("live output leaked secret: %s", string(rawOut))
	}

	// 4. Query-based API key injected without leaking in resolved_url
	queryTool := *tool
	queryTool.Auth = &ToolAuth{
		Type:         "api_key",
		In:           "query",
		KeyName:      "apikey",
		AllowRuntime: true,
	}
	resQuery := RunLiveHTTPStep(ExecutionRequest{
		NodeID:         "n1",
		Label:          "check",
		Tool:           "vt_live",
		RuntimeAuth:    "query-secret-999",
		RuntimeContext: map[string]any{"alert": map[string]any{"src_ip": "8.8.8.8"}},
	}, &queryTool)
	if resQuery.Status != "success" || gotQueryKey != "query-secret-999" {
		t.Fatalf("expected query auth success, got status=%s key=%q", resQuery.Status, gotQueryKey)
	}
	rawQueryOut, _ := json.Marshal(resQuery.Output)
	if strings.Contains(string(rawQueryOut), "query-secret-999") {
		t.Fatalf("query auth output leaked secret: %s", string(rawQueryOut))
	}

	// 5. Upstream 401 returns clear auth error
	res401 := RunLiveHTTPStep(ExecutionRequest{
		NodeID:         "n1",
		Label:          "check",
		Tool:           "vt_live",
		RuntimeAuth:    "bad-key",
		RuntimeContext: map[string]any{"alert": map[string]any{"src_ip": "8.8.8.8"}},
	}, tool)
	if res401.Status != "failed" || res401.Output["http_status"] != http.StatusUnauthorized {
		t.Fatalf("expected 401 failure, got %+v", res401)
	}
}

