package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidScenario          = errors.New("invalid scenario definition")
	ErrScenarioNotFound         = errors.New("scenario not found")
	ErrScenarioAlreadyExists    = errors.New("scenario id already exists")
	ErrBuiltinScenarioProtected = errors.New("builtin scenario cannot be deleted")

	validScenarioDifficulties = map[string]bool{"easy": true, "medium": true, "hard": true}
	validScenarioVerdicts     = map[string]bool{"clean": true, "suspicious": true, "malicious": true}
)

func DefaultBuiltinScenarios() []Scenario {
	now := time.Now()
	return []Scenario{
		{
			ID:          "ssh_brute_force_tp",
			Title:       "SSH Brute Force from External Botnet (True Positive)",
			Description: "ตรวจพบการพยายามล็อกอิน SSH ล้มเหลวกว่า 400 ครั้งจาก IP ภายนอกภายใน 3 นาที — ควรตรวจสอบ reputation แล้วบล็อก IP พร้อมเปิดเคสแจ้ง Tier 2",
			Difficulty:  "easy",
			Alert: map[string]any{
				"alert_name": "Multiple Failed SSH Logins",
				"severity":   "high",
				"src_ip":     "185.220.101.42",
				"dst_ip":     "10.20.1.15",
				"hostname":   "prod-web-01",
				"username":   "root",
			},
			ToolMocks: map[string]map[string]map[string]any{
				"virustotal": {
					"185.220.101.42": {
						"tool":            "VirusTotal (Scenario)",
						"malicious_count": 14,
						"verdict":         "Malicious IP Confirmed (SSH Brute-Force / Tor Exit)",
						"country":         "DE",
						"asn":             "AS205100",
					},
					"default": {
						"tool":            "VirusTotal (Scenario)",
						"malicious_count": 14,
						"verdict":         "Malicious IP Confirmed",
					},
				},
				"abuseipdb": {
					"185.220.101.42": {
						"tool":             "AbuseIPDB (Scenario)",
						"confidence_score": 94,
						"total_reports":    312,
						"verdict":          "High Confidence Malicious",
					},
					"default": {
						"tool":             "AbuseIPDB (Scenario)",
						"confidence_score": 94,
						"verdict":          "High Confidence Malicious",
					},
				},
				"jira_search": {
					"default": {
						"tool":          "Jira Case Lookup (Scenario)",
						"matched_cases": []string{"SOC-2041"},
					},
				},
				"firewall_block": {
					"default": {
						"tool":       "Firewall Controller (Scenario)",
						"rule_id":    "FW-SIM-8842",
						"blocked_ip": "185.220.101.42",
						"result":     "Blocked Successfully",
					},
				},
				"ticket": {
					"default": {
						"tool":      "Ticket (Scenario)",
						"ticket_id": "SOC-3012",
						"priority":  "High",
						"status":    "Created",
					},
				},
			},
			GroundTruth: &ScenarioGroundTruth{
				ExpectedVerdict: "malicious",
				MaxDurationSec:  300,
			},
			IsBuiltin: true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          "internal_scanner_fp",
			Title:       "Authorized Vulnerability Scanner Alert (False Positive)",
			Description: "แจ้งเตือน Port Scan บนเครื่อง Staging แต่ IP ต้นทางเป็นเครื่องสแกนช่องโหว่ที่ได้รับอนุญาตตามรอบบำรุงรักษา — ตรวจสอบแล้วควรปิดเคสเป็น False Positive โดยไม่บล็อก",
			Difficulty:  "easy",
			Alert: map[string]any{
				"alert_name": "Port Scan Detected",
				"severity":   "medium",
				"src_ip":     "198.51.100.25",
				"dst_ip":     "10.20.4.50",
				"hostname":   "app-staging-02",
				"username":   "svc-scanner",
			},
			ToolMocks: map[string]map[string]map[string]any{
				"virustotal": {
					"198.51.100.25": {
						"tool":            "VirusTotal (Scenario)",
						"malicious_count": 0,
						"verdict":         "Clean / No Detections",
						"org":             "Authorized Security Scanner",
					},
					"default": {
						"tool":            "VirusTotal (Scenario)",
						"malicious_count": 0,
						"verdict":         "Clean / No Detections",
					},
				},
				"abuseipdb": {
					"198.51.100.25": {
						"tool":             "AbuseIPDB (Scenario)",
						"confidence_score": 0,
						"total_reports":    0,
						"verdict":          "Clean / Whitelisted",
					},
					"default": {
						"tool":             "AbuseIPDB (Scenario)",
						"confidence_score": 0,
						"verdict":          "Clean / Whitelisted",
					},
				},
				"jira_search": {
					"default": {
						"tool":          "Jira Case Lookup (Scenario)",
						"matched_cases": []string{},
					},
				},
			},
			GroundTruth: &ScenarioGroundTruth{
				ExpectedVerdict: "clean",
				MaxDurationSec:  240,
			},
			IsBuiltin: true,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          "phishing_url_manual",
			Title:       "Suspicious Phishing Portal Click (Medium)",
			Description: "พนักงานคลิกลิงก์ปลอมที่เลียนแบบหน้าล็อกอิน Microsoft 365 — ตรวจสอบ IP/Domain แล้วเปิดเคสแจ้งทีม Incident Response",
			Difficulty:  "medium",
			Alert: map[string]any{
				"alert_name": "Suspicious Outbound Click from Email",
				"severity":   "high",
				"src_ip":     "203.0.113.77",
				"domain":     "login-verify-office365.example",
				"url":        "https://login-verify-office365.example/auth",
				"username":   "somchai.k",
			},
			ToolMocks: map[string]map[string]map[string]any{
				"virustotal": {
					"203.0.113.77": {
						"tool":            "VirusTotal (Scenario)",
						"malicious_count": 6,
						"verdict":         "Phishing / Credential Harvesting",
					},
					"login-verify-office365.example": {
						"tool":            "VirusTotal (Scenario)",
						"malicious_count": 9,
						"verdict":         "Phishing Domain Confirmed",
					},
					"default": {
						"tool":            "VirusTotal (Scenario)",
						"malicious_count": 6,
						"verdict":         "Phishing / Credential Harvesting",
					},
				},
				"abuseipdb": {
					"203.0.113.77": {
						"tool":             "AbuseIPDB (Scenario)",
						"confidence_score": 68,
						"total_reports":    47,
						"verdict":          "Suspicious Phishing Infrastructure",
					},
					"default": {
						"tool":             "AbuseIPDB (Scenario)",
						"confidence_score": 68,
						"verdict":          "Suspicious Hosting",
					},
				},
				"jira_search": {
					"default": {
						"tool":          "Jira Case Lookup (Scenario)",
						"matched_cases": []string{},
					},
				},
				"ticket": {
					"default": {
						"tool":      "Ticket (Scenario)",
						"ticket_id": "SOC-3099",
						"priority":  "High",
						"status":    "Created",
					},
				},
			},
			GroundTruth: &ScenarioGroundTruth{
				ExpectedVerdict: "malicious",
				MaxDurationSec:  360,
			},
			IsBuiltin: true,
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

func FindBuiltinScenario(id string) *Scenario {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	for _, s := range DefaultBuiltinScenarios() {
		if s.ID == id {
			copyS := s
			return &copyS
		}
	}
	return nil
}

func NormalizeAndValidateScenario(s Scenario, isCreate bool) (Scenario, error) {
	s.Title = strings.TrimSpace(s.Title)
	if s.Title == "" {
		return s, fmt.Errorf("%w: title is required", ErrInvalidScenario)
	}
	s.Description = strings.TrimSpace(s.Description)
	s.PlaybookID = strings.TrimSpace(s.PlaybookID)

	if isCreate {
		s.ID = strings.TrimSpace(s.ID)
		if s.ID == "" {
			s.ID = GenerateToolSlug(s.Title)
		} else {
			s.ID = strings.ToLower(s.ID)
		}
		if s.ID == "" || !toolIDRegex.MatchString(s.ID) {
			return s, fmt.Errorf("%w: id must start with a-z/0-9 and contain only a-z, 0-9, _, -", ErrInvalidScenario)
		}
	}

	s.Difficulty = strings.ToLower(strings.TrimSpace(s.Difficulty))
	if s.Difficulty == "" {
		s.Difficulty = "easy"
	}
	if !validScenarioDifficulties[s.Difficulty] {
		return s, fmt.Errorf("%w: difficulty must be easy, medium, or hard", ErrInvalidScenario)
	}

	if s.Alert == nil {
		s.Alert = map[string]any{}
	}

	if len(s.ToolMocks) > 0 {
		normalizedMocks := make(map[string]map[string]map[string]any, len(s.ToolMocks))
		for rawToolID, targets := range s.ToolMocks {
			canonicalTool := NormalizeLegacyToolID(strings.ToLower(strings.TrimSpace(rawToolID)))
			if canonicalTool == "" || len(targets) == 0 {
				continue
			}
			cleanTargets := make(map[string]map[string]any, len(targets))
			for targetKey, mockOut := range targets {
				tk := strings.TrimSpace(targetKey)
				if tk == "" || mockOut == nil {
					continue
				}
				cleanTargets[tk] = cloneMapShallow(mockOut)
			}
			if len(cleanTargets) > 0 {
				normalizedMocks[canonicalTool] = cleanTargets
			}
		}
		s.ToolMocks = normalizedMocks
	}

	if s.GroundTruth != nil {
		gt := *s.GroundTruth
		gt.ExpectedVerdict = strings.ToLower(strings.TrimSpace(gt.ExpectedVerdict))
		if gt.ExpectedVerdict != "" && !validScenarioVerdicts[gt.ExpectedVerdict] {
			return s, fmt.Errorf("%w: ground_truth.expected_verdict must be clean, suspicious, or malicious", ErrInvalidScenario)
		}
		if gt.MaxDurationSec < 0 {
			gt.MaxDurationSec = 0
		}
		s.GroundTruth = &gt
	}

	return s, nil
}

// StripScenarioAnswers returns a copy safe to expose to trainees (removes ground truth and mock definitions).
func StripScenarioAnswers(s Scenario) Scenario {
	s.GroundTruth = nil
	s.ToolMocks = nil
	return s
}

// ResolveScenarioMockOutput looks up a scenario-specific mock response for toolID and candidate targets.
func ResolveScenarioMockOutput(scenario *Scenario, toolID string, targets []string) (map[string]any, string, bool) {
	if scenario == nil || len(scenario.ToolMocks) == 0 {
		return nil, "", false
	}
	canonical := NormalizeLegacyToolID(strings.ToLower(strings.TrimSpace(toolID)))
	toolTable, ok := scenario.ToolMocks[canonical]
	if !ok || len(toolTable) == 0 {
		return nil, "", false
	}

	// Exact or case-insensitive target match first.
	for _, t := range targets {
		trimmed := strings.TrimSpace(t)
		if trimmed == "" {
			continue
		}
		if hit, found := toolTable[trimmed]; found {
			return cloneMapShallow(hit), trimmed, true
		}
		for key, val := range toolTable {
			if strings.EqualFold(key, trimmed) {
				return cloneMapShallow(val), key, true
			}
		}
	}

	// Fallback to "default" key inside this tool's scenario mocks.
	if def, found := toolTable["default"]; found {
		return cloneMapShallow(def), "default", true
	}
	return nil, "", false
}

func cloneMapShallow(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
