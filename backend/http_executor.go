package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const liveHTTPTimeout = 10 * time.Second
const maxLiveResponseBytes = 1 << 20 // 1 MB

var liveHTTPClient = &http.Client{Timeout: liveHTTPTimeout}

func RunLiveHTTPStep(req ExecutionRequest, toolDef *ToolDefinition) ExecutionResult {
	start := time.Now()
	fail := func(msg string, extra map[string]any) ExecutionResult {
		out := map[string]any{
			"mode":  "live",
			"tool":  req.Tool,
			"step":  req.Label,
			"error": msg,
		}
		if toolDef != nil {
			out["tool"] = toolDef.Name
			out["tool_id"] = toolDef.ID
		}
		for k, v := range extra {
			out[k] = v
		}
		return ExecutionResult{
			NodeID:     req.NodeID,
			Status:     "failed",
			DurationMs: time.Since(start).Milliseconds(),
			Timestamp:  time.Now().Format("15:04:05"),
			Output:     out,
		}
	}

	if !Config.AllowLiveExecution {
		return fail("Live HTTP execution is disabled on server (set ALLOW_LIVE_EXECUTION=true in .env)", nil)
	}
	if toolDef == nil {
		return fail("Live HTTP execution requires a configured Tool definition", nil)
	}
	if toolDef.CommandType != "" && toolDef.CommandType != "http-api" {
		return fail(fmt.Sprintf("Tool %q uses command_type %q (not http-api); run manually or in sandbox mode", toolDef.Name, toolDef.CommandType), nil)
	}

	resolved := BuildToolRequest(toolDef, req)
	if len(resolved.MissingRequired) > 0 {
		return fail(
			fmt.Sprintf("Missing required input parameters: %s", strings.Join(resolved.MissingRequired, ", ")),
			map[string]any{
				"http_method":    resolved.Method,
				"resolved_url":   resolved.URL,
				"missing_params": resolved.MissingRequired,
			},
		)
	}

	auth := ResolveToolAuth(toolDef, req.RuntimeAuth)
	if auth.Summary.Type != "none" && !auth.Summary.Configured {
		hint := "enter a session key"
		if auth.Summary.EnvVar != "" {
			hint = fmt.Sprintf("set %s in .env or enter a session key", auth.Summary.EnvVar)
		}
		return fail(
			fmt.Sprintf("Missing API credential (%s)", hint),
			map[string]any{
				"http_method":  resolved.Method,
				"resolved_url": resolved.URL,
				"auth_status":  "missing",
				"auth_type":    auth.Summary.Type,
				"auth_env_var": auth.Summary.EnvVar,
			},
		)
	}

	// Build actual outbound URL with unmasked query params + auth query param if applicable.
	actualURL := resolved.URL
	parsedURL, err := url.Parse(actualURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return fail(fmt.Sprintf("Invalid or relative endpoint URL %q (must start with http:// or https://)", actualURL), map[string]any{
			"http_method":  resolved.Method,
			"resolved_url": resolved.URL,
		})
	}
	q := parsedURL.Query()
	for k, v := range resolved.rawQuery {
		q.Set(k, v)
	}
	if auth.queryKey != "" && auth.queryValue != "" {
		q.Set(auth.queryKey, auth.queryValue)
	}
	parsedURL.RawQuery = q.Encode()
	actualURL = parsedURL.String()

	var bodyReader io.Reader
	if resolved.Method != http.MethodGet && resolved.Method != http.MethodDelete && len(resolved.rawBody) > 0 {
		bodyBytes, err := json.Marshal(resolved.rawBody)
		if err != nil {
			return fail("Failed to encode request body JSON", nil)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	httpReq, err := http.NewRequest(resolved.Method, actualURL, bodyReader)
	if err != nil {
		return fail(fmt.Sprintf("Failed to create HTTP request: %v", err), nil)
	}
	httpReq.Header.Set("Accept", "application/json")
	if bodyReader != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	for k, v := range resolved.rawHeaders {
		httpReq.Header.Set(k, v)
	}
	if auth.headerKey != "" && auth.headerValue != "" {
		httpReq.Header.Set(auth.headerKey, auth.headerValue)
	}

	resp, err := liveHTTPClient.Do(httpReq)
	if err != nil {
		return fail(fmt.Sprintf("HTTP request failed: %v", err), map[string]any{
			"http_method":  resolved.Method,
			"resolved_url": resolved.URL,
		})
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxLiveResponseBytes))
	if err != nil {
		return fail(fmt.Sprintf("Failed to read HTTP response: %v", err), map[string]any{
			"http_method":  resolved.Method,
			"resolved_url": resolved.URL,
			"http_status":  resp.StatusCode,
		})
	}

	var parsedBody any
	if len(bytes.TrimSpace(respBytes)) > 0 {
		if jsonErr := json.Unmarshal(respBytes, &parsedBody); jsonErr != nil {
			text := string(respBytes)
			if len(text) > 1000 {
				text = text[:1000] + "..."
			}
			parsedBody = text
		}
	}

	out := map[string]any{
		"mode":         "live",
		"tool":         toolDef.Name,
		"tool_id":      toolDef.ID,
		"step":         req.Label,
		"http_method":  resolved.Method,
		"resolved_url": resolved.URL,
		"http_status":  resp.StatusCode,
	}
	if auth.Summary.Type != "none" {
		out["auth_status"] = fmt.Sprintf("configured (%s)", auth.Summary.Source)
		if auth.Summary.MaskedPreview != "" {
			out["auth_preview"] = auth.Summary.MaskedPreview
		}
	}
	if parsedBody != nil {
		out["response"] = parsedBody
	}

	status := "success"
	if resp.StatusCode >= 400 {
		status = "failed"
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			out["error"] = fmt.Sprintf("Authentication failed (HTTP %d) — check API key or permissions", resp.StatusCode)
		case http.StatusTooManyRequests:
			out["error"] = fmt.Sprintf("Rate limit exceeded (HTTP %d)", resp.StatusCode)
		default:
			out["error"] = fmt.Sprintf("Upstream API returned HTTP %d", resp.StatusCode)
		}
	}

	return ExecutionResult{
		NodeID:     req.NodeID,
		Status:     status,
		DurationMs: time.Since(start).Milliseconds(),
		Timestamp:  time.Now().Format("15:04:05"),
		Output:     out,
	}
}
