package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// SINGLE-FILE TOOL CONFIGURATION (backend/tools_config.go)
// ============================================================================
// ไฟล์นี้เป็นศูนย์กลางเดียวสำหรับการเพิ่ม แก้ไข หรือตั้งค่า Real Tools และ Mockup Tools
// หากต้องการเพิ่มหรือแก้ Tool:
//   1. แก้ไขหรือเพิ่มรายการในตัวแปร ConfiguredTools ด้านล่างนี้
//   2. ระบุ InputFields เพื่อให้ระบบสร้างช่องพิมพ์ (text / password) ฝังลงใน Node อัตโนมัติ
//      - หากช่องมี EnvFallback (เช่น VT_API_KEY) ระบบจะใช้ค่าที่พิมพ์ในหน้า Node ก่อน
//        และหากเว้นว่างไว้จะดึงค่าจากไฟล์ .env มาใช้แทนโดยอัตโนมัติ
//   3. ระบบจะนำ Keywords ไปจับคู่กับ Node ใน Flowchart อัตโนมัติ (ไม่ต้องแก้ flow_builder.go)
//   4. หากยังไม่ได้กรอกค่าและไม่ได้ตั้งค่าใน .env (เมื่อ FallbackToSandbox: true)
//      ระบบจะสลับไปรันโหมดจำลอง (Sandbox) อัตโนมัติโดยไม่ทำให้ระบบพัง
// ============================================================================

type ToolInputField struct {
	Key         string `json:"key"`                   // ชื่อตัวแปร เช่น "target", "ip", "query", "api_key"
	Label       string `json:"label"`                 // ป้ายชื่อช่องบน Node
	Placeholder string `json:"placeholder,omitempty"` // ข้อความตัวอย่างในช่องพิมพ์
	Type        string `json:"type"`                  // "text" หรือ "password"
	Required    bool   `json:"required,omitempty"`
	EnvFallback string `json:"-"`                     // ตัวแปร .env สำรองกรณีไม่ได้กรอกในหน้า Node
}

type ToolConfig struct {
	ID                 string
	Name               string
	Category           string // "lookup" หรือ "action"
	Keywords           []string
	InputFields        []ToolInputField
	Enabled            bool
	FallbackToSandbox  bool
	RequiredEnv        []string // Env ที่ต้องมีใน .env (สำหรับ URL ภายในที่ไม่ได้กรอกใน Node)
	RequiredInputs     []string // Input keys ที่ต้องมีค่า (จากช่อง Node, env fallback, หรือ label/link)
	Method             string
	Endpoint           string
	Headers            map[string]string
	QueryParams        map[string]string
	BodyTemplate       map[string]any
	TimeoutSec         int
	InsecureSkipVerify bool
	ResponseMapper     func(statusCode int, rawBody []byte) (map[string]any, error)
	CustomExecute      func(ctx context.Context, tool *ToolConfig, req ExecutionRequest, vars map[string]string) (output map[string]any, mode string, err error)
}

type ToolStatusSummary struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Category    string           `json:"category"`
	Enabled     bool             `json:"enabled"`
	ReadyReal   bool             `json:"ready_real"`
	Mode        string           `json:"mode"`
	MissingEnv  []string         `json:"missing_env,omitempty"`
	InputFields []ToolInputField `json:"input_fields,omitempty"`
}

// NormalizeToolID แปลงรหัส Tool เก่าให้เป็นรหัสมาตรฐานปัจจุบันโดยไม่ผูกกับชื่อแบรนด์ภายนอก
func NormalizeToolID(id string) string {
	trimmed := strings.ToLower(strings.TrimSpace(id))
	switch trimmed {
	case "jira_search", "case_search":
		return "ticket_search"
	case "jira_ticket":
		return "ticket"
	default:
		return trimmed
	}
}

// ConfiguredTools คือรายการ Tool ทั้งหมดในระบบ (แก้ไขไฟล์นี้ไฟล์เดียวเพื่อเพิ่ม/ปรับแต่ง Tool)
var ConfiguredTools = []ToolConfig{
	// 1. VirusTotal — ค้นหาได้ทั้ง IP / Domain / File Hash (ใช้ API Key จากช่องใน Node ก่อน .env)
	{
		ID:       "virustotal",
		Name:     "VirusTotal Threat Lookup",
		Category: "lookup",
		Keywords: []string{"virustotal", "vt"},
		InputFields: []ToolInputField{
			{
				Key:         "target",
				Label:       "ค้นหา IP / Domain / Hash",
				Placeholder: "เช่น 8.8.8.8, evil.com, SHA256",
				Type:        "text",
				Required:    true,
			},
			{
				Key:         "api_key",
				Label:       "VT API Key",
				Placeholder: "กรอก Key (เว้นว่างใช้ .env)",
				Type:        "password",
				EnvFallback: "VT_API_KEY",
			},
		},
		Enabled:           true,
		FallbackToSandbox: true,
		RequiredInputs:    []string{"target", "api_key"},
		Method:            "GET",
		Endpoint:          "https://www.virustotal.com/api/v3/ip_addresses/{{target}}",
		Headers: map[string]string{
			"x-apikey": "{{api_key}}",
			"Accept":   "application/json",
		},
		TimeoutSec:     15,
		ResponseMapper: mapVirusTotalResponse,
		CustomExecute:  executeVirusTotalSmart,
	},

	// 2. AbuseIPDB — ตรวจสอบชื่อเสียง IP (ใช้ API Key จากช่องใน Node ก่อน .env)
	{
		ID:       "abuseipdb",
		Name:     "AbuseIPDB Reputation Check",
		Category: "lookup",
		Keywords: []string{"abuseipdb", "reputation"},
		InputFields: []ToolInputField{
			{
				Key:         "ip",
				Label:       "ค้นหา IP Address",
				Placeholder: "เช่น 185.220.101.42",
				Type:        "text",
				Required:    true,
			},
			{
				Key:         "api_key",
				Label:       "AbuseIPDB Key",
				Placeholder: "กรอก Key (เว้นว่างใช้ .env)",
				Type:        "password",
				EnvFallback: "ABUSEIPDB_API_KEY",
			},
		},
		Enabled:           true,
		FallbackToSandbox: true,
		RequiredInputs:    []string{"ip", "api_key"},
		Method:            "GET",
		Endpoint:          "https://api.abuseipdb.com/api/v2/check",
		QueryParams: map[string]string{
			"ipAddress":    "{{ip}}",
			"maxAgeInDays": "90",
		},
		Headers: map[string]string{
			"Key":    "{{api_key}}",
			"Accept": "application/json",
		},
		TimeoutSec:     15,
		ResponseMapper: mapAbuseIPDBResponse,
	},

	// 3. Shodan — ค้นหา Host IP หรือ Shodan Query (ใช้ API Key จากช่องใน Node ก่อน .env)
	{
		ID:       "shodan",
		Name:     "Shodan Host & Search",
		Category: "lookup",
		Keywords: []string{"shodan"},
		InputFields: []ToolInputField{
			{
				Key:         "query",
				Label:       "ค้นหา Shodan (IP / Query)",
				Placeholder: "เช่น 8.8.8.8 หรือ port:22",
				Type:        "text",
				Required:    true,
			},
			{
				Key:         "api_key",
				Label:       "Shodan API Key",
				Placeholder: "กรอก Key (เว้นว่างใช้ .env)",
				Type:        "password",
				EnvFallback: "SHODAN_API_KEY",
			},
		},
		Enabled:           true,
		FallbackToSandbox: true,
		RequiredInputs:    []string{"query", "api_key"},
		Method:            "GET",
		Endpoint:          "https://api.shodan.io/shodan/host/{{query}}",
		TimeoutSec:        15,
		ResponseMapper:    mapShodanResponse,
		CustomExecute:     executeShodanSmart,
	},

	// 4. Splunk SIEM — ดึง SPL อัตโนมัติจากลิงก์ใน Node หรือพิมพ์แก้ไขในช่อง (Token ใน Node ก่อน .env)
	{
		ID:       "splunk",
		Name:     "Splunk SIEM Search",
		Category: "lookup",
		Keywords: []string{"splunk", "siem"},
		InputFields: []ToolInputField{
			{
				Key:         "query",
				Label:       "Splunk Query (SPL)",
				Placeholder: "เช่น index=auth action=failure",
				Type:        "text",
				Required:    true,
			},
			{
				Key:         "token",
				Label:       "Splunk Token",
				Placeholder: "กรอก Token (เว้นว่างใช้ .env)",
				Type:        "password",
				EnvFallback: "SPLUNK_TOKEN",
			},
		},
		Enabled:           true,
		FallbackToSandbox: true,
		RequiredEnv:       []string{"SPLUNK_URL"},
		RequiredInputs:    []string{"query", "token"},
		Method:            "POST",
		Endpoint:          "{{env.SPLUNK_URL}}/services/search/jobs/export",
		QueryParams: map[string]string{
			"output_mode": "json",
			"search":      "{{query}}",
		},
		Headers: map[string]string{
			"Authorization": "Bearer {{token}}",
			"Accept":        "application/json",
		},
		TimeoutSec:         20,
		InsecureSkipVerify: true,
	},

	// 5. Ticket Search — ระบบค้นหาทิกเก็ต/เคสเก่าจำลองภายใน (ไม่ใช้ชื่อ Jira)
	{
		ID:       "ticket_search",
		Name:     "Ticket Search System",
		Category: "lookup",
		Keywords: []string{"เคสเก่า", "ประวัติเคส", "ค้นหาเคส", "ticket search", "case search", "ค้นหา ticket", "ประวัติ"},
		InputFields: []ToolInputField{
			{
				Key:         "query",
				Label:       "ค้นหา Ticket / IP",
				Placeholder: "เช่น 185.220.101.42 หรือ TCK-1024",
				Type:        "text",
			},
		},
		Enabled:           true,
		FallbackToSandbox: true,
		Method:            "GET",
		Endpoint:          "{{env.TICKET_SEARCH_API_URL}}",
		QueryParams: map[string]string{
			"q":  "{{query}}",
			"ip": "{{ip}}",
		},
		TimeoutSec:    10,
		CustomExecute: executeMockupTicketSearch,
	},

	// 6. Firewall Block — สั่งบล็อก IP บน Firewall (Token ใน Node ก่อน .env)
	{
		ID:       "firewall_block",
		Name:     "Firewall Block Controller",
		Category: "action",
		Keywords: []string{"บล็อก", "ระงับ", "block", "firewall"},
		InputFields: []ToolInputField{
			{
				Key:         "ip",
				Label:       "IP ที่ต้องการบล็อก",
				Placeholder: "เช่น 185.220.101.42",
				Type:        "text",
				Required:    true,
			},
			{
				Key:         "api_key",
				Label:       "Firewall Token",
				Placeholder: "กรอก Token (เว้นว่างใช้ .env)",
				Type:        "password",
				EnvFallback: "FIREWALL_API_KEY",
			},
		},
		Enabled:           true,
		FallbackToSandbox: true,
		RequiredEnv:       []string{"FIREWALL_API_URL"},
		RequiredInputs:    []string{"ip", "api_key"},
		Method:            "POST",
		Endpoint:          "{{env.FIREWALL_API_URL}}",
		Headers: map[string]string{
			"Authorization": "Bearer {{api_key}}",
			"Content-Type":  "application/json",
		},
		BodyTemplate: map[string]any{
			"ip":     "{{ip}}",
			"action": "deny",
			"reason": "{{label}}",
		},
		TimeoutSec: 15,
	},

	// 7. Ticket System — ระบบเปิดเคส/บันทึกทิกเก็ตจำลองภายใน (เชื่อมต่อกับ ticket_search)
	{
		ID:       "ticket",
		Name:     "Ticket Management System",
		Category: "action",
		Keywords: []string{"ticket", "เปิดเคส", "บันทึกเคส", "เคส", "tier"},
		InputFields: []ToolInputField{
			{
				Key:         "ip",
				Label:       "Target IP (ถ้ามี)",
				Placeholder: "เช่น 185.220.101.42",
				Type:        "text",
			},
		},
		Enabled:           true,
		FallbackToSandbox: true,
		Method:            "POST",
		Endpoint:          "{{env.TICKET_API_URL}}",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		BodyTemplate: map[string]any{
			"summary":   "{{label}}",
			"target_ip": "{{ip}}",
			"priority":  "High",
		},
		TimeoutSec:    10,
		CustomExecute: executeMockupTicketCreate,
	},

	/*
		// -------------------------------------------------------------------------
		// ตัวอย่างการเพิ่ม Real Tool ใหม่ (Copy บล็อกนี้แล้วเปิดคอมเมนต์เพื่อใช้งานได้เลย)
		// -------------------------------------------------------------------------
		{
			ID:       "slack_notify",
			Name:     "Slack SOC Alert Webhook",
			Category: "action",
			Keywords: []string{"slack", "แจ้งเตือนแชท", "notify"},
			InputFields: []ToolInputField{
				{Key: "message", Label: "ข้อความเพิ่มเติม", Placeholder: "รายละเอียดแจ้งเตือน", Type: "text"},
				{Key: "webhook_url", Label: "Webhook URL", Placeholder: "กรอก URL (เว้นว่างใช้ .env)", Type: "password", EnvFallback: "SLACK_WEBHOOK_URL"},
			},
			Enabled:           true,
			FallbackToSandbox: true,
			RequiredInputs:    []string{"webhook_url"},
			Method:            "POST",
			Endpoint:          "{{webhook_url}}",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			BodyTemplate: map[string]any{
				"text": "🚨 [SOC Playbook] {{label}} {{message}}",
			},
			TimeoutSec: 10,
		},
	*/
}

// ============================================================================
// SMART REAL TOOL HANDLERS (VIRUSTOTAL & SHODAN)
// ============================================================================

func executeVirusTotalSmart(ctx context.Context, tool *ToolConfig, req ExecutionRequest, vars map[string]string) (map[string]any, string, error) {
	target := strings.TrimSpace(vars["target"])
	if target == "" {
		target = strings.TrimSpace(vars["ip"])
	}
	baseURL := "https://www.virustotal.com/api/v3"
	if customBase := strings.TrimRight(strings.TrimSpace(os.Getenv("VT_BASE_URL")), "/"); customBase != "" {
		baseURL = customBase
	}

	endpoint := baseURL + "/domains/{{target}}"
	switch {
	case ipv4ExactRegex.MatchString(target):
		endpoint = baseURL + "/ip_addresses/{{target}}"
	case hashExactRegex.MatchString(target):
		endpoint = baseURL + "/files/{{target}}"
	}

	clone := *tool
	clone.Endpoint = endpoint
	out, err := executeHTTPConfigTool(ctx, &clone, req, vars)
	return out, "real", err
}

func executeShodanSmart(ctx context.Context, tool *ToolConfig, req ExecutionRequest, vars map[string]string) (map[string]any, string, error) {
	query := strings.TrimSpace(vars["query"])
	if query == "" {
		query = strings.TrimSpace(vars["ip"])
	}
	baseURL := "https://api.shodan.io"
	if customBase := strings.TrimRight(strings.TrimSpace(os.Getenv("SHODAN_BASE_URL")), "/"); customBase != "" {
		baseURL = customBase
	}

	clone := *tool
	if ipv4ExactRegex.MatchString(query) {
		clone.Endpoint = baseURL + "/shodan/host/{{query}}"
		clone.QueryParams = map[string]string{
			"key": "{{api_key}}",
		}
	} else {
		clone.Endpoint = baseURL + "/shodan/host/search"
		clone.QueryParams = map[string]string{
			"key":   "{{api_key}}",
			"query": "{{query}}",
		}
	}

	out, err := executeHTTPConfigTool(ctx, &clone, req, vars)
	return out, "real", err
}

// ============================================================================
// INTERNAL MOCKUP TICKET SEARCH & TICKET MANAGEMENT SYSTEM
// ============================================================================

type MockTicketRecord struct {
	TicketID   string `json:"ticket_id"`
	Title      string `json:"title"`
	TargetIP   string `json:"target_ip,omitempty"`
	Status     string `json:"status"`
	Severity   string `json:"severity"`
	Assignee   string `json:"assignee"`
	Resolution string `json:"resolution,omitempty"`
	CreatedAt  string `json:"created_at"`
}

var (
	mockTicketMu      sync.RWMutex
	mockTicketCounter = 2000
	mockTicketRecords = []MockTicketRecord{
		{
			TicketID:   "TCK-1024",
			Title:      "ตรวจพบการพยายาม Brute-Force เข้าระบบจาก IP ภายนอก",
			TargetIP:   "185.220.101.42",
			Status:     "Closed",
			Severity:   "High",
			Assignee:   "SOC Tier 2",
			Resolution: "บล็อก IP บน Firewall และรีเซ็ตรหัสผ่านผู้ใช้งาน",
			CreatedAt:  "2026-11-18 09:15:00",
		},
		{
			TicketID:   "TCK-1089",
			Title:      "แจ้งเตือน Port Scan และพฤติกรรมต้องสงสัยซ้ำซ้อน",
			TargetIP:   "45.155.205.233",
			Status:     "In Progress",
			Severity:   "Medium",
			Assignee:   "SOC Tier 1",
			Resolution: "อยู่ระหว่างเฝ้าระวังพฤติกรรม 24 ชั่วโมง",
			CreatedAt:  "2026-11-20 14:42:10",
		},
		{
			TicketID:   "TCK-1105",
			Title:      "การเชื่อมต่อผิดปกติไปยัง IP ต้องสงสัยในรายการเฝ้าระวัง",
			TargetIP:   "1.1.1.1",
			Status:     "Closed",
			Severity:   "Low",
			Assignee:   "SOC Tier 1",
			Resolution: "ตรวจสอบแล้วเป็น False Positive จากระบบ DNS",
			CreatedAt:  "2026-11-21 08:05:33",
		},
	}
)

func executeMockupTicketSearch(ctx context.Context, tool *ToolConfig, req ExecutionRequest, vars map[string]string) (map[string]any, string, error) {
	if tool != nil && strings.TrimSpace(substituteTemplateVars(tool.Endpoint, vars)) != "" {
		out, err := executeHTTPConfigTool(ctx, tool, req, vars)
		return out, "real", err
	}
	return searchInternalMockupTickets(req, vars), "mockup", nil
}

func searchInternalMockupTickets(req ExecutionRequest, vars map[string]string) map[string]any {
	mockTicketMu.RLock()
	snapshot := make([]MockTicketRecord, len(mockTicketRecords))
	copy(snapshot, mockTicketRecords)
	mockTicketMu.RUnlock()

	searchIP := strings.ToLower(strings.TrimSpace(vars["ip"]))
	searchQuery := strings.ToLower(strings.TrimSpace(vars["query"]))

	matched := []MockTicketRecord{}
	for _, item := range snapshot {
		if searchIP != "" && strings.EqualFold(item.TargetIP, searchIP) {
			matched = append(matched, item)
			continue
		}
		if searchQuery != "" {
			haystack := strings.ToLower(item.TicketID + " " + item.Title + " " + item.TargetIP + " " + item.Status + " " + item.Resolution)
			if strings.Contains(haystack, searchQuery) {
				matched = append(matched, item)
				continue
			}
		}
	}

	filterUsed := "all_recent_cases"
	switch {
	case searchIP != "" && searchQuery != "" && searchIP != searchQuery:
		filterUsed = "ip:" + searchIP + " | query:" + searchQuery
	case searchIP != "":
		filterUsed = "ip:" + searchIP
	case searchQuery != "":
		filterUsed = "query:" + searchQuery
	default:
		matched = snapshot
	}

	matchedIDs := make([]string, 0, len(matched))
	for _, m := range matched {
		matchedIDs = append(matchedIDs, m.TicketID)
	}

	return map[string]any{
		"tool":            "Ticket Search System (Internal Mockup)",
		"step":            req.Label,
		"filter":          filterUsed,
		"matched_count":   len(matched),
		"matched_cases":   matchedIDs,
		"matched_tickets": matched,
	}
}

func executeMockupTicketCreate(ctx context.Context, tool *ToolConfig, req ExecutionRequest, vars map[string]string) (map[string]any, string, error) {
	if tool != nil && strings.TrimSpace(substituteTemplateVars(tool.Endpoint, vars)) != "" {
		out, err := executeHTTPConfigTool(ctx, tool, req, vars)
		return out, "real", err
	}
	return createInternalMockupTicket(req, vars), "mockup", nil
}

func createInternalMockupTicket(req ExecutionRequest, vars map[string]string) map[string]any {
	mockTicketMu.Lock()
	mockTicketCounter++
	newID := fmt.Sprintf("TCK-%d", mockTicketCounter)
	targetIP := strings.TrimSpace(vars["ip"])
	title := strings.TrimSpace(req.Label)
	if title == "" {
		title = "SOC Playbook Automated Ticket"
	}
	assignee := "SOC Tier 1"
	if strings.Contains(strings.ToLower(title), "tier 2") {
		assignee = "SOC Tier 2"
	}
	record := MockTicketRecord{
		TicketID:  newID,
		Title:     title,
		TargetIP:  targetIP,
		Status:    "Open",
		Severity:  "High",
		Assignee:  assignee,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	mockTicketRecords = append([]MockTicketRecord{record}, mockTicketRecords...)
	mockTicketMu.Unlock()

	return map[string]any{
		"tool":      "Ticket Management System (Internal Mockup)",
		"step":      req.Label,
		"ticket_id": record.TicketID,
		"status":    record.Status,
		"assignee":  record.Assignee,
		"target_ip": record.TargetIP,
		"ticket":    record,
	}
}

// ============================================================================
// RESPONSE MAPPERS & REGISTRY LOOKUP HELPERS
// ============================================================================

func mapVirusTotalResponse(statusCode int, rawBody []byte) (map[string]any, error) {
	var parsed struct {
		Data struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			Attributes struct {
				Country           string         `json:"country"`
				ASOwner           string         `json:"as_owner"`
				LastAnalysisStats map[string]int `json:"last_analysis_stats"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &parsed); err != nil || parsed.Data.ID == "" {
		return nil, err
	}
	stats := parsed.Data.Attributes.LastAnalysisStats
	malicious := stats["malicious"]
	suspicious := stats["suspicious"]
	verdict := "Clean / No Detections"
	if malicious > 0 {
		verdict = "Malicious Indicator Confirmed"
	} else if suspicious > 0 {
		verdict = "Suspicious Indicator"
	}
	return map[string]any{
		"tool":             "VirusTotal API",
		"http_status":      statusCode,
		"indicator_type":   parsed.Data.Type,
		"target":           parsed.Data.ID,
		"country":          parsed.Data.Attributes.Country,
		"as_owner":         parsed.Data.Attributes.ASOwner,
		"malicious_count":  malicious,
		"suspicious_count": suspicious,
		"harmless_count":   stats["harmless"],
		"verdict":          verdict,
	}, nil
}

func mapAbuseIPDBResponse(statusCode int, rawBody []byte) (map[string]any, error) {
	var parsed struct {
		Data struct {
			IPAddress            string `json:"ipAddress"`
			AbuseConfidenceScore int    `json:"abuseConfidenceScore"`
			TotalReports         int    `json:"totalReports"`
			CountryCode          string `json:"countryCode"`
			ISP                  string `json:"isp"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &parsed); err != nil || parsed.Data.IPAddress == "" {
		return nil, err
	}
	verdict := "Low Risk"
	if parsed.Data.AbuseConfidenceScore >= 75 {
		verdict = "High Risk / Malicious Source"
	} else if parsed.Data.AbuseConfidenceScore >= 25 {
		verdict = "Suspicious Source"
	}
	return map[string]any{
		"tool":             "AbuseIPDB API",
		"http_status":      statusCode,
		"ip":               parsed.Data.IPAddress,
		"confidence_score": parsed.Data.AbuseConfidenceScore,
		"total_reports":    parsed.Data.TotalReports,
		"country":          parsed.Data.CountryCode,
		"isp":              parsed.Data.ISP,
		"verdict":          verdict,
	}, nil
}

func mapShodanResponse(statusCode int, rawBody []byte) (map[string]any, error) {
	var hostResp struct {
		IPStr       string   `json:"ip_str"`
		Org         string   `json:"org"`
		OS          string   `json:"os"`
		CountryName string   `json:"country_name"`
		Ports       []int    `json:"ports"`
		Vulns       []string `json:"vulns"`
	}
	if err := json.Unmarshal(rawBody, &hostResp); err == nil && hostResp.IPStr != "" {
		return map[string]any{
			"tool":        "Shodan Host API",
			"http_status": statusCode,
			"ip":          hostResp.IPStr,
			"org":         hostResp.Org,
			"os":          hostResp.OS,
			"country":     hostResp.CountryName,
			"open_ports":  hostResp.Ports,
			"vulns":       hostResp.Vulns,
		}, nil
	}

	var searchResp struct {
		Total   int `json:"total"`
		Matches []struct {
			IPStr string `json:"ip_str"`
			Port  int    `json:"port"`
			Org   string `json:"org"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(rawBody, &searchResp); err == nil && (searchResp.Total > 0 || len(searchResp.Matches) > 0) {
		limit := len(searchResp.Matches)
		if limit > 5 {
			limit = 5
		}
		topMatches := make([]map[string]any, 0, limit)
		for _, m := range searchResp.Matches[:limit] {
			topMatches = append(topMatches, map[string]any{
				"ip":   m.IPStr,
				"port": m.Port,
				"org":  m.Org,
			})
		}
		return map[string]any{
			"tool":          "Shodan Search API",
			"http_status":   statusCode,
			"total_matches": searchResp.Total,
			"top_results":   topMatches,
		}, nil
	}

	return nil, fmt.Errorf("unrecognized shodan payload")
}

func GetToolConfigByID(id string) (*ToolConfig, bool) {
	canonical := NormalizeToolID(id)
	for i := range ConfiguredTools {
		if strings.EqualFold(ConfiguredTools[i].ID, canonical) {
			return &ConfiguredTools[i], true
		}
	}
	return nil, false
}

func MissingToolEnvVars(tool *ToolConfig) []string {
	if tool == nil {
		return nil
	}
	var missing []string
	for _, envKey := range tool.RequiredEnv {
		if strings.TrimSpace(os.Getenv(envKey)) == "" {
			missing = append(missing, envKey)
		}
	}
	return missing
}

func MissingToolCredentialEnvs(tool *ToolConfig) []string {
	if tool == nil {
		return nil
	}
	missing := MissingToolEnvVars(tool)
	for _, f := range tool.InputFields {
		if f.EnvFallback != "" && strings.TrimSpace(os.Getenv(f.EnvFallback)) == "" {
			missing = append(missing, f.EnvFallback)
		}
	}
	return missing
}

func ListToolStatuses() []ToolStatusSummary {
	out := make([]ToolStatusSummary, 0, len(ConfiguredTools))
	for i := range ConfiguredTools {
		t := &ConfiguredTools[i]
		missing := MissingToolCredentialEnvs(t)
		mode := "sandbox"
		readyReal := false
		if t.Enabled {
			resolvedEndpoint := strings.TrimSpace(substituteTemplateVars(t.Endpoint, nil))
			hasHTTPEndpoint := strings.HasPrefix(resolvedEndpoint, "http://") || strings.HasPrefix(resolvedEndpoint, "https://")
			if t.CustomExecute != nil && len(t.RequiredEnv) == 0 && len(t.RequiredInputs) == 0 {
				if hasHTTPEndpoint {
					mode = "real"
				} else {
					mode = "mockup"
				}
				readyReal = true
			} else if len(missing) == 0 && (t.Endpoint != "" || t.CustomExecute != nil) {
				mode = "real"
				readyReal = true
			}
		}
		out = append(out, ToolStatusSummary{
			ID:          t.ID,
			Name:        t.Name,
			Category:    t.Category,
			Enabled:     t.Enabled,
			ReadyReal:   readyReal,
			Mode:        mode,
			MissingEnv:  missing,
			InputFields: t.InputFields,
		})
	}
	return out
}
