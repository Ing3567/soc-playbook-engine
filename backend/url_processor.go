package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

type NodeLink struct {
	Label       string `json:"label" bson:"label"`
	URL         string `json:"url" bson:"url"`
	Source      string `json:"source,omitempty" bson:"source,omitempty"`
	SplunkQuery string `json:"splunk_query,omitempty" bson:"splunk_query,omitempty"`
}

type URLRegistry struct {
	placeholderToURL map[string]string
	urlToPlaceholder map[string]string
	nextIndex        int
}

var (
	rawURLPattern      = regexp.MustCompile(`https?://[^\s<>"'\]]+`)
	placeholderPattern = regexp.MustCompile(`\[\[URL_\d+\]\]`)
)

func MaskURLsWithPlaceholders(input string) (string, *URLRegistry) {
	registry := &URLRegistry{
		placeholderToURL: make(map[string]string),
		urlToPlaceholder: make(map[string]string),
		nextIndex:        1,
	}

	masked := rawURLPattern.ReplaceAllStringFunc(input, func(match string) string {
		cleanedURL := trimTrailingURLPunctuation(match)
		trailingSuffix := strings.TrimPrefix(match, cleanedURL)

		placeholder, exists := registry.urlToPlaceholder[cleanedURL]
		if !exists {
			placeholder = fmt.Sprintf("[[URL_%d]]", registry.nextIndex)
			registry.nextIndex++
			registry.urlToPlaceholder[cleanedURL] = placeholder
			registry.placeholderToURL[placeholder] = cleanedURL
		}

		return placeholder + trailingSuffix
	})

	return masked, registry
}

func (r *URLRegistry) ResolveNodeLinks(explicitTokens []string, extraTexts ...string) []NodeLink {
	if r == nil {
		return nil
	}

	seenURLs := make(map[string]bool)
	var links []NodeLink

	appendResolved := func(candidate string) {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			return
		}

		resolvedURL := ""
		if mapped, ok := r.placeholderToURL[trimmed]; ok {
			resolvedURL = mapped
		} else if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
			resolvedURL = trimTrailingURLPunctuation(trimmed)
		}

		if resolvedURL == "" || seenURLs[resolvedURL] {
			return
		}
		seenURLs[resolvedURL] = true
		links = append(links, buildNodeLink(resolvedURL))
	}

	for _, token := range explicitTokens {
		matches := placeholderPattern.FindAllString(token, -1)
		if len(matches) > 0 {
			for _, m := range matches {
				appendResolved(m)
			}
		} else {
			appendResolved(token)
		}
	}

	for _, text := range extraTexts {
		for _, m := range placeholderPattern.FindAllString(text, -1) {
			appendResolved(m)
		}
	}

	return links
}

func (r *URLRegistry) StripPlaceholders(text string) string {
	cleaned := placeholderPattern.ReplaceAllString(text, "")
	cleaned = strings.ReplaceAll(cleaned, "()", "")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return strings.TrimSpace(cleaned)
}

func buildNodeLink(rawURL string) NodeLink {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return NodeLink{
			Label:  "Reference Link",
			URL:    rawURL,
			Source: "external",
		}
	}

	splunkQuery := extractSplunkQuery(parsed)
	if splunkQuery != "" {
		return NodeLink{
			Label:       "Splunk Query",
			URL:         rawURL,
			Source:      "splunk",
			SplunkQuery: splunkQuery,
		}
	}

	label := parsed.Host
	if label == "" {
		label = "Reference Link"
	}

	return NodeLink{
		Label:  label,
		URL:    rawURL,
		Source: "external",
	}
}

func extractSplunkQuery(parsed *url.URL) string {
	queryValue := strings.TrimSpace(parsed.Query().Get("q"))
	if queryValue == "" && strings.Contains(parsed.Fragment, "q=") {
		if idx := strings.Index(parsed.Fragment, "?"); idx != -1 {
			if fragValues, err := url.ParseQuery(parsed.Fragment[idx+1:]); err == nil {
				queryValue = strings.TrimSpace(fragValues.Get("q"))
			}
		}
	}
	if queryValue == "" {
		return ""
	}

	lowerHost := strings.ToLower(parsed.Host)
	lowerPath := strings.ToLower(parsed.Path)
	lowerQuery := strings.ToLower(queryValue)

	isSplunkURL := strings.Contains(lowerHost, "splunk") ||
		strings.Contains(lowerPath, "/app/") ||
		strings.Contains(lowerPath, "/search") ||
		strings.HasPrefix(lowerQuery, "search ") ||
		strings.Contains(lowerQuery, "index=") ||
		strings.Contains(lowerQuery, "|")

	if !isSplunkURL {
		return ""
	}

	return queryValue
}

func trimTrailingURLPunctuation(raw string) string {
	cleaned := strings.TrimRight(raw, ".,;")
	for strings.HasSuffix(cleaned, ")") && strings.Count(cleaned, "(") < strings.Count(cleaned, ")") {
		cleaned = strings.TrimSuffix(cleaned, ")")
	}
	return cleaned
}