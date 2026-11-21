package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	ipv4ExtractRegex   = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	hashExtractRegex   = regexp.MustCompile(`\b[a-fA-F0-9]{32,64}\b`)
	domainExtractRegex = regexp.MustCompile(`\b(?:[a-zA-Z0-9-]+\.)+[a-zA-Z]{2,}\b`)
	templateVarRegex   = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)
)

func ExecuteToolStep(ctx context.Context, req ExecutionRequest) ExecutionResult {
	start := time.Now()
	req.Tool = NormalizeToolID(req.Tool)
	vars := extractExecutionVars(req)

	toolCfg, exists := GetToolConfigByID(req.Tool)
	if !exists {
		res := RunSandboxStep(req)
		res.Mode = "sandbox"
		return res
	}

	if !toolCfg.Enabled {
		if toolCfg.FallbackToSandbox {
			return withSandboxFallback(req, fmt.Sprintf("Tool '%s' is disabled in tools_config.go", toolCfg.ID))
		}
		return buildFailedExecutionResult(req, start, "real", fmt.Sprintf("Tool '%s' is disabled", toolCfg.ID))
	}

	// Custom Go handler (e.g. Internal Mockup Ticket Search / Ticket System or custom integration)
	if toolCfg.CustomExecute != nil && len(toolCfg.RequiredEnv) == 0 {
		out, mode, err := toolCfg.CustomExecute(ctx, toolCfg, req, vars)
		if err != nil {
			if toolCfg.FallbackToSandbox {
				return withSandboxFallback(req, err.Error())
			}
			return buildFailedExecutionResult(req, start, mode, err.Error())
		}
		if mode == "" {
			mode = "real"
		}
		return ExecutionResult{
			NodeID:     req.NodeID,
			Status:     "success",
			Mode:       mode,
			DurationMs: time.Since(start).Milliseconds(),
			Timestamp:  time.Now().Format("15:04:05"),
			Output:     out,
		}
	}

	missingEnv := MissingToolEnvVars(toolCfg)
	if len(missingEnv) > 0 {
		reason := fmt.Sprintf("Missing .env configuration: %s", strings.Join(missingEnv, ", "))
		if toolCfg.FallbackToSandbox {
			return withSandboxFallback(req, reason)
		}
		return buildFailedExecutionResult(req, start, "real", reason)
	}

	var missingInputs []string
	for _, reqKey := range toolCfg.RequiredInputs {
		if strings.TrimSpace(vars[reqKey]) == "" {
			missingInputs = append(missingInputs, reqKey)
		}
	}
	if len(missingInputs) > 0 {
		reason := fmt.Sprintf("Missing required runtime input (%s) — provide in Run Inputs or node text/link", strings.Join(missingInputs, ", "))
		if toolCfg.FallbackToSandbox {
			return withSandboxFallback(req, reason)
		}
		return buildFailedExecutionResult(req, start, "real", reason)
	}

	var (
		output map[string]any
		mode   = "real"
		err    error
	)
	if toolCfg.CustomExecute != nil {
		output, mode, err = toolCfg.CustomExecute(ctx, toolCfg, req, vars)
	} else {
		output, err = executeHTTPConfigTool(ctx, toolCfg, req, vars)
	}

	if err != nil {
		return buildFailedExecutionResult(req, start, mode, err.Error())
	}

	return ExecutionResult{
		NodeID:     req.NodeID,
		Status:     "success",
		Mode:       mode,
		DurationMs: time.Since(start).Milliseconds(),
		Timestamp:  time.Now().Format("15:04:05"),
		Output:     output,
	}
}

func withSandboxFallback(req ExecutionRequest, reason string) ExecutionResult {
	res := RunSandboxStep(req)
	res.Mode = "sandbox_fallback"
	if res.Output == nil {
		res.Output = map[string]any{}
	}
	res.Output["fallback_reason"] = reason
	return res
}

func executeHTTPConfigTool(ctx context.Context, tool *ToolConfig, req ExecutionRequest, vars map[string]string) (map[string]any, error) {
	rawEndpoint := strings.TrimSpace(substituteEndpointVars(tool.Endpoint, vars))
	if rawEndpoint == "" {
		return nil, errors.New("empty tool endpoint URL")
	}

	parsedURL, err := url.Parse(rawEndpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported endpoint scheme %q (only http/https allowed)", parsedURL.Scheme)
	}

	if len(tool.QueryParams) > 0 {
		q := parsedURL.Query()
		for k, v := range tool.QueryParams {
			resolvedVal := strings.TrimSpace(substituteTemplateVars(v, vars))
			if resolvedVal != "" {
				q.Set(k, resolvedVal)
			}
		}
		parsedURL.RawQuery = q.Encode()
	}

	method := strings.ToUpper(strings.TrimSpace(tool.Method))
	if method == "" {
		method = http.MethodGet
	}

	var bodyReader io.Reader
	if len(tool.BodyTemplate) > 0 && method != http.MethodGet {
		resolvedBody := substituteValueTree(tool.BodyTemplate, vars)
		rawBytes, marshalErr := json.Marshal(resolvedBody)
		if marshalErr != nil {
			return nil, marshalErr
		}
		bodyReader = bytes.NewReader(rawBytes)
	}

	timeout := time.Duration(tool.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, method, parsedURL.String(), bodyReader)
	if err != nil {
		return nil, err
	}

	for hk, hv := range tool.Headers {
		resolvedHeader := strings.TrimSpace(substituteTemplateVars(hv, vars))
		if resolvedHeader != "" {
			httpReq.Header.Set(hk, resolvedHeader)
		}
	}

	client := &http.Client{Timeout: timeout}
	if tool.InsecureSkipVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		snippet := truncateRunes(strings.TrimSpace(string(respBytes)), 300)
		return nil, fmt.Errorf("%s returned HTTP %d: %s", tool.Name, resp.StatusCode, snippet)
	}

	if tool.ResponseMapper != nil {
		if mapped, mapErr := tool.ResponseMapper(resp.StatusCode, respBytes); mapErr == nil && len(mapped) > 0 {
			mapped["step"] = req.Label
			return mapped, nil
		}
	}

	var decoded any
	if json.Unmarshal(respBytes, &decoded) == nil {
		return map[string]any{
			"tool":        tool.Name,
			"step":        req.Label,
			"http_status": resp.StatusCode,
			"response":    decoded,
		}, nil
	}

	textOut := truncateRunes(strings.TrimSpace(string(respBytes)), 1000)
	return map[string]any{
		"tool":        tool.Name,
		"step":        req.Label,
		"http_status": resp.StatusCode,
		"response":    textOut,
	}, nil
}

func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

func extractExecutionVars(req ExecutionRequest) map[string]string {
	vars := map[string]string{
		"label":   strings.TrimSpace(req.Label),
		"node_id": strings.TrimSpace(req.NodeID),
	}

	// 1. Auto-detect from embedded node links (e.g. Splunk query or URL containing IP)
	for _, l := range req.Links {
		if vars["url"] == "" && strings.TrimSpace(l.URL) != "" {
			vars["url"] = strings.TrimSpace(l.URL)
		}
		if vars["query"] == "" && strings.TrimSpace(l.SplunkQuery) != "" {
			vars["query"] = strings.TrimSpace(l.SplunkQuery)
		}
		if vars["ip"] == "" {
			if ip := ipv4ExtractRegex.FindString(l.URL); ip != "" {
				vars["ip"] = ip
			}
		}
	}

	// 2. Auto-detect from node label text
	if vars["ip"] == "" {
		if ip := ipv4ExtractRegex.FindString(req.Label); ip != "" {
			vars["ip"] = ip
		}
	}
	if vars["hash"] == "" {
		if h := hashExtractRegex.FindString(req.Label); h != "" {
			vars["hash"] = h
		}
	}
	if vars["domain"] == "" {
		if d := domainExtractRegex.FindString(req.Label); d != "" && ipv4ExtractRegex.FindString(d) == "" {
			vars["domain"] = d
		}
	}

	// 3. Explicit runtime inputs override auto-detected values
	for k, v := range req.Inputs {
		cleanKey := strings.ToLower(strings.TrimSpace(k))
		cleanVal := strings.TrimSpace(v)
		if cleanKey != "" && cleanVal != "" {
			vars[cleanKey] = cleanVal
		}
	}

	return vars
}

func substituteEndpointVars(template string, vars map[string]string) string {
	matches := templateVarRegex.FindAllStringSubmatchIndex(template, -1)
	if len(matches) == 0 {
		return template
	}
	var sb strings.Builder
	lastIdx := 0
	for _, m := range matches {
		sb.WriteString(template[lastIdx:m[0]])
		key := strings.TrimSpace(template[m[2]:m[3]])
		var val string
		if strings.HasPrefix(key, "env.") {
			envName := strings.TrimPrefix(key, "env.")
			val = strings.TrimSpace(os.Getenv(envName))
			if m[1] < len(template) && template[m[1]] == '/' {
				val = strings.TrimRight(val, "/")
			}
		} else if raw, ok := vars[strings.ToLower(key)]; ok {
			val = url.PathEscape(raw)
		}
		sb.WriteString(val)
		lastIdx = m[1]
	}
	sb.WriteString(template[lastIdx:])
	return sb.String()
}

func substituteTemplateVars(template string, vars map[string]string) string {
	return templateVarRegex.ReplaceAllStringFunc(template, func(match string) string {
		sub := templateVarRegex.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		key := strings.TrimSpace(sub[1])
		if strings.HasPrefix(key, "env.") {
			envName := strings.TrimPrefix(key, "env.")
			return strings.TrimSpace(os.Getenv(envName))
		}
		if val, ok := vars[strings.ToLower(key)]; ok {
			return val
		}
		return ""
	})
}

func substituteValueTree(v any, vars map[string]string) any {
	switch typed := v.(type) {
	case string:
		return substituteTemplateVars(typed, vars)
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			out[k] = substituteValueTree(val, vars)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, val := range typed {
			out[i] = substituteValueTree(val, vars)
		}
		return out
	default:
		return v
	}
}

func buildFailedExecutionResult(req ExecutionRequest, start time.Time, mode, errMsg string) ExecutionResult {
	return ExecutionResult{
		NodeID:     req.NodeID,
		Status:     "failed",
		Mode:       mode,
		DurationMs: time.Since(start).Milliseconds(),
		Timestamp:  time.Now().Format("15:04:05"),
		Error:      errMsg,
		Output: map[string]any{
			"tool":  req.Tool,
			"step":  req.Label,
			"error": errMsg,
		},
	}
}

func RunSandboxStep(req ExecutionRequest) ExecutionResult {
	start := time.Now()
	time.Sleep(250 * time.Millisecond)

	canonicalTool := NormalizeToolID(req.Tool)
	var output map[string]any
	switch canonicalTool {
	case "virustotal":
		output = map[string]any{
			"tool":            "VirusTotal API Sandbox",
			"step":            req.Label,
			"malicious_count": 12,
			"verdict":         "Malicious IP Confirmed",
		}
	case "abuseipdb":
		output = map[string]any{
			"tool":             "AbuseIPDB Sandbox",
			"step":             req.Label,
			"confidence_score": 85,
			"verdict":          "Suspicious Source",
		}
	case "splunk":
		query := "search index=main | head 20"
		for _, l := range req.Links {
			if strings.TrimSpace(l.SplunkQuery) != "" {
				query = l.SplunkQuery
				break
			}
		}
		if q := strings.TrimSpace(req.Inputs["query"]); q != "" {
			query = q
		}
		output = map[string]any{
			"tool":          "Splunk SIEM Sandbox",
			"step":          req.Label,
			"query":         query,
			"matched_count": 4,
			"verdict":       "Suspicious Events Found",
		}
	case "ticket_search":
		output = searchInternalMockupTickets(req, extractExecutionVars(req))
	case "firewall_block":
		output = map[string]any{
			"tool":    "Firewall Controller",
			"step":    req.Label,
			"rule_id": fmt.Sprintf("FW-%d", time.Now().Unix()%10000),
			"result":  "Blocked Successfully",
		}
	case "ticket":
		output = createInternalMockupTicket(req, extractExecutionVars(req))
	default:
		output = map[string]any{
			"tool":   "Manual SOC Procedure",
			"step":   req.Label,
			"result": "Completed",
		}
	}

	return ExecutionResult{
		NodeID:     req.NodeID,
		Status:     "success",
		Mode:       "sandbox",
		DurationMs: time.Since(start).Milliseconds(),
		Timestamp:  time.Now().Format("15:04:05"),
		Output:     output,
	}
}