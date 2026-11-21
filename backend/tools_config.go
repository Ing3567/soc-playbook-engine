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
//   2. ระบบจะนำ Keywords ไปจับคู่กับ Node ใน Flowchart อัตโนมัติ (ไม่ต้องแก้ flow_builder.go)
//   3. เวลาสั่ง Run Node ระบบจะยิง HTTP จริงตาม Endpoint/Headers/BodyTemplate หรือรัน CustomExecute
//   4. หากยังไม่ได้ตั้งค่า RequiredEnv ในไฟล์ .env และเปิด FallbackToSandbox: true
//      ระบบจะสลับไปรันโหมดจำลอง (Sandbox) อัตโนมัติโดยไม่ทำให้ระบบพัง
//
// ตัวแปร Placeholder ที่ใช้ใน Endpoint, Headers, QueryParams, BodyTemplate ได้:
//   - {{ip}}       : IP Address ที่ตรวจพบจากข้อความ Node หรือจากช่อง Run Inputs
//   - {{query}}    : คำค้นหา หรือ Splunk Query ที่ติดมากับลิงก์ใน Node / Run Inputs
//   - {{hash}}     : ค่า File Hash (MD5 / SHA256)
//   - {{domain}}   : ชื่อโดเมน
//   - {{url}}      : ลิงก์ URL ที่แนบอยู่ใน Node
//   - {{label}}    : ข้อความของขั้นตอน (Node Label)
//   - {{env.NAME}} : ดึงค่าตัวแปรจาก Environment / ไฟล์ .env เช่น {{env.VT_API_KEY}}
// ============================================================================

type ToolConfig struct {
	ID                 string
	Name               string
	Category           string // "lookup" หรือ "action"
	Keywords           []string
	Enabled            bool
	FallbackToSandbox  bool
	RequiredEnv        []string
	RequiredInputs     []string
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
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Enabled     bool     `json:"enabled"`
	ReadyReal   bool     `json:"ready_real"`
	Mode        string   `json:"mode"`
	MissingEnv  []string `json:"missing_env,omitempty"`
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
	// 1. VirusTotal (Real HTTP API เมื่อตั้งค่า VT_API_KEY ใน .env)
	{
		ID:                "virustotal",
		Name:              "VirusTotal Threat Lookup",
		Category:          "lookup",
		Keywords:          []string{"virustotal", "vt"},
		Enabled:           true,
		FallbackToSandbox: true,
		RequiredEnv:       []string{"VT_API_KEY"},
		RequiredInputs:    []string{"ip"},
		Method:            "GET",
		Endpoint:          "https://www.virustotal.com/api/v3/ip_addresses/{{ip}}",
		Headers: map[string]string{
			"x-apikey": "{{env.VT_API_KEY}}",
			"Accept":   "application/json",
		},
		TimeoutSec:     15,
		ResponseMapper: mapVirusTotalResponse,
	},

	// 2. AbuseIPDB (Real HTTP API เมื่อตั้งค่า ABUSEIPDB_API_KEY ใน .env)
	{
		ID:                "abuseipdb",
		Name:              "AbuseIPDB Reputation Check",
		Category:          "lookup",
		Keywords:          []string{"abuseipdb", "reputation"},
		Enabled:           true,
		FallbackToSandbox: true,
		RequiredEnv:       []string{"ABUSEIPDB_API_KEY"},
		RequiredInputs:    []string{"ip"},
		Method:            "GET",
		Endpoint:          "https://api.abuseipdb.com/api/v2/check",
		QueryParams: map[string]string{
			"ipAddress":    "{{ip}}",
			"maxAgeInDays": "90",
		},
		Headers: map[string]string{
			"Key":    "{{env.ABUSEIPDB_API_KEY}}",
			"Accept": "application/json",
		},
		TimeoutSec:     15,
		ResponseMapper: mapAbuseIPDBResponse,
	},

	// 3. Splunk SIEM (Real HTTP API เมื่อตั้งค่า SPLUNK_URL และ SPLUNK_TOKEN ใน .env)
	// ดึง {{query}} อัตโนมัติจากลิงก์ Splunk ที่ฝังอยู่ใน Node หรือจากช่อง Run Inputs
	{
		ID:                "splunk",
		Name:              "Splunk SIEM Search",
		Category:          "lookup",
		Keywords:          []string{"splunk", "siem"},
		Enabled:           true,
		FallbackToSandbox: true,
		RequiredEnv:       []string{"SPLUNK_URL", "SPLUNK_TOKEN"},
		RequiredInputs:    []string{"query"},
		Method:            "POST",
		Endpoint:          "{{env.SPLUNK_URL}}/services/search/jobs/export",
		QueryParams: map[string]string{
			"output_mode": "json",
			"search":      "{{query}}",
		},
		Headers: map[string]string{
			"Authorization": "Bearer {{env.SPLUNK_TOKEN}}",
			"Accept":        "application/json",
		},
		TimeoutSec:         20,
		InsecureSkipVerify: true,
	},

	// 4. Ticket Search (ระบบค้นหาทิกเก็ต/เคสเก่าที่เรา Mockup ขึ้นมาเองภายใน)
	// สามารถสลับไปยิง API จริงได้ทันทีหากตั้งค่า TICKET_SEARCH_API_URL ใน .env
	{
		ID:                "ticket_search",
		Name:              "Ticket Search System",
		Category:          "lookup",
		Keywords:          []string{"เคสเก่า", "ประวัติเคส", "ค้นหาเคส", "ticket search", "case search", "ค้นหา ticket", "ประวัติ"},
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

	// 5. Firewall Block (Real HTTP Webhook/API เมื่อตั้งค่า FIREWALL_API_URL และ FIREWALL_API_KEY)
	{
		ID:                "firewall_block",
		Name:              "Firewall Block Controller",
		Category:          "action",
		Keywords:          []string{"บล็อก", "ระงับ", "block", "firewall"},
		Enabled:           true,
		FallbackToSandbox: true,
		RequiredEnv:       []string{"FIREWALL_API_URL", "FIREWALL_API_KEY"},
		RequiredInputs:    []string{"ip"},
		Method:            "POST",
		Endpoint:          "{{env.FIREWALL_API_URL}}",
		Headers: map[string]string{
			"Authorization": "Bearer {{env.FIREWALL_API_KEY}}",
			"Content-Type":  "application/json",
		},
		BodyTemplate: map[string]any{
			"ip":     "{{ip}}",
			"action": "deny",
			"reason": "{{label}}",
		},
		TimeoutSec: 15,
	},

	// 6. Ticket System (ระบบเปิดเคส/บันทึกทิกเก็ตที่เรา Mockup ขึ้นมาเองภายใน)
	// เมื่อสร้างทิกเก็ตใหม่ ข้อมูลจะถูกเก็บเข้าฐานข้อมูลจำลองเพื่อให้ ticket_search ค้นเจอได้ทันที
	{
		ID:                "ticket",
		Name:              "Ticket Management System",
		Category:          "action",
		Keywords:          []string{"ticket", "เปิดเคส", "บันทึกเคส", "เคส", "tier"},
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
			ID:                "slack_notify",
			Name:              "Slack SOC Alert Webhook",
			Category:          "action",
			Keywords:          []string{"slack", "แจ้งเตือนแชท", "notify"},
			Enabled:           true,
			FallbackToSandbox: true,
			RequiredEnv:       []string{"SLACK_WEBHOOK_URL"},
			Method:            "POST",
			Endpoint:          "{{env.SLACK_WEBHOOK_URL}}",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			BodyTemplate: map[string]any{
				"text": "🚨 [SOC Playbook] {{label}} (Target IP: {{ip}})",
			},
			TimeoutSec: 10,
		},
	*/
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
	case searchIP != "" && searchQuery != "":
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

func ListToolStatuses() []ToolStatusSummary {
	out := make([]ToolStatusSummary, 0, len(ConfiguredTools))
	for i := range ConfiguredTools {
		t := &ConfiguredTools[i]
		missing := MissingToolEnvVars(t)
		mode := "sandbox"
		readyReal := false
		if t.Enabled {
			resolvedEndpoint := strings.TrimSpace(substituteTemplateVars(t.Endpoint, nil))
			hasHTTPEndpoint := strings.HasPrefix(resolvedEndpoint, "http://") || strings.HasPrefix(resolvedEndpoint, "https://")
			if t.CustomExecute != nil && len(t.RequiredEnv) == 0 {
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
			ID:         t.ID,
			Name:       t.Name,
			Category:   t.Category,
			Enabled:    t.Enabled,
			ReadyReal:  readyReal,
			Mode:       mode,
			MissingEnv: missing,
		})
	}
	return out
}
