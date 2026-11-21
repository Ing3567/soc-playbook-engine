package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
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
	prevClient := liveHTTPClient
	prevValidator := validateLiveURLFunc
	liveHTTPClient = srv.Client()
	validateLiveURLFunc = func(_ *url.URL, _ []string) error { return nil }
	defer func() {
		Config.AllowLiveExecution = prevLive
		liveHTTPClient = prevClient
		validateLiveURLFunc = prevValidator
	}()

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

func TestSSRFProtection(t *testing.T) {
	blockedIPs := []string{
		"127.0.0.1",
		"10.0.0.1",
		"172.16.0.5",
		"192.168.1.1",
		"169.254.169.254",
		"100.64.0.1",
		"0.0.0.0",
		"::1",
		"fe80::1",
	}
	for _, s := range blockedIPs {
		ip := net.ParseIP(s)
		if !isBlockedIP(ip) {
			t.Fatalf("expected IP %s to be blocked", s)
		}
	}
	if isBlockedIP(net.ParseIP("8.8.8.8")) {
		t.Fatalf("expected public IP 8.8.8.8 to be allowed")
	}

	blockedURLs := []string{
		"http://localhost:8080/api",
		"http://127.0.0.1:27017/",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/admin",
		"http://mongo.internal:27017",
		"file:///etc/passwd",
	}
	for _, raw := range blockedURLs {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if err := validateLiveTargetURL(u, nil); err == nil {
			t.Fatalf("expected URL %q to be blocked by SSRF guard", raw)
		}
	}

	vtURL, _ := url.Parse("https://www.virustotal.com/api/v3/ip_addresses/8.8.8.8")
	if err := validateLiveTargetURL(vtURL, nil); err != nil {
		t.Fatalf("expected public URL to pass when allowlist empty, got %v", err)
	}
	if err := validateLiveTargetURL(vtURL, []string{"api.abuseipdb.com"}); err == nil {
		t.Fatalf("expected host not in LIVE_ALLOWED_HOSTS to be rejected")
	}
	if err := validateLiveTargetURL(vtURL, []string{"www.virustotal.com"}); err != nil {
		t.Fatalf("expected allowlisted host to pass, got %v", err)
	}

	prevLive := Config.AllowLiveExecution
	Config.AllowLiveExecution = true
	defer func() { Config.AllowLiveExecution = prevLive }()

	ssrfTool := &ToolDefinition{
		ID:          "bad_ssrf",
		Name:        "Internal Metadata",
		Category:    "lookup",
		CommandType: "http-api",
		HTTPMethod:  "GET",
		Endpoint:    "http://169.254.169.254/latest/meta-data/",
	}
	res := RunLiveHTTPStep(ExecutionRequest{NodeID: "n1", Label: "ssrf", Tool: "bad_ssrf"}, ssrfTool)
	if res.Status != "failed" {
		t.Fatalf("expected RunLiveHTTPStep to block metadata endpoint, got %+v", res)
	}
}

func TestRateLimiterAndCORS(t *testing.T) {
	if !IsOriginAllowed("http://localhost:5173", []string{"http://localhost:5173"}) {
		t.Fatalf("expected localhost:5173 to be allowed")
	}
	if IsOriginAllowed("https://evil.example", []string{"http://localhost:5173"}) {
		t.Fatalf("expected evil.example to be rejected")
	}

	limiter := NewIPRateLimiter(60, 2)
	now := time.Now()
	if !limiter.allowAt("1.2.3.4", now) || !limiter.allowAt("1.2.3.4", now) {
		t.Fatalf("expected initial burst of 2 requests to pass")
	}
	if limiter.allowAt("1.2.3.4", now) {
		t.Fatalf("expected 3rd immediate request to be rate-limited")
	}
	if !limiter.allowAt("5.6.7.8", now) {
		t.Fatalf("expected different IP to have independent bucket")
	}
	if !limiter.allowAt("1.2.3.4", now.Add(2*time.Second)) {
		t.Fatalf("expected bucket to refill after 2s")
	}
}

func TestToolAuthEnvVarRestrictions(t *testing.T) {
	blocked := []string{
		"MONGO_URI",
		"MONGO_INITDB_ROOT_PASSWORD",
		"HF_TOKEN",
		"PORT",
		"PATH",
		"HOME",
		"ALLOWED_ORIGINS",
		"AWS_SECRET_ACCESS_KEY",
		"LLM_ENDPOINT",
		"RANDOM_VAR",
	}
	for _, envName := range blocked {
		if _, err := NormalizeAndValidateToolAuth(&ToolAuth{Type: "api_key", EnvVar: envName}); err == nil {
			t.Fatalf("expected env_var %q to be rejected", envName)
		}
	}

	allowed := []string{"VT_API_KEY", "JIRA_API_TOKEN", "TOOL_SHODAN", "SIEM_AUTH"}
	for _, envName := range allowed {
		if _, err := NormalizeAndValidateToolAuth(&ToolAuth{Type: "api_key", EnvVar: envName}); err != nil {
			t.Fatalf("expected env_var %q to be allowed, got %v", envName, err)
		}
	}

	// Defense in depth: even if a blocked env_var is injected directly, ResolveToolAuth & PopulateToolAuthStatus must ignore it.
	t.Setenv("MONGO_URI", "mongodb://admin:secret@mongo:27017")
	tampered := &ToolDefinition{
		ID:   "tampered",
		Auth: &ToolAuth{Type: "api_key", In: "header", KeyName: "X-Leak", EnvVar: "MONGO_URI"},
	}
	PopulateToolAuthStatus(tampered)
	if tampered.AuthConfigured {
		t.Fatalf("expected blocked env_var MONGO_URI not to report auth_configured=true")
	}
	resolved := ResolveToolAuth(tampered, "")
	if resolved.Summary.Configured || resolved.headerValue != "" {
		t.Fatalf("expected ResolveToolAuth to refuse reading blocked env_var MONGO_URI, got %+v", resolved)
	}
}

func TestSecretPathParamMaskingAndLiveSend(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	prevLive := Config.AllowLiveExecution
	prevClient := liveHTTPClient
	prevValidator := validateLiveURLFunc
	Config.AllowLiveExecution = true
	liveHTTPClient = srv.Client()
	validateLiveURLFunc = func(_ *url.URL, _ []string) error { return nil }
	defer func() {
		Config.AllowLiveExecution = prevLive
		liveHTTPClient = prevClient
		validateLiveURLFunc = prevValidator
	}()

	tool := &ToolDefinition{
		ID:          "bot_webhook",
		Name:        "Bot Webhook",
		Category:    "action",
		CommandType: "http-api",
		HTTPMethod:  "POST",
		Endpoint:    srv.URL + "/bot/{{bot_token}}/send",
		Params: []ToolParameter{
			{Key: "bot_token", In: "path", Mode: "dynamic", Required: true, IsSecret: true},
		},
	}

	req := ExecutionRequest{
		NodeID:        "n1",
		Label:         "send alert",
		Tool:          "bot_webhook",
		RuntimeInputs: map[string]string{"bot_token": "super-secret-path-token"},
	}

	built := BuildToolRequest(tool, req)
	if strings.Contains(built.URL, "super-secret-path-token") {
		t.Fatalf("BuildToolRequest leaked secret path param in URL: %q", built.URL)
	}
	if built.Params["bot_token"] != "***" {
		t.Fatalf("expected masked bot_token param, got %q", built.Params["bot_token"])
	}

	res := RunLiveHTTPStep(req, tool)
	if res.Status != "success" {
		t.Fatalf("expected live success, got %+v", res)
	}
	if gotPath != "/bot/super-secret-path-token/send" {
		t.Fatalf("expected unmasked secret in outbound path, got %q", gotPath)
	}
	rawOut, _ := json.Marshal(res.Output)
	if strings.Contains(string(rawOut), "super-secret-path-token") {
		t.Fatalf("live output leaked secret path param: %s", string(rawOut))
	}
}

func TestScenarioValidationAndStripping(t *testing.T) {
	if _, err := NormalizeAndValidateScenario(Scenario{Title: ""}, true); err == nil {
		t.Fatalf("expected error for empty scenario title")
	}
	if _, err := NormalizeAndValidateScenario(Scenario{Title: "Bad", Difficulty: "extreme"}, true); err == nil {
		t.Fatalf("expected error for invalid difficulty")
	}
	if _, err := NormalizeAndValidateScenario(Scenario{
		Title:       "Bad GT",
		GroundTruth: &ScenarioGroundTruth{ExpectedVerdict: "unknown"},
	}, true); err == nil {
		t.Fatalf("expected error for invalid ground_truth verdict")
	}

	norm, err := NormalizeAndValidateScenario(Scenario{
		Title: "Custom Ransomware Drill",
		ToolMocks: map[string]map[string]map[string]any{
			"jira_ticket": {"default": {"ticket_id": "SOC-999"}},
		},
		GroundTruth: &ScenarioGroundTruth{ExpectedVerdict: "MALICIOUS", MaxDurationSec: 180},
	}, true)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if norm.ID != "custom_ransomware_drill" || norm.Difficulty != "easy" {
		t.Fatalf("unexpected normalized metadata: id=%q diff=%q", norm.ID, norm.Difficulty)
	}
	if _, ok := norm.ToolMocks["ticket"]; !ok {
		t.Fatalf("expected legacy jira_ticket mock key to normalize to ticket")
	}

	stripped := StripScenarioAnswers(norm)
	if stripped.GroundTruth != nil || stripped.ToolMocks != nil {
		t.Fatalf("expected StripScenarioAnswers to hide ground_truth and tool_mocks, got %+v", stripped)
	}
}

func TestScenarioDrivenSandboxExecution(t *testing.T) {
	tp := FindBuiltinScenario("ssh_brute_force_tp")
	fp := FindBuiltinScenario("internal_scanner_fp")
	phish := FindBuiltinScenario("phishing_url_manual")
	if tp == nil || fp == nil || phish == nil {
		t.Fatalf("expected all 3 builtin scenarios to exist")
	}

	vtTool := DefaultBuiltinTools()[0] // virustotal

	// Same step label ("ตรวจสอบ IP") produces opposite outcomes based on active scenario.
	resTP := RunSandboxStepWithScenario(ExecutionRequest{
		NodeID:         "n1",
		Type:           "lookup",
		Label:          "ตรวจสอบ IP ด้วย VirusTotal",
		Tool:           "virustotal",
		ScenarioID:     tp.ID,
		RuntimeContext: map[string]any{"alert": tp.Alert},
	}, &vtTool, tp)
	if resTP.Status != "success" || resTP.Output["malicious_count"] != 14 || resTP.Output["scenario_match"] != "185.220.101.42" {
		t.Fatalf("unexpected TP scenario output: %+v", resTP)
	}

	resFP := RunSandboxStepWithScenario(ExecutionRequest{
		NodeID:         "n1",
		Type:           "lookup",
		Label:          "ตรวจสอบ IP ด้วย VirusTotal",
		Tool:           "virustotal",
		ScenarioID:     fp.ID,
		RuntimeContext: map[string]any{"alert": fp.Alert},
	}, &vtTool, fp)
	if resFP.Status != "success" || resFP.Output["malicious_count"] != 0 || resFP.Output["scenario_match"] != "198.51.100.25" {
		t.Fatalf("unexpected FP scenario output: %+v", resFP)
	}

	// Target-specific match on phishing domain vs fallback to default when querying unknown IP.
	resDomain := RunSandboxStepWithScenario(ExecutionRequest{
		NodeID:        "n2",
		Type:          "lookup",
		Label:         "ตรวจสอบ Domain",
		Tool:          "virustotal",
		RuntimeInputs: map[string]string{"ip": "login-verify-office365.example"},
	}, &vtTool, phish)
	if resDomain.Output["malicious_count"] != 9 || resDomain.Output["scenario_match"] != "login-verify-office365.example" {
		t.Fatalf("expected domain-specific mock match, got %+v", resDomain.Output)
	}

	resFallback := RunSandboxStepWithScenario(ExecutionRequest{
		NodeID:        "n3",
		Type:          "lookup",
		Label:         "ตรวจสอบ IP อื่น",
		Tool:          "virustotal",
		RuntimeInputs: map[string]string{"ip": "9.9.9.9"},
	}, &vtTool, phish)
	if resFallback.Output["malicious_count"] != 6 || resFallback.Output["scenario_match"] != "default" {
		t.Fatalf("expected default mock fallback, got %+v", resFallback.Output)
	}
}

func TestStructuredDecisionConditions(t *testing.T) {
	edges := []Edge{
		{
			ID:        "e_high",
			Source:    "d1",
			Target:    "block_node",
			Label:     "ใช่ (>5)",
			Condition: &EdgeCondition{Field: "prev.malicious_count", Op: ">", Value: 5},
		},
		{
			ID:        "e_phish",
			Source:    "d1",
			Target:    "ir_node",
			Label:     "Phishing",
			Condition: &EdgeCondition{Field: "prev.verdict", Op: "contains", Value: "phishing"},
		},
		{
			ID:     "e_else",
			Source: "d1",
			Target: "close_node",
			Label:  "ไม่ใช่",
		},
	}

	pickedHigh := PickDecisionEdge(edges, map[string]any{"malicious_count": 14, "verdict": "Malicious"}, nil)
	if pickedHigh == nil || pickedHigh.ID != "e_high" {
		t.Fatalf("expected e_high for malicious_count=14, got %+v", pickedHigh)
	}

	pickedPhish := PickDecisionEdge(edges, map[string]any{"malicious_count": 2, "verdict": "Phishing Portal"}, nil)
	if pickedPhish == nil || pickedPhish.ID != "e_phish" {
		t.Fatalf("expected e_phish for phishing verdict, got %+v", pickedPhish)
	}

	pickedElse := PickDecisionEdge(edges, map[string]any{"malicious_count": 0, "verdict": "Clean"}, nil)
	if pickedElse == nil || pickedElse.ID != "e_else" {
		t.Fatalf("expected unconditional else branch e_else, got %+v", pickedElse)
	}

	// Alert context condition
	alertCond := &EdgeCondition{Field: "alert.severity", Op: "==", Value: "high"}
	if !EvaluateEdgeCondition(alertCond, nil, map[string]any{"alert": map[string]any{"severity": "high"}}) {
		t.Fatalf("expected alert.severity == high to evaluate true")
	}

	// Legacy fallback when no structured conditions exist
	legacyEdges := []Edge{
		{ID: "e_yes", Source: "d2", Target: "a1", Label: "ใช่"},
		{ID: "e_no", Source: "d2", Target: "a2", Label: "ไม่ใช่"},
	}
	if got := PickDecisionEdge(legacyEdges, map[string]any{"malicious_count": 0}, nil); got == nil || got.ID != "e_no" {
		t.Fatalf("expected legacy fallback e_no for clean output, got %+v", got)
	}
	if got := PickDecisionEdge(legacyEdges, map[string]any{"malicious_count": 3}, nil); got == nil || got.ID != "e_yes" {
		t.Fatalf("expected legacy fallback e_yes for malicious output, got %+v", got)
	}
}

func TestAuthPasswordHashAndJWT(t *testing.T) {
	if _, err := NormalizeAndValidateUsername("ab"); err == nil {
		t.Fatalf("expected short username to fail validation")
	}
	u, err := NormalizeAndValidateUsername("  Alice.SOC  ")
	if err != nil || u != "alice.soc" {
		t.Fatalf("unexpected normalized username: %q err=%v", u, err)
	}
	if _, err := HashPassword("12345"); err == nil {
		t.Fatalf("expected short password to fail validation")
	}

	hash, err := HashPassword("S0c-Tr@inee!")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if !VerifyPassword("S0c-Tr@inee!", hash) {
		t.Fatalf("expected VerifyPassword to succeed for matching password")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Fatalf("expected VerifyPassword to reject wrong password")
	}

	secret := "unit-test-auth-secret-key"
	claims := AuthClaims{
		UserID:      "usr_1",
		Username:    "trainee1",
		DisplayName: "Trainee One",
		Role:        "trainee",
		Exp:         time.Now().Add(time.Hour).Unix(),
	}
	token, err := SignAuthToken(claims, secret, time.Hour)
	if err != nil {
		t.Fatalf("SignAuthToken failed: %v", err)
	}
	parsed, err := VerifyAuthToken(token, secret, time.Now())
	if err != nil {
		t.Fatalf("VerifyAuthToken failed: %v", err)
	}
	if parsed.UserID != "usr_1" || parsed.Role != "trainee" || parsed.Username != "trainee1" {
		t.Fatalf("unexpected parsed claims: %+v", parsed)
	}
	if _, err := VerifyAuthToken(token, "wrong-secret", time.Now()); err == nil {
		t.Fatalf("expected wrong secret to fail JWT verification")
	}
	if _, err := VerifyAuthToken(token, secret, time.Now().Add(2*time.Hour)); err == nil {
		t.Fatalf("expected expired JWT to fail verification")
	}
}

func TestServerRunEngineAndManualVerdict(t *testing.T) {
	tp := FindBuiltinScenario("ssh_brute_force_tp")
	vtTool := DefaultBuiltinTools()[0]
	fwTool := DefaultBuiltinTools()[3]
	manualTool := &ToolDefinition{ID: "web_manual", Name: "Web Check", Category: "lookup", CommandType: "manual"}

	run := &SimulationRun{
		ID:            "run_test_1",
		ScenarioID:    tp.ID,
		PlaybookTitle: "SSH Triage",
		Alert:         tp.Alert,
		Mode:          "sandbox",
		Status:        "in_progress",
		NodeStatuses:  map[string]string{},
		NodeOutputs:   map[string]map[string]any{},
		StartedAt:     time.Now().Add(-45 * time.Second),
		Nodes: []Node{
			{ID: "start", Type: "terminal", Label: "Start", Tool: "none"},
			{ID: "n_vt", Type: "lookup", Label: "Check VT", Tool: "virustotal"},
			{ID: "d1", Type: "decision", Label: "Malicious?", Tool: "none"},
			{ID: "n_block", Type: "action", Label: "Block IP", Tool: "firewall_block"},
			{ID: "n_end_mal", Type: "end", Label: "Close Escalated", Tool: "none"},
			{ID: "n_end_clean", Type: "end", Label: "Close FP", Tool: "none"},
		},
		Edges: []Edge{
			{ID: "e0", Source: "start", Target: "n_vt"},
			{ID: "e1", Source: "n_vt", Target: "d1"},
			{ID: "e2", Source: "d1", Target: "n_block", Label: "ใช่", Condition: &EdgeCondition{Field: "prev.malicious_count", Op: ">", Value: 5}},
			{ID: "e3", Source: "n_block", Target: "n_end_mal"},
			{ID: "e4", Source: "d1", Target: "n_end_clean", Label: "ไม่ใช่"},
		},
	}

	// Cannot jump ahead to locked n_block before n_vt succeeds.
	if _, _, err := ExecuteRunNode(run, "n_block", RunStepRequest{}, &fwTool, tp, false); err == nil {
		t.Fatalf("expected locked node n_block to be rejected")
	}

	// Execute n_vt -> malicious_count=14 -> unlocks n_block and skips n_end_clean.
	ev1, st1, err := ExecuteRunNode(run, "n_vt", RunStepRequest{}, &vtTool, tp, false)
	if err != nil {
		t.Fatalf("n_vt failed: %v", err)
	}
	if ev1.Action != "step_executed" || ev1.Status != "success" {
		t.Fatalf("unexpected ev1: %+v", ev1)
	}
	if st1.NodeStates["n_block"].Availability != "ready" || st1.NodeStates["n_end_clean"].Availability != "skipped" {
		t.Fatalf("unexpected graph state after n_vt: %+v", st1.NodeStates)
	}

	// Execute n_block -> reaches n_end_mal.
	if _, st2, err := ExecuteRunNode(run, "n_block", RunStepRequest{}, &fwTool, tp, false); err != nil {
		t.Fatalf("n_block failed: %v", err)
	} else if st2.NodeStates["n_end_mal"].Availability != "ready" {
		t.Fatalf("expected n_end_mal ready after n_block, got %+v", st2.NodeStates)
	}

	// Manual verdict step & secret input masking in audit event.
	manualRun := &SimulationRun{
		ID:           "run_manual_1",
		Status:       "in_progress",
		NodeStatuses: map[string]string{},
		NodeOutputs:  map[string]map[string]any{},
		Nodes:        []Node{{ID: "m1", Type: "lookup", Label: "Manual Check", Tool: "web_manual"}},
	}
	evM, _, err := ExecuteRunNode(manualRun, "m1", RunStepRequest{ManualVerdict: "suspicious", Notes: "Saw phishing form"}, manualTool, nil, false)
	if err != nil {
		t.Fatalf("manual node failed: %v", err)
	}
	if evM.Action != "manual_verdict" || evM.Verdict != "suspicious" || evM.Output["is_malicious"] != true {
		t.Fatalf("unexpected manual event: %+v", evM)
	}

	// Score perfect TP run -> 100/100
	run.FinalVerdict = "malicious"
	perfect := ScoreSimulationRun(run, tp, time.Now())
	if perfect.Total != 100 {
		t.Fatalf("expected 100/100 score for complete TP run, got %+v", perfect)
	}
}

func TestScoringSimulationRunDeductions(t *testing.T) {
	tp := FindBuiltinScenario("ssh_brute_force_tp")
	started := time.Now().Add(-600 * time.Second) // 600s > 300s max (>1.5x -> 0 speed pts)
	run := &SimulationRun{
		ID:           "run_partial",
		ScenarioID:   tp.ID,
		StartedAt:    started,
		FinalVerdict: "suspicious", // partial match vs "malicious" -> 20/40
		NodeStatuses: map[string]string{"n1": "success"},
		NodeOutputs:  map[string]map[string]any{"n1": {"malicious_count": 14}},
		Nodes: []Node{
			{ID: "n1", Type: "lookup", Label: "Check VT", Tool: "virustotal"},
			{ID: "n2", Type: "action", Label: "Block IP", Tool: "firewall_block"},
			{ID: "n3", Type: "end", Label: "End", Tool: "none"},
		},
		Edges: []Edge{
			{ID: "e1", Source: "n1", Target: "n2"},
			{ID: "e2", Source: "n2", Target: "n3"},
		},
	}

	sc := ScoreSimulationRun(run, tp, time.Now())
	if sc.VerdictPoints != 20 {
		t.Fatalf("expected 20 verdict pts for suspicious vs malicious, got %d", sc.VerdictPoints)
	}
	if len(sc.MissedNodes) != 1 || sc.MissedNodes[0] != "Block IP" {
		t.Fatalf("expected MissedNodes=[Block IP], got %v", sc.MissedNodes)
	}
	if sc.PathPoints != 0 || sc.SpeedPoints != 0 {
		t.Fatalf("expected 0 path and 0 speed pts, got path=%d speed=%d", sc.PathPoints, sc.SpeedPoints)
	}
	if sc.Total >= 60 {
		t.Fatalf("expected low total score for incomplete slow run, got %+v", sc)
	}
}

func TestPlaybookVersioningSnapshot(t *testing.T) {
	t1 := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	doc1, snap1 := PreparePlaybookVersion(PlaybookDocument{
		Title: "Phishing SOP",
		Nodes: []Node{{ID: "n1", Type: "lookup", Label: "Check VT", Tool: "virustotal"}},
	}, 0, t1)
	if doc1.Version != 1 || doc1.ID == "" {
		t.Fatalf("expected version 1 and generated ID, got v=%d id=%q", doc1.Version, doc1.ID)
	}
	if snap1.PlaybookID != doc1.ID || snap1.Version != 1 || snap1.ID != doc1.ID+"_v1" {
		t.Fatalf("unexpected v1 snapshot: %+v", snap1)
	}

	t2 := t1.Add(5 * time.Minute)
	doc2, snap2 := PreparePlaybookVersion(doc1, doc1.Version, t2)
	if doc2.Version != 2 || snap2.Version != 2 || snap2.ID != doc1.ID+"_v2" {
		t.Fatalf("expected version 2 increment, got doc.v=%d snap=%+v", doc2.Version, snap2)
	}
}
