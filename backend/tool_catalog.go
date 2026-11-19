package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrToolNotFound         = errors.New("tool not found")
	ErrToolAlreadyExists    = errors.New("tool already exists")
	ErrBuiltinToolProtected = errors.New("built-in tool cannot be deleted")
	ErrInvalidTool          = errors.New("invalid tool definition")
)

var (
	toolIDRegex         = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	nonSlugCharsRegex   = regexp.MustCompile(`[^a-z0-9_-]+`)
	multiUnderscoreRx   = regexp.MustCompile(`_+`)
	reservedToolIDs     = map[string]bool{"none": true, "manual_lookup": true, "manual_action": true}
	validToolCategories = map[string]bool{"lookup": true, "action": true}
	validCommandTypes   = map[string]bool{"http-api": true, "manual": true, "bash": true, "ssh": true}
)

func DefaultBuiltinTools() []ToolDefinition {
	return []ToolDefinition{
		{
			ID:          "virustotal",
			Name:        "VirusTotal",
			Category:    "lookup",
			Description: "Check IP, domain, URL, or file hash reputation and malicious detections on VirusTotal",
			Keywords:    []string{"virustotal", "vt"},
			CommandType: "http-api",
			Endpoint:    "https://www.virustotal.com/api/v3",
			IsBuiltin:   true,
			Enabled:     true,
		},
		{
			ID:          "abuseipdb",
			Name:        "AbuseIPDB",
			Category:    "lookup",
			Description: "Query IP address abuse confidence score and threat reports on AbuseIPDB",
			Keywords:    []string{"abuseipdb"},
			CommandType: "http-api",
			Endpoint:    "https://api.abuseipdb.com/api/v2/check",
			IsBuiltin:   true,
			Enabled:     true,
		},
		{
			ID:          "jira_search",
			Name:        "Jira Case Search",
			Category:    "lookup",
			Description: "Search historical incidents, past cases, or existing tickets in Jira",
			Keywords:    []string{"jira", "เคสเก่า"},
			CommandType: "http-api",
			Endpoint:    "/rest/api/3/search",
			IsBuiltin:   true,
			Enabled:     true,
		},
		{
			ID:          "firewall_block",
			Name:        "Firewall Block",
			Category:    "action",
			Description: "Block malicious IP address, domain, or suspend network access on the Firewall",
			Keywords:    []string{"บล็อก", "ระงับ", "block", "firewall"},
			CommandType: "http-api",
			Endpoint:    "/api/firewall/block",
			IsBuiltin:   true,
			Enabled:     true,
		},
		{
			ID:          "jira_ticket",
			Name:        "Jira Ticket",
			Category:    "action",
			Description: "Open or update an incident ticket, escalate to Tier 2, or record results in ticketing system",
			Keywords:    []string{"ticket", "tier", "เปิดเคส", "เคส"},
			CommandType: "http-api",
			Endpoint:    "/rest/api/3/issue",
			IsBuiltin:   true,
			Enabled:     true,
		},
	}
}

func GenerateToolSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = nonSlugCharsRegex.ReplaceAllString(s, "_")
	s = multiUnderscoreRx.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_-")
	return s
}

func NormalizeAndValidateTool(tool ToolDefinition, isCreate bool) (ToolDefinition, error) {
	tool.Name = strings.TrimSpace(tool.Name)
	if tool.Name == "" {
		return tool, fmt.Errorf("%w: tool name is required", ErrInvalidTool)
	}

	if isCreate {
		tool.ID = strings.ToLower(strings.TrimSpace(tool.ID))
		if tool.ID == "" {
			tool.ID = GenerateToolSlug(tool.Name)
		}
		if tool.ID == "" {
			tool.ID = fmt.Sprintf("tool_%d", time.Now().UnixNano())
		}
		if !toolIDRegex.MatchString(tool.ID) || reservedToolIDs[tool.ID] {
			return tool, fmt.Errorf("%w: invalid or reserved tool id %q", ErrInvalidTool, tool.ID)
		}
	}

	tool.Category = strings.ToLower(strings.TrimSpace(tool.Category))
	if !validToolCategories[tool.Category] {
		return tool, fmt.Errorf("%w: category must be lookup or action", ErrInvalidTool)
	}

	tool.CommandType = strings.ToLower(strings.TrimSpace(tool.CommandType))
	if tool.CommandType == "" {
		tool.CommandType = "http-api"
	}
	if !validCommandTypes[tool.CommandType] {
		return tool, fmt.Errorf("%w: command_type must be http-api, manual, bash, or ssh", ErrInvalidTool)
	}

	tool.Description = strings.TrimSpace(tool.Description)
	if tool.Description == "" {
		return tool, fmt.Errorf("%w: description is required", ErrInvalidTool)
	}

	cleanedKeywords := make([]string, 0, len(tool.Keywords))
	seen := make(map[string]bool)
	for _, kw := range tool.Keywords {
		trimmed := strings.TrimSpace(kw)
		lower := strings.ToLower(trimmed)
		if trimmed != "" && !seen[lower] {
			seen[lower] = true
			cleanedKeywords = append(cleanedKeywords, trimmed)
		}
	}
	tool.Keywords = cleanedKeywords
	tool.Endpoint = strings.TrimSpace(tool.Endpoint)
	return tool, nil
}
