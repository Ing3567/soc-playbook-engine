package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const liveHTTPTimeout = 10 * time.Second
const maxLiveResponseBytes = 1 << 20 // 1 MB

var cgnatRange = func() *net.IPNet {
	_, ipNet, _ := net.ParseCIDR("100.64.0.0/10")
	return ipNet
}()

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if cgnatRange != nil && cgnatRange.Contains(ip) {
		return true
	}
	return false
}

func validateLiveTargetURL(parsed *url.URL, allowedHosts []string) error {
	if parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("invalid URL scheme or host (must be http:// or https://)")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return fmt.Errorf("missing target hostname")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("target host %q is not allowed (internal/localhost)", host)
	}
	if ip := net.ParseIP(host); ip != nil && isBlockedIP(ip) {
		return fmt.Errorf("target IP %q is blocked (private/loopback/link-local)", ip.String())
	}
	if len(allowedHosts) > 0 {
		matched := false
		for _, allowed := range allowedHosts {
			if host == strings.ToLower(strings.TrimSpace(allowed)) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("target host %q is not in LIVE_ALLOWED_HOSTS", host)
		}
	}
	return nil
}

func newSafeLiveHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("no IP addresses resolved for %q", host)
			}
			for _, ip := range ips {
				if isBlockedIP(ip.IP) {
					return nil, fmt.Errorf("blocked connection to internal/private IP %s (%s)", ip.IP.String(), host)
				}
			}
			// Dial the validated IP directly to prevent DNS rebinding.
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
	return &http.Client{
		Timeout:   liveHTTPTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after 5 redirects")
			}
			return validateLiveTargetURL(req.URL, Config.LiveAllowedHosts)
		},
	}
}

var liveHTTPClient = newSafeLiveHTTPClient()
var validateLiveURLFunc = validateLiveTargetURL

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
	if err != nil {
		return fail(fmt.Sprintf("Invalid endpoint URL %q: %v", actualURL, err), map[string]any{
			"http_method":  resolved.Method,
			"resolved_url": resolved.URL,
		})
	}
	if err := validateLiveURLFunc(parsedURL, Config.LiveAllowedHosts); err != nil {
		return fail(fmt.Sprintf("Blocked target URL %q: %v", resolved.URL, err), map[string]any{
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
