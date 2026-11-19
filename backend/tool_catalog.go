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
	validHTTPMethods    = map[string]bool{"GET": true, "POST": true, "PUT": true, "DELETE": true}
	validPayloadModes   = map[string]bool{"fixed": true, "dynamic": true}
	validParamLocations = map[string]bool{"path": true, "query": true, "body": true, "header": true}
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
			HTTPMethod:  "GET",
			Endpoint:    "https://www.virustotal.com/api/v3/ip_addresses/{{ip}}",
			PayloadMode: "dynamic",
			Params: []ToolParameter{
				{Key: "ip", Label: "Target IP", In: "path", Mode: "dynamic", Value: "{{alert.src_ip}}", Required: true},
			},
			IsBuiltin: true,
			Enabled:   true,
		},
		{
			ID:          "abuseipdb",
			Name:        "AbuseIPDB",
			Category:    "lookup",
			Description: "Query IP address abuse confidence score and threat reports on AbuseIPDB",
			Keywords:    []string{"abuseipdb"},
			CommandType: "http-api",
			HTTPMethod:  "GET",
			Endpoint:    "https://api.abuseipdb.com/api/v2/check",
			PayloadMode: "dynamic",
			Params: []ToolParameter{
				{Key: "ipAddress", Label: "IP Address", In: "query", Mode: "dynamic", Value: "{{alert.src_ip}}", Required: true},
				{Key: "maxAgeInDays", Label: "Max Age (Days)", In: "query", Mode: "fixed", Value: "90", DefaultValue: "90"},
			},
			IsBuiltin: true,
			Enabled:   true,
		},
		{
			ID:          "jira_search",
			Name:        "Jira Case Search",
			Category:    "lookup",
			Description: "Search historical incidents, past cases, or existing tickets in Jira",
			Keywords:    []string{"jira", "เคสเก่า"},
			CommandType: "http-api",
			HTTPMethod:  "GET",
			Endpoint:    "/rest/api/3/search",
			PayloadMode: "dynamic",
			Params: []ToolParameter{
				{Key: "jql", Label: "JQL Filter", In: "query", Mode: "dynamic", Value: `text ~ "{{alert.src_ip}}"`, Required: true},
				{Key: "maxResults", Label: "Max Results", In: "query", Mode: "fixed", Value: "10", DefaultValue: "10"},
			},
			IsBuiltin: true,
			Enabled:   true,
		},
		{
			ID:          "firewall_block",
			Name:        "Firewall Block",
			Category:    "action",
			Description: "Block malicious IP address, domain, or suspend network access on the Firewall",
			Keywords:    []string{"บล็อก", "ระงับ", "block", "firewall"},
			CommandType: "http-api",
			HTTPMethod:  "POST",
			Endpoint:    "/api/firewall/block",
			PayloadMode: "dynamic",
			Params: []ToolParameter{
				{Key: "ip", Label: "Block IP", In: "body", Mode: "dynamic", Value: "{{alert.src_ip}}", Required: true},
				{Key: "action", Label: "Action", In: "body", Mode: "fixed", Value: "deny"},
			},
			IsBuiltin: true,
			Enabled:   true,
		},
		{
			ID:          "jira_ticket",
			Name:        "Jira Ticket",
			Category:    "action",
			Description: "Open or update an incident ticket, escalate to Tier 2, or record results in ticketing system",
			Keywords:    []string{"ticket", "tier", "เปิดเคส", "เคส"},
			CommandType: "http-api",
			HTTPMethod:  "POST",
			Endpoint:    "/rest/api/3/issue",
			PayloadMode: "dynamic",
			Params: []ToolParameter{
				{Key: "summary", Label: "Ticket Summary", In: "body", Mode: "dynamic", Value: "SOC Alert for {{alert.src_ip}}", Required: true},
				{Key: "priority", Label: "Priority", In: "body", Mode: "fixed", Value: "High"},
			},
			IsBuiltin: true,
			Enabled:   true,
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

	tool.HTTPMethod = strings.ToUpper(strings.TrimSpace(tool.HTTPMethod))
	if tool.HTTPMethod == "" {
		if tool.Category == "lookup" {
			tool.HTTPMethod = "GET"
		} else {
			tool.HTTPMethod = "POST"
		}
	}
	if !validHTTPMethods[tool.HTTPMethod] {
		return tool, fmt.Errorf("%w: http_method must be GET, POST, PUT, or DELETE", ErrInvalidTool)
	}

	tool.PayloadMode = strings.ToLower(strings.TrimSpace(tool.PayloadMode))
	if tool.PayloadMode == "" {
		tool.PayloadMode = "dynamic"
	}
	if !validPayloadModes[tool.PayloadMode] {
		return tool, fmt.Errorf("%w: payload_mode must be fixed or dynamic", ErrInvalidTool)
	}

	tool.Description = strings.TrimSpace(tool.Description)
	if tool.Description == "" {
		if tool.Category == "lookup" {
			tool.Description = fmt.Sprintf("Lookup and query security data using %s", tool.Name)
		} else {
			tool.Description = fmt.Sprintf("Execute security response action using %s", tool.Name)
		}
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

	cleanedParams := make([]ToolParameter, 0, len(tool.Params))
	seenParams := make(map[string]bool)
	for _, p := range tool.Params {
		p.Key = strings.TrimSpace(p.Key)
		if p.Key == "" {
			return tool, fmt.Errorf("%w: parameter key is required", ErrInvalidTool)
		}
		p.In = strings.ToLower(strings.TrimSpace(p.In))
		if p.In == "" {
			p.In = "query"
		}
		if !validParamLocations[p.In] {
			return tool, fmt.Errorf("%w: parameter 'in' must be path, query, body, or header", ErrInvalidTool)
		}
		p.Mode = strings.ToLower(strings.TrimSpace(p.Mode))
		if p.Mode == "" {
			p.Mode = tool.PayloadMode
		}
		if !validPayloadModes[p.Mode] {
			return tool, fmt.Errorf("%w: parameter 'mode' must be fixed or dynamic", ErrInvalidTool)
		}
		p.Label = strings.TrimSpace(p.Label)
		p.Value = strings.TrimSpace(p.Value)
		p.DefaultValue = strings.TrimSpace(p.DefaultValue)
		dedupKey := p.In + ":" + p.Key
		if !seenParams[dedupKey] {
			seenParams[dedupKey] = true
			cleanedParams = append(cleanedParams, p)
		}
	}
	tool.Params = cleanedParams

	return tool, nil
}

// SanitizePlaybookForSave ensures SOP blueprints persist only fixed parameters and
// template expressions (e.g. {{alert.src_ip}}, {{prev.verdict}}), never raw runtime customer inputs.
func SanitizePlaybookForSave(doc PlaybookDocument, toolMap map[string]ToolDefinition) PlaybookDocument {
	if len(doc.Nodes) == 0 {
		return doc
	}
	sanitizedNodes := make([]Node, len(doc.Nodes))
	for i, n := range doc.Nodes {
		copyNode := n
		toolDef, hasTool := toolMap[n.Tool]

		if len(n.ParamModes) > 0 {
			modes := make(map[string]string, len(n.ParamModes))
			for k, m := range n.ParamModes {
				lm := strings.ToLower(strings.TrimSpace(m))
				if lm == "fixed" || lm == "dynamic" {
					modes[k] = lm
				}
			}
			if len(modes) > 0 {
				copyNode.ParamModes = modes
			} else {
				copyNode.ParamModes = nil
			}
		}

		if len(n.InputPayload) > 0 {
			cleanedPayload := make(map[string]string)
			for k, v := range n.InputPayload {
				mode := ""
				if copyNode.ParamModes != nil {
					mode = copyNode.ParamModes[k]
				}
				if mode == "" && hasTool {
					for _, p := range toolDef.Params {
						if p.Key == k {
							mode = p.Mode
							break
						}
					}
				}
				if mode == "" {
					mode = strings.ToLower(strings.TrimSpace(n.PayloadMode))
				}
				if mode == "" {
					mode = "dynamic"
				}
				isTemplate := strings.Contains(v, "{{") && strings.Contains(v, "}}")
				if mode == "fixed" || isTemplate {
					cleanedPayload[k] = v
				}
			}
			if len(cleanedPayload) > 0 {
				copyNode.InputPayload = cleanedPayload
			} else {
				copyNode.InputPayload = nil
			}
		}

		sanitizedNodes[i] = copyNode
	}
	doc.Nodes = sanitizedNodes
	return doc
}
