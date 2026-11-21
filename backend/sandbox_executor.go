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
	ipv4ExactRegex     = regexp.MustCompile(`^(?:\d{1,3}\.){3}\d{1,3}$`)
	hashExtractRegex   = regexp.MustCompile(`\b[a-fA-F0-9]{32,64}\b`)
	hashExactRegex     = regexp.MustCompile(`^[a-fA-F0-9]{32,64}$`)
	domainExtractRegex = regexp.MustCompile(`\b(?:[a-zA-Z0-9-]+\.)+[a-zA-Z]{2,}\b`)
	templateVarRegex   = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)
)

func ExecuteToolStep(ctx context.Context, req ExecutionRequest) ExecutionResult {
	start := time.Now()
	req.Tool = NormalizeToolID(req.Tool)

	toolCfg, exists := GetToolConfigByID(req.Tool)
	if !exists {
		res := RunSandboxStep(req)
		res.Mode = "sandbox"
		return res
	}

	vars, secrets := extractExecutionVarsWithSecrets(req, toolCfg)

	if !toolCfg.Enabled {
		if toolCfg.FallbackToSandbox {
			return withSandboxFallback(req, fmt.Sprintf("Tool '%s' is disabled in tools_config.go", toolCfg.ID))
		}
		return buildFailedExecutionResult(req, start, "real", fmt.Sprintf("Tool '%s' is disabled", toolCfg.ID))
	}

	// Tools with zero required env/inputs (e.g. Internal Mockup Ticket Search / Ticket System)
	if toolCfg.CustomExecute != nil && len(toolCfg.RequiredEnv) == 0 && len(toolCfg.RequiredInputs) == 0 {
		out, mode, err := toolCfg.CustomExecute(ctx, toolCfg, req, vars)
		if err != nil {
			safeErr := redactSecrets(err.Error(), secrets)
			if toolCfg.FallbackToSandbox {
				return withSandboxFallback(req, safeErr)
			}
			return buildFailedExecutionResult(req, start, mode, safeErr)
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
			Output:     redactSecretsInValue(out, secrets).(map[string]any),
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
			envHint := findEnvFallbackForKey(toolCfg, reqKey)
			if envHint != "" {
				missingInputs = append(missingInputs, fmt.Sprintf("%s (กรอกในช่อง Node หรือตั้ง %s ใน .env)", reqKey, envHint))
			} else {
				missingInputs = append(missingInputs, reqKey)
			}
		}
	}
	if len(missingInputs) > 0 {
		reason := fmt.Sprintf("Missing required input: %s", strings.Join(missingInputs, ", "))
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
		return buildFailedExecutionResult(req, start, mode, redactSecrets(err.Error(), secrets))
	}

	if output != nil {
		if sanitized, ok := redactSecretsInValue(output, secrets).(map[string]any); ok {
			output = sanitized
		}
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

func findEnvFallbackForKey(tool *ToolConfig, key string) string {
	if tool == nil {
		return ""
	}
	for _, f := range tool.InputFields {
		if strings.EqualFold(f.Key, key) {
			return f.EnvFallback
		}
	}
	return ""
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
	vars, _ := extractExecutionVarsWithSecrets(req, nil)
	return vars
}

func extractExecutionVarsWithSecrets(req ExecutionRequest, tool *ToolConfig) (map[string]string, []string) {
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

	// 3. Explicit per-node UI inputs override auto-detected values (UI takes priority BEFORE .env!)
	for k, v := range req.Inputs {
		cleanKey := strings.ToLower(strings.TrimSpace(k))
		cleanVal := strings.TrimSpace(v)
		if cleanKey != "" && cleanVal != "" {
			vars[cleanKey] = cleanVal
		}
	}

	// 4. Cross-populate target / ip / query aliases so a single search box works seamlessly
	if vars["target"] == "" {
		switch {
		case vars["ip"] != "":
			vars["target"] = vars["ip"]
		case vars["hash"] != "":
			vars["target"] = vars["hash"]
		case vars["domain"] != "":
			vars["target"] = vars["domain"]
		case vars["query"] != "":
			vars["target"] = vars["query"]
		}
	}
	if vars["ip"] == "" {
		if ip := ipv4ExtractRegex.FindString(vars["target"]); ip != "" {
			vars["ip"] = ip
		} else if ip := ipv4ExtractRegex.FindString(vars["query"]); ip != "" {
			vars["ip"] = ip
		}
	}
	if vars["query"] == "" {
		if vars["target"] != "" {
			vars["query"] = vars["target"]
		} else if vars["ip"] != "" {
			vars["query"] = vars["ip"]
		}
	}

	// 5. Env fallback ONLY when the user left the node UI field blank + collect sensitive secrets
	var secrets []string
	if tool != nil {
		for _, field := range tool.InputFields {
			key := strings.ToLower(strings.TrimSpace(field.Key))
			if vars[key] == "" && field.EnvFallback != "" {
				if envVal := strings.TrimSpace(os.Getenv(field.EnvFallback)); envVal != "" {
					vars[key] = envVal
				}
			}
			if (field.Type == "password" || field.EnvFallback != "") && vars[key] != "" {
				secrets = append(secrets, vars[key])
			}
		}
		for _, envKey := range tool.RequiredEnv {
			if envVal := strings.TrimSpace(os.Getenv(envKey)); envVal != "" && !strings.HasPrefix(envVal, "http://") && !strings.HasPrefix(envVal, "https://") {
				secrets = append(secrets, envVal)
			}
		}
	}

	return vars, secrets
}

func redactSecrets(text string, secrets []string) string {
	out := text
	for _, s := range secrets {
		if len(s) >= 4 {
			out = strings.ReplaceAll(out, s, "***REDACTED***")
		}
	}
	return out
}

func redactSecretsInValue(v any, secrets []string) any {
	if len(secrets) == 0 {
		return v
	}
	switch typed := v.(type) {
	case string:
		return redactSecrets(typed, secrets)
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			out[k] = redactSecretsInValue(val, secrets)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, val := range typed {
			out[i] = redactSecretsInValue(val, secrets)
		}
		return out
	default:
		return v
	}
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
			if m[0] == 0 && (strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")) {
				val = raw
			} else {
				val = url.PathEscape(raw)
			}
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

	vars := extractExecutionVars(req)
	canonicalTool := NormalizeToolID(req.Tool)
	var output map[string]any
	switch canonicalTool {
	case "virustotal":
		target := vars["target"]
		if target == "" {
			target = "185.220.101.42"
		}
		output = map[string]any{
			"tool":            "VirusTotal API Sandbox",
			"step":            req.Label,
			"target":          target,
			"malicious_count": 12,
			"verdict":         "Malicious Indicator Confirmed",
		}
	case "abuseipdb":
		ip := vars["ip"]
		if ip == "" {
			ip = "185.220.101.42"
		}
		output = map[string]any{
			"tool":             "AbuseIPDB Sandbox",
			"step":             req.Label,
			"ip":               ip,
			"confidence_score": 85,
			"verdict":          "Suspicious Source",
		}
	case "shodan":
		query := vars["query"]
		if query == "" {
			query = "185.220.101.42"
		}
		output = map[string]any{
			"tool":       "Shodan API Sandbox",
			"step":       req.Label,
			"query":      query,
			"org":        "Example Hosting ASN",
			"open_ports": []int{22, 80, 443, 8080},
			"vulns":      []string{"CVE-2023-38408"},
			"verdict":    "Exposed Services Detected",
		}
	case "splunk":
		query := vars["query"]
		if query == "" {
			query = "search index=main | head 20"
		}
		output = map[string]any{
			"tool":          "Splunk SIEM Sandbox",
			"step":          req.Label,
			"query":         query,
			"matched_count": 4,
			"verdict":       "Suspicious Events Found",
		}
	case "ticket_search":
		output = searchInternalMockupTickets(req, vars)
	case "firewall_block":
		output = map[string]any{
			"tool":    "Firewall Controller",
			"step":    req.Label,
			"ip":      vars["ip"],
			"rule_id": fmt.Sprintf("FW-%d", time.Now().Unix()%10000),
			"result":  "Blocked Successfully",
		}
	case "ticket":
		output = createInternalMockupTicket(req, vars)
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