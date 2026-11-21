package main

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
)

var placeholderRegex = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

type AuthSummary struct {
	Type          string `json:"type"`
	In            string `json:"in,omitempty"`
	KeyName       string `json:"key_name,omitempty"`
	EnvVar        string `json:"env_var,omitempty"`
	Source        string `json:"source"`
	Configured    bool   `json:"configured"`
	MaskedPreview string `json:"masked_preview,omitempty"`
}

type ResolvedAuth struct {
	Summary     AuthSummary
	headerKey   string
	headerValue string
	queryKey    string
	queryValue  string
}

type ResolvedToolRequest struct {
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	PayloadMode     string            `json:"payload_mode"`
	Params          map[string]string `json:"params,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Body            map[string]any    `json:"body,omitempty"`
	Auth            *AuthSummary      `json:"auth,omitempty"`
	MissingRequired []string          `json:"missing_required,omitempty"`
	rawHeaders      map[string]string
	rawQuery        map[string]string
	rawBody         map[string]any
}

func ResolveToolAuth(tool *ToolDefinition, runtimeAuth string) ResolvedAuth {
	if tool == nil || tool.Auth == nil {
		return ResolvedAuth{Summary: AuthSummary{Type: "none", Source: "none", Configured: false}}
	}
	authType := strings.ToLower(strings.TrimSpace(tool.Auth.Type))
	if authType == "" || authType == "none" {
		return ResolvedAuth{Summary: AuthSummary{Type: "none", Source: "none", Configured: false}}
	}

	in := strings.ToLower(strings.TrimSpace(tool.Auth.In))
	keyName := strings.TrimSpace(tool.Auth.KeyName)
	prefix := tool.Auth.Prefix
	envVar := strings.ToUpper(strings.TrimSpace(tool.Auth.EnvVar))

	switch authType {
	case "api_key":
		if in == "" {
			in = "header"
		}
		if keyName == "" {
			keyName = "X-API-Key"
		}
	case "bearer":
		in = "header"
		if keyName == "" {
			keyName = "Authorization"
		}
		if strings.TrimSpace(prefix) == "" {
			prefix = "Bearer "
		}
	case "basic":
		in = "header"
		if keyName == "" {
			keyName = "Authorization"
		}
		if strings.TrimSpace(prefix) == "" {
			prefix = "Basic "
		}
	}

	rawSecret := ""
	source := "missing"
	if envVar != "" {
		if envVal := strings.TrimSpace(os.Getenv(envVar)); envVal != "" {
			rawSecret = envVal
			source = "env"
		}
	}
	if rawSecret == "" && tool.Auth.AllowRuntime {
		if rt := strings.TrimSpace(runtimeAuth); rt != "" {
			rawSecret = rt
			source = "runtime"
		}
	}

	summary := AuthSummary{
		Type:       authType,
		In:         in,
		KeyName:    keyName,
		EnvVar:     envVar,
		Source:     source,
		Configured: rawSecret != "",
	}
	if in == "query" {
		summary.MaskedPreview = fmt.Sprintf("?%s=***", keyName)
	} else {
		summary.MaskedPreview = fmt.Sprintf("%s: %s***", keyName, prefix)
	}

	if rawSecret == "" {
		return ResolvedAuth{Summary: summary}
	}

	res := ResolvedAuth{Summary: summary}
	switch authType {
	case "api_key":
		if in == "query" {
			res.queryKey = keyName
			res.queryValue = rawSecret
		} else {
			res.headerKey = keyName
			res.headerValue = prefix + rawSecret
		}
	case "bearer":
		val := rawSecret
		if prefix != "" && !strings.HasPrefix(strings.ToLower(val), strings.ToLower(strings.TrimSpace(prefix))+" ") {
			val = prefix + val
		}
		res.headerKey = keyName
		res.headerValue = val
	case "basic":
		val := rawSecret
		if strings.Contains(val, ":") && !strings.HasPrefix(strings.ToLower(val), "basic ") {
			val = base64.StdEncoding.EncodeToString([]byte(val))
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(val), strings.ToLower(strings.TrimSpace(prefix))+" ") {
			val = prefix + val
		}
		res.headerKey = keyName
		res.headerValue = val
	}
	return res
}

func ExtractEndpointPlaceholders(endpoint string) []string {
	matches := placeholderRegex.FindAllStringSubmatch(endpoint, -1)
	seen := make(map[string]bool, len(matches))
	keys := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 && !seen[m[1]] {
			seen[m[1]] = true
			keys = append(keys, m[1])
		}
	}
	return keys
}

func lookupContextPath(ctx map[string]any, path string) (string, bool) {
	if len(ctx) == 0 || path == "" {
		return "", false
	}
	if direct, ok := ctx[path]; ok && direct != nil {
		return fmt.Sprintf("%v", direct), true
	}
	parts := strings.Split(path, ".")
	var current any = ctx
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		val, exists := m[part]
		if !exists || val == nil {
			return "", false
		}
		current = val
	}
	return fmt.Sprintf("%v", current), true
}

func ResolveDynamicValue(expr string, ctx map[string]any) (string, []string) {
	if expr == "" {
		return "", nil
	}
	var missing []string
	seenMissing := make(map[string]bool)
	resolved := placeholderRegex.ReplaceAllStringFunc(expr, func(match string) string {
		sub := placeholderRegex.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		varPath := sub[1]
		if val, ok := lookupContextPath(ctx, varPath); ok && strings.TrimSpace(val) != "" {
			return val
		}
		if !seenMissing[varPath] {
			seenMissing[varPath] = true
			missing = append(missing, varPath)
		}
		return ""
	})
	return resolved, missing
}

func resolveTemplateAny(v any, mode string, ctx map[string]any) any {
	switch typed := v.(type) {
	case string:
		if mode == "fixed" {
			return typed
		}
		out, _ := ResolveDynamicValue(typed, ctx)
		return out
	case map[string]any:
		res := make(map[string]any, len(typed))
		for k, val := range typed {
			res[k] = resolveTemplateAny(val, mode, ctx)
		}
		return res
	case []any:
		res := make([]any, len(typed))
		for i, item := range typed {
			res[i] = resolveTemplateAny(item, mode, ctx)
		}
		return res
	default:
		return v
	}
}

func BuildToolRequest(tool *ToolDefinition, req ExecutionRequest) ResolvedToolRequest {
	method := "GET"
	endpoint := ""
	effectiveMode := "dynamic"

	if tool != nil {
		if tool.HTTPMethod != "" {
			method = strings.ToUpper(tool.HTTPMethod)
		} else if tool.Category == "action" {
			method = "POST"
		}
		endpoint = tool.Endpoint
		if tool.PayloadMode != "" {
			effectiveMode = strings.ToLower(tool.PayloadMode)
		}
	}
	if req.PayloadMode == "fixed" || req.PayloadMode == "dynamic" {
		effectiveMode = req.PayloadMode
	}

	resolvedParams := make(map[string]string)
	headers := make(map[string]string)
	body := make(map[string]any)
	queryPairs := make(map[string]string)
	pathValues := make(map[string]string)
	var rawHeaders map[string]string
	var rawQuery map[string]string
	var rawBody map[string]any
	var missingRequired []string

	if tool != nil {
		if len(tool.PayloadTemplate) > 0 {
			rawBody = make(map[string]any, len(tool.PayloadTemplate))
			for k, v := range tool.PayloadTemplate {
				resolvedVal := resolveTemplateAny(v, effectiveMode, req.RuntimeContext)
				body[k] = resolvedVal
				rawBody[k] = resolvedVal
			}
		}

		for _, p := range tool.Params {
			rawVal := p.Value
			if override, ok := req.InputPayload[p.Key]; ok {
				rawVal = override
			}

			paramMode := effectiveMode
			if m, ok := req.ParamModes[p.Key]; ok && (m == "fixed" || m == "dynamic") {
				paramMode = m
			} else if req.PayloadMode == "" && (p.Mode == "fixed" || p.Mode == "dynamic") {
				paramMode = p.Mode
			}

			val := rawVal
			var unresolved []string
			if paramMode == "dynamic" {
				if rtVal, hasRT := req.RuntimeInputs[p.Key]; hasRT && strings.TrimSpace(rtVal) != "" {
					val = strings.TrimSpace(rtVal)
				} else {
					val, unresolved = ResolveDynamicValue(rawVal, req.RuntimeContext)
					if strings.TrimSpace(val) == "" && p.DefaultValue != "" {
						val = p.DefaultValue
						unresolved = nil
					}
				}
			} else if strings.TrimSpace(val) == "" && p.DefaultValue != "" {
				val = p.DefaultValue
			}

			if p.Required && (strings.TrimSpace(val) == "" || len(unresolved) > 0) {
				missingRequired = append(missingRequired, p.Key)
			}

			displayVal := val
			if p.IsSecret && strings.TrimSpace(val) != "" {
				displayVal = "***"
			}

			resolvedParams[p.Key] = displayVal
			switch p.In {
			case "path":
				pathValues[p.Key] = val
			case "query":
				if val != "" {
					queryPairs[p.Key] = displayVal
					if rawQuery == nil {
						rawQuery = make(map[string]string)
					}
					rawQuery[p.Key] = val
				}
			case "header":
				if val != "" {
					headers[p.Key] = displayVal
					if rawHeaders == nil {
						rawHeaders = make(map[string]string)
					}
					rawHeaders[p.Key] = val
				}
			case "body":
				body[p.Key] = displayVal
				if rawBody == nil {
					rawBody = make(map[string]any)
				}
				rawBody[p.Key] = val
			}
		}
	}

	// Include any ad-hoc runtime_inputs or input_payload keys not explicitly listed in tool.Params.
	for k, v := range req.RuntimeInputs {
		if _, exists := resolvedParams[k]; exists || strings.TrimSpace(v) == "" {
			continue
		}
		val := strings.TrimSpace(v)
		resolvedParams[k] = val
		if strings.Contains(endpoint, "{{"+k+"}}") {
			pathValues[k] = val
		} else if method == "GET" || method == "DELETE" {
			queryPairs[k] = val
			if rawQuery == nil {
				rawQuery = make(map[string]string)
			}
			rawQuery[k] = val
		} else {
			body[k] = val
			if rawBody == nil {
				rawBody = make(map[string]any)
			}
			rawBody[k] = val
		}
	}

	for k, v := range req.InputPayload {
		if _, exists := resolvedParams[k]; exists {
			continue
		}
		val := v
		if effectiveMode == "dynamic" {
			val, _ = ResolveDynamicValue(v, req.RuntimeContext)
		}
		resolvedParams[k] = val
		if strings.Contains(endpoint, "{{"+k+"}}") {
			pathValues[k] = val
		} else if method == "GET" || method == "DELETE" {
			queryPairs[k] = val
			if rawQuery == nil {
				rawQuery = make(map[string]string)
			}
			rawQuery[k] = val
		} else {
			body[k] = val
			if rawBody == nil {
				rawBody = make(map[string]any)
			}
			rawBody[k] = val
		}
	}

	resolvedURL := endpoint
	for k, v := range pathValues {
		escaped := url.PathEscape(v)
		resolvedURL = strings.ReplaceAll(resolvedURL, "{{"+k+"}}", escaped)
	}

	if resolvedURL != "" && len(queryPairs) > 0 {
		if parsed, err := url.Parse(resolvedURL); err == nil {
			q := parsed.Query()
			for k, v := range queryPairs {
				q.Set(k, v)
			}
			parsed.RawQuery = q.Encode()
			resolvedURL = parsed.String()
		}
	}

	var authSummary *AuthSummary
	resolvedAuth := ResolveToolAuth(tool, req.RuntimeAuth)
	if resolvedAuth.Summary.Type != "none" {
		s := resolvedAuth.Summary
		authSummary = &s
	}

	return ResolvedToolRequest{
		Method:          method,
		URL:             resolvedURL,
		PayloadMode:     effectiveMode,
		Params:          resolvedParams,
		Headers:         headers,
		Body:            body,
		Auth:            authSummary,
		MissingRequired: missingRequired,
		rawHeaders:      rawHeaders,
		rawQuery:        rawQuery,
		rawBody:         rawBody,
	}
}
