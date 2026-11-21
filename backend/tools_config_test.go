package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMockupTicketCreateAndSearchFlow(t *testing.T) {
	ctx := context.Background()

	// 1. Create a new ticket for IP 203.0.113.55 using legacy ID "jira_ticket" to also verify normalization
	createRes := ExecuteToolStep(ctx, ExecutionRequest{
		NodeID: "n4",
		Type:   "action",
		Label:  "เปิดเคสแจ้งเตือนไปยัง Tier 2 สำหรับ 203.0.113.55",
		Tool:   "jira_ticket",
	})
	if createRes.Status != "success" || createRes.Mode != "mockup" {
		t.Fatalf("expected mockup success for ticket creation, got status=%s mode=%s err=%s", createRes.Status, createRes.Mode, createRes.Error)
	}
	createdID, _ := createRes.Output["ticket_id"].(string)
	if !strings.HasPrefix(createdID, "TCK-") {
		t.Fatalf("expected ticket_id starting with TCK-, got %q", createdID)
	}
	toolName, _ := createRes.Output["tool"].(string)
	if strings.Contains(strings.ToLower(toolName), "jira") {
		t.Errorf("expected no Jira branding in tool output, got %q", toolName)
	}

	// 2. Search for tickets matching IP 203.0.113.55 via ticket_search
	searchRes := ExecuteToolStep(ctx, ExecutionRequest{
		NodeID: "n2",
		Type:   "lookup",
		Label:  "ค้นหาประวัติเคสเก่าในระบบ",
		Tool:   "ticket_search",
		Inputs: map[string]string{"ip": "203.0.113.55"},
	})
	if searchRes.Status != "success" || searchRes.Mode != "mockup" {
		t.Fatalf("expected mockup success for ticket_search, got status=%s mode=%s", searchRes.Status, searchRes.Mode)
	}
	matchedCases, ok := searchRes.Output["matched_cases"].([]string)
	if !ok || len(matchedCases) == 0 || matchedCases[0] != createdID {
		t.Fatalf("expected ticket_search to find newly created ticket %s, got %+v", createdID, searchRes.Output)
	}
}

func TestRealHTTPToolExecutionAndSecretIsolation(t *testing.T) {
	const secretToken = "super-secret-fw-token-999"
	t.Setenv("TEST_FW_TOKEN", secretToken)

	var receivedAuthHeader string
	var receivedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"rule_id":"FW-REAL-777","status":"blocked"}`))
	}))
	defer server.Close()

	t.Setenv("FIREWALL_API_URL", server.URL+"/api/block")
	t.Setenv("FIREWALL_API_KEY", secretToken)

	res := ExecuteToolStep(context.Background(), ExecutionRequest{
		NodeID: "n3",
		Type:   "action",
		Label:  "บล็อก IP บน Firewall",
		Tool:   "firewall_block",
		Inputs: map[string]string{"ip": "198.51.100.77"},
	})

	if res.Status != "success" || res.Mode != "real" {
		t.Fatalf("expected real execution success, got status=%s mode=%s err=%s", res.Status, res.Mode, res.Error)
	}
	if receivedAuthHeader != "Bearer "+secretToken {
		t.Errorf("expected Authorization header with Bearer token, got %q", receivedAuthHeader)
	}
	if receivedBody["ip"] != "198.51.100.77" || receivedBody["action"] != "deny" {
		t.Errorf("unexpected request body sent to firewall server: %+v", receivedBody)
	}

	rawOut, _ := json.Marshal(res.Output)
	if strings.Contains(string(rawOut), secretToken) {
		t.Errorf("secret token leaked into execution output: %s", string(rawOut))
	}
}

func TestSandboxFallbackWhenEnvNotConfigured(t *testing.T) {
	t.Setenv("VT_API_KEY", "")

	res := ExecuteToolStep(context.Background(), ExecutionRequest{
		NodeID: "n1",
		Type:   "lookup",
		Label:  "ตรวจสอบ IP 8.8.8.8 ใน VirusTotal",
		Tool:   "virustotal",
	})
	if res.Status != "success" || res.Mode != "sandbox_fallback" {
		t.Fatalf("expected sandbox_fallback when VT_API_KEY is empty, got status=%s mode=%s", res.Status, res.Mode)
	}
	reason, _ := res.Output["fallback_reason"].(string)
	if !strings.Contains(reason, "VT_API_KEY") {
		t.Errorf("expected fallback_reason to mention VT_API_KEY, got %q", reason)
	}
}

func TestKeywordMatchingAndToolsEndpoint(t *testing.T) {
	if got := MatchToolByKeywords("lookup", "ค้นหาประวัติเคสเก่าในระบบ"); got != "ticket_search" {
		t.Errorf("expected ticket_search for historical case lookup, got %q", got)
	}
	if got := MatchToolByKeywords("action", "เปิดเคสแจ้งเตือนไปยัง Tier 2"); got != "ticket" {
		t.Errorf("expected ticket for opening case action, got %q", got)
	}
	if got := MatchToolByKeywords("lookup", "ค้นหา Log ใน Splunk SIEM"); got != "splunk" {
		t.Errorf("expected splunk for SIEM lookup, got %q", got)
	}

	gin.SetMode(gin.TestMode)
	handler := NewApiHandler(NewInMemoryPlaybookRepository())
	router := gin.New()
	handler.RegisterRoutes(router)

	req := httptest.NewRequest(http.MethodGet, "/api/tools", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/tools returned %d", rec.Code)
	}

	var tools []ToolStatusSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &tools); err != nil {
		t.Fatalf("decode /api/tools: %v", err)
	}
	if len(tools) < 6 {
		t.Errorf("expected at least 6 configured tools, got %d", len(tools))
	}

	// Test POST /api/execute via HTTP router
	execPayload, _ := json.Marshal(ExecutionRequest{
		NodeID: "n1",
		Type:   "lookup",
		Label:  "ค้นหาเคสเก่าของ IP 185.220.101.42",
		Tool:   "ticket_search",
	})
	execReq := httptest.NewRequest(http.MethodPost, "/api/execute", bytes.NewReader(execPayload))
	execReq.Header.Set("Content-Type", "application/json")
	execRec := httptest.NewRecorder()
	router.ServeHTTP(execRec, execReq)
	if execRec.Code != http.StatusOK {
		t.Fatalf("POST /api/execute returned %d", execRec.Code)
	}
}

func TestTicketSearchFailingAPIFallsBackSafely(t *testing.T) {
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream ticket api down", http.StatusInternalServerError)
	}))
	defer failingServer.Close()

	t.Setenv("TICKET_SEARCH_API_URL", failingServer.URL+"/search")

	res := ExecuteToolStep(context.Background(), ExecutionRequest{
		NodeID: "n5",
		Type:   "lookup",
		Label:  "ค้นหาประวัติเคสเก่า",
		Tool:   "ticket_search",
		Inputs: map[string]string{"ip": "185.220.101.42"},
	})
	if res.Status != "success" || res.Mode != "sandbox_fallback" {
		t.Fatalf("expected graceful sandbox_fallback when external ticket API fails, got status=%s mode=%s err=%s", res.Status, res.Mode, res.Error)
	}
	if reason, _ := res.Output["fallback_reason"].(string); !strings.Contains(reason, "500") {
		t.Errorf("expected fallback_reason with HTTP 500, got %q", reason)
	}
	if count, _ := res.Output["matched_count"].(int); count == 0 {
		t.Errorf("expected internal mockup tickets returned on fallback, got %+v", res.Output)
	}
}

func TestSubstituteEndpointVarsPathEscaping(t *testing.T) {
	t.Setenv("SPLUNK_URL", "https://splunk.local:8089/")
	resolved := substituteEndpointVars("{{env.SPLUNK_URL}}/services/search/{{ip}}", map[string]string{
		"ip": "../../admin?x=1",
	})
	if strings.Contains(resolved, "//services") {
		t.Errorf("expected trimmed trailing slash on env base URL, got %q", resolved)
	}
	if strings.Contains(resolved, "/../") || strings.Contains(resolved, "?x=1") {
		t.Errorf("expected path-escaped user input in endpoint URL, got %q", resolved)
	}
}

func TestVirusTotalSmartRoutingAndUIBeforeEnvPriority(t *testing.T) {
	var seenPaths []string
	var seenKeys []string

	vtServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPaths = append(seenPaths, r.URL.Path)
		seenKeys = append(seenKeys, r.Header.Get("x-apikey"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"ok","type":"ip_address","attributes":{"country":"US","as_owner":"Cloudflare","last_analysis_stats":{"malicious":2,"suspicious":0,"harmless":80}}}}`))
	}))
	defer vtServer.Close()

	t.Setenv("VT_BASE_URL", vtServer.URL)
	t.Setenv("VT_API_KEY", "env-vt-key-fallback")

	// 1. UI API key takes priority over .env key for IP lookup
	resIP := ExecuteToolStep(context.Background(), ExecutionRequest{
		NodeID: "n1",
		Type:   "lookup",
		Label:  "ตรวจสอบบน VirusTotal",
		Tool:   "virustotal",
		Inputs: map[string]string{
			"target":  "8.8.8.8",
			"api_key": "ui-vt-key-priority",
		},
	})
	if resIP.Status != "success" || resIP.Mode != "real" {
		t.Fatalf("expected VT real IP success, got status=%s mode=%s err=%s", resIP.Status, resIP.Mode, resIP.Error)
	}

	// 2. Empty UI API key falls back to .env key for Domain lookup
	resDomain := ExecuteToolStep(context.Background(), ExecutionRequest{
		NodeID: "n2",
		Type:   "lookup",
		Label:  "ตรวจสอบโดเมนบน VirusTotal",
		Tool:   "virustotal",
		Inputs: map[string]string{"target": "malicious.example.com"},
	})
	if resDomain.Status != "success" || resDomain.Mode != "real" {
		t.Fatalf("expected VT real domain success, got status=%s mode=%s err=%s", resDomain.Status, resDomain.Mode, resDomain.Error)
	}

	// 3. SHA256 hash routes to /files/{hash}
	sha := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	resHash := ExecuteToolStep(context.Background(), ExecutionRequest{
		NodeID: "n3",
		Type:   "lookup",
		Label:  "ตรวจสอบ Hash บน VirusTotal",
		Tool:   "virustotal",
		Inputs: map[string]string{"target": sha},
	})
	if resHash.Status != "success" || resHash.Mode != "real" {
		t.Fatalf("expected VT real hash success, got status=%s mode=%s err=%s", resHash.Status, resHash.Mode, resHash.Error)
	}

	if len(seenPaths) != 3 || seenPaths[0] != "/ip_addresses/8.8.8.8" || seenPaths[1] != "/domains/malicious.example.com" || seenPaths[2] != "/files/"+sha {
		t.Errorf("unexpected VT smart routes: %v", seenPaths)
	}
	if len(seenKeys) != 3 || seenKeys[0] != "ui-vt-key-priority" || seenKeys[1] != "env-vt-key-fallback" {
		t.Errorf("expected UI key priority then .env fallback, got %v", seenKeys)
	}
}

func TestShodanSmartRoutingAndSecretRedaction(t *testing.T) {
	if got := MatchToolByKeywords("lookup", "ค้นหาพอร์ตที่เปิดบน Shodan"); got != "shodan" {
		t.Fatalf("expected shodan tool match, got %q", got)
	}
	if got := inferStepType("ค้นหา Shodan"); got != "lookup" {
		t.Fatalf("expected lookup step type for Shodan, got %q", got)
	}

	const uiShodanKey = "shodan-ui-secret-key-888"
	var seenRequests []string

	shodanServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenRequests = append(seenRequests, r.URL.Path+"?"+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/shodan/host/search") {
			_, _ = w.Write([]byte(`{"total":2,"matches":[{"ip_str":"1.2.3.4","port":22,"org":"TestNet"}]}`))
			return
		}
		if r.URL.Path == "/shodan/host/9.9.9.9" {
			http.Error(w, "invalid key: "+uiShodanKey, http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"ip_str":"8.8.8.8","org":"Google LLC","country_name":"United States","ports":[53,443]}`))
	}))
	defer shodanServer.Close()

	t.Setenv("SHODAN_BASE_URL", shodanServer.URL)
	t.Setenv("SHODAN_API_KEY", "")

	// 1. Shodan IP lookup using UI API key directly (with empty .env!)
	resIP := ExecuteToolStep(context.Background(), ExecutionRequest{
		NodeID: "n1",
		Type:   "lookup",
		Label:  "ตรวจสอบ IP บน Shodan",
		Tool:   "shodan",
		Inputs: map[string]string{
			"query":   "8.8.8.8",
			"api_key": uiShodanKey,
		},
	})
	if resIP.Status != "success" || resIP.Mode != "real" {
		t.Fatalf("expected Shodan real IP lookup success, got status=%s mode=%s err=%s", resIP.Status, resIP.Mode, resIP.Error)
	}
	if resIP.Output["org"] != "Google LLC" {
		t.Errorf("unexpected Shodan host output: %+v", resIP.Output)
	}

	// 2. Shodan query search
	resSearch := ExecuteToolStep(context.Background(), ExecutionRequest{
		NodeID: "n2",
		Type:   "lookup",
		Label:  "ค้นหา Shodan",
		Tool:   "shodan",
		Inputs: map[string]string{
			"query":   "port:22",
			"api_key": uiShodanKey,
		},
	})
	if resSearch.Status != "success" || resSearch.Mode != "real" {
		t.Fatalf("expected Shodan search success, got status=%s mode=%s err=%s", resSearch.Status, resSearch.Mode, resSearch.Error)
	}

	// 3. Error echoing the UI key MUST be redacted
	resErr := ExecuteToolStep(context.Background(), ExecutionRequest{
		NodeID: "n3",
		Type:   "lookup",
		Label:  "ตรวจสอบ Shodan",
		Tool:   "shodan",
		Inputs: map[string]string{
			"query":   "9.9.9.9",
			"api_key": uiShodanKey,
		},
	})
	if resErr.Status != "failed" {
		t.Fatalf("expected failed status for 401 response, got %s", resErr.Status)
	}
	if strings.Contains(resErr.Error, uiShodanKey) {
		t.Errorf("UI API key leaked into error message: %s", resErr.Error)
	}
	if !strings.Contains(resErr.Error, "***REDACTED***") {
		t.Errorf("expected redacted marker in error message, got: %s", resErr.Error)
	}

	// 4. Verify ListToolStatuses includes input_fields for searchable tools
	foundShodanFields := false
	for _, s := range ListToolStatuses() {
		if s.ID == "shodan" && len(s.InputFields) == 2 && s.InputFields[0].Type == "text" && s.InputFields[1].Type == "password" {
			foundShodanFields = true
		}
	}
	if !foundShodanFields {
		t.Errorf("expected shodan to expose text + password input_fields in ListToolStatuses")
	}
}


