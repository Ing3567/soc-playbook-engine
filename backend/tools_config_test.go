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

