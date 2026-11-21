package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"path"
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
	placeholderToURL   map[string]string
	urlToPlaceholder   map[string]string
	placeholderToLabel map[string]string
	urlToLabel         map[string]string
	nextIndex          int

	placeholderToImage map[string]NodeImage
	imageToPlaceholder map[string]string
	nextImageIndex     int

	lineHints []lineAssetHint
}

type lineAssetHint struct {
	cleanText        string
	urlPlaceholders  []string
	imgPlaceholders  []string
}

var (
	markdownImagePattern = regexp.MustCompile(`!\[([^\]]*)\]\((/api/images/[^\s)]+|https?://[^\s)]+)\)`)
	markdownLinkPattern  = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)
	rawURLPattern        = regexp.MustCompile(`https?://[^\s<>"'\])]+`)
	placeholderPattern   = regexp.MustCompile(`\[\[URL_\d+\]\]`)
	imageTokenPattern    = regexp.MustCompile(`\[\[IMG_\d+\]\]`)
	anyTokenPattern      = regexp.MustCompile(`\[\[(?:URL|IMG)_\d+\]\]`)
)

func NewURLRegistry() *URLRegistry {
	return &URLRegistry{
		placeholderToURL:   make(map[string]string),
		urlToPlaceholder:   make(map[string]string),
		placeholderToLabel: make(map[string]string),
		urlToLabel:         make(map[string]string),
		nextIndex:          1,
		placeholderToImage: make(map[string]NodeImage),
		imageToPlaceholder: make(map[string]string),
		nextImageIndex:     1,
	}
}

func (r *URLRegistry) RegisterURL(rawURL, customLabel string) string {
	cleanedURL := trimTrailingURLPunctuation(strings.TrimSpace(rawURL))
	if cleanedURL == "" {
		return ""
	}
	trimmedLabel := strings.TrimSpace(customLabel)
	if trimmedLabel == cleanedURL {
		trimmedLabel = ""
	}

	placeholder, exists := r.urlToPlaceholder[cleanedURL]
	if !exists {
		placeholder = fmt.Sprintf("[[URL_%d]]", r.nextIndex)
		r.nextIndex++
		r.urlToPlaceholder[cleanedURL] = placeholder
		r.placeholderToURL[placeholder] = cleanedURL
	}
	if trimmedLabel != "" && r.placeholderToLabel[placeholder] == "" {
		r.placeholderToLabel[placeholder] = trimmedLabel
		r.urlToLabel[cleanedURL] = trimmedLabel
	}
	return placeholder
}

func (r *URLRegistry) RegisterImage(img NodeImage) string {
	key := strings.TrimSpace(img.ID)
	if key == "" {
		key = strings.TrimSpace(img.URL)
	}
	if key == "" {
		return ""
	}
	if placeholder, exists := r.imageToPlaceholder[key]; exists {
		if img.Caption != "" {
			existing := r.placeholderToImage[placeholder]
			if existing.Caption == "" {
				existing.Caption = img.Caption
				r.placeholderToImage[placeholder] = existing
			}
		}
		return placeholder
	}
	placeholder := fmt.Sprintf("[[IMG_%d]]", r.nextImageIndex)
	r.nextImageIndex++
	r.imageToPlaceholder[key] = placeholder
	r.placeholderToImage[placeholder] = img
	return placeholder
}

func MaskURLsWithPlaceholders(input string) (string, *URLRegistry) {
	registry := NewURLRegistry()
	masked := registry.maskText(input, nil)
	return masked, registry
}

func (r *URLRegistry) maskText(input string, imageLookup func(id, caption string) NodeImage) string {
	working := markdownImagePattern.ReplaceAllStringFunc(input, func(match string) string {
		sub := markdownImagePattern.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		caption := strings.TrimSpace(sub[1])
		target := strings.TrimSpace(sub[2])
		var nodeImg NodeImage
		if strings.HasPrefix(target, "/api/images/") {
			id := strings.TrimPrefix(target, "/api/images/")
			if imageLookup != nil {
				nodeImg = imageLookup(id, caption)
			} else {
				nodeImg = NodeImage{
					ID:          id,
					Filename:    defaultCaptionFilename(caption, id),
					ContentType: "image/png",
					URL:         target,
					Caption:     caption,
				}
			}
		} else {
			base := path.Base(target)
			if base == "" || base == "." || base == "/" {
				base = "image.png"
			}
			nodeImg = NodeImage{
				ID:          target,
				Filename:    base,
				ContentType: "image/png",
				URL:         target,
				Caption:     caption,
			}
		}
		ph := r.RegisterImage(nodeImg)
		if ph == "" {
			return match
		}
		return ph
	})

	working = markdownLinkPattern.ReplaceAllStringFunc(working, func(match string) string {
		sub := markdownLinkPattern.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		label := strings.TrimSpace(sub[1])
		rawURL := strings.TrimSpace(sub[2])
		ph := r.RegisterURL(rawURL, label)
		if ph == "" {
			return match
		}
		if label != "" && !strings.EqualFold(label, rawURL) && !strings.Contains(label, rawURL) {
			return label + " " + ph
		}
		return ph
	})

	working = rawURLPattern.ReplaceAllStringFunc(working, func(match string) string {
		cleanedURL := trimTrailingURLPunctuation(match)
		trailingSuffix := strings.TrimPrefix(match, cleanedURL)
		ph := r.RegisterURL(cleanedURL, "")
		if ph == "" {
			return match
		}
		return ph + trailingSuffix
	})

	return working
}

func PrepareLinesAndRegistry(ctx context.Context, extracted []ExtractedLine, repo *PlaybookRepository) ([]Line, []Line, *URLRegistry) {
	registry := NewURLRegistry()
	displayLines := make([]Line, 0, len(extracted))
	maskedLines := make([]Line, 0, len(extracted))

	imageLookup := func(id, caption string) NodeImage {
		if repo != nil {
			if meta, err := repo.GetImageMeta(ctx, id); err == nil {
				if caption != "" && meta.Caption == "" {
					meta.Caption = caption
				}
				return meta
			}
		}
		return NodeImage{
			ID:          id,
			Filename:    defaultCaptionFilename(caption, id),
			ContentType: "image/png",
			URL:         "/api/images/" + id,
			Caption:     caption,
		}
	}

	for _, item := range extracted {
		displayBuilder := strings.TrimSpace(item.Text)
		maskedBuilder := registry.maskText(displayBuilder, imageLookup)

		for _, link := range item.Links {
			u := trimTrailingURLPunctuation(strings.TrimSpace(link.URL))
			if u == "" {
				continue
			}
			lbl := strings.TrimSpace(link.Label)
			ph := registry.RegisterURL(u, lbl)
			if ph == "" {
				continue
			}
			if !strings.Contains(displayBuilder, u) {
				safeLbl := strings.ReplaceAll(strings.ReplaceAll(lbl, "[", "("), "]", ")")
				if safeLbl != "" && safeLbl != u && !strings.Contains(displayBuilder, safeLbl) {
					displayBuilder = strings.TrimSpace(displayBuilder + fmt.Sprintf(" [%s](%s)", safeLbl, u))
				} else {
					displayBuilder = strings.TrimSpace(displayBuilder + " " + u)
				}
			}
			if !strings.Contains(maskedBuilder, ph) {
				if lbl != "" && lbl != u && !strings.Contains(maskedBuilder, lbl) {
					maskedBuilder = strings.TrimSpace(maskedBuilder + " " + lbl + " " + ph)
				} else {
					maskedBuilder = strings.TrimSpace(maskedBuilder + " " + ph)
				}
			}
		}

		for _, rawImg := range item.Images {
			if len(rawImg.Data) == 0 {
				continue
			}
			var savedImg NodeImage
			var err error
			if repo != nil {
				savedImg, err = repo.SaveImage(ctx, rawImg)
			}
			if err != nil {
				log.Printf("save image error (%s): %v", rawImg.Filename, err)
				continue
			}
			if savedImg.ID == "" {
				continue
			}
			ph := registry.RegisterImage(savedImg)
			if ph == "" {
				continue
			}
			caption := savedImg.Caption
			if caption == "" {
				caption = savedImg.Filename
			}
			caption = strings.ReplaceAll(strings.ReplaceAll(caption, "[", "("), "]", ")")
			imgMarkdown := fmt.Sprintf("![%s](%s)", caption, savedImg.URL)
			if !strings.Contains(displayBuilder, savedImg.URL) {
				displayBuilder = strings.TrimSpace(displayBuilder + " " + imgMarkdown)
			}
			if !strings.Contains(maskedBuilder, ph) {
				maskedBuilder = strings.TrimSpace(maskedBuilder + " " + ph)
			}
		}

		displayBuilder = strings.TrimSpace(displayBuilder)
		maskedBuilder = strings.TrimSpace(maskedBuilder)
		if displayBuilder == "" && maskedBuilder == "" {
			continue
		}

		displayLines = append(displayLines, Line{
			Level: item.Level,
			Text:  displayBuilder,
		})
		maskedLines = append(maskedLines, Line{
			Level: item.Level,
			Text:  maskedBuilder,
		})

		urlTokens := placeholderPattern.FindAllString(maskedBuilder, -1)
		imgTokens := imageTokenPattern.FindAllString(maskedBuilder, -1)
		if len(urlTokens) > 0 || len(imgTokens) > 0 {
			registry.lineHints = append(registry.lineHints, lineAssetHint{
				cleanText:       registry.StripPlaceholders(maskedBuilder),
				urlPlaceholders: urlTokens,
				imgPlaceholders: imgTokens,
			})
		}
	}

	return displayLines, maskedLines, registry
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
		customLabel := ""
		if mapped, ok := r.placeholderToURL[trimmed]; ok {
			resolvedURL = mapped
			customLabel = r.placeholderToLabel[trimmed]
		} else if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
			resolvedURL = trimTrailingURLPunctuation(trimmed)
			customLabel = r.urlToLabel[resolvedURL]
		}

		if resolvedURL == "" || seenURLs[resolvedURL] {
			return
		}
		seenURLs[resolvedURL] = true
		links = append(links, buildNodeLinkWithLabel(resolvedURL, customLabel))
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
		textWithoutImages := markdownImagePattern.ReplaceAllString(text, "")
		for _, sub := range markdownLinkPattern.FindAllStringSubmatch(textWithoutImages, -1) {
			if len(sub) >= 3 {
				u := trimTrailingURLPunctuation(strings.TrimSpace(sub[2]))
				lbl := strings.TrimSpace(sub[1])
				if u != "" && !seenURLs[u] {
					seenURLs[u] = true
					links = append(links, buildNodeLinkWithLabel(u, lbl))
				}
			}
		}
		for _, raw := range rawURLPattern.FindAllString(textWithoutImages, -1) {
			appendResolved(raw)
		}
	}

	return links
}

func (r *URLRegistry) ResolveNodeImages(explicitTokens []string, extraTexts ...string) []NodeImage {
	if r == nil {
		return nil
	}

	seenIDs := make(map[string]bool)
	var images []NodeImage

	appendResolved := func(candidate string) {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			return
		}
		if img, ok := r.placeholderToImage[trimmed]; ok {
			key := img.ID
			if key == "" {
				key = img.URL
			}
			if key != "" && !seenIDs[key] {
				seenIDs[key] = true
				images = append(images, img)
			}
			return
		}
		if strings.HasPrefix(trimmed, "/api/images/") {
			id := strings.TrimPrefix(trimmed, "/api/images/")
			if ph, ok := r.imageToPlaceholder[id]; ok {
				if img, found := r.placeholderToImage[ph]; found && !seenIDs[id] {
					seenIDs[id] = true
					images = append(images, img)
					return
				}
			}
			if !seenIDs[id] {
				seenIDs[id] = true
				images = append(images, NodeImage{
					ID:          id,
					Filename:    id + ".png",
					ContentType: "image/png",
					URL:         trimmed,
				})
			}
		}
	}

	for _, token := range explicitTokens {
		matches := imageTokenPattern.FindAllString(token, -1)
		if len(matches) > 0 {
			for _, m := range matches {
				appendResolved(m)
			}
		} else {
			appendResolved(token)
		}
	}

	for _, text := range extraTexts {
		for _, m := range imageTokenPattern.FindAllString(text, -1) {
			appendResolved(m)
		}
		for _, sub := range markdownImagePattern.FindAllStringSubmatch(text, -1) {
			if len(sub) >= 3 {
				appendResolved(sub[2])
			}
		}
	}

	return images
}

func (r *URLRegistry) StripPlaceholders(text string) string {
	cleaned := anyTokenPattern.ReplaceAllString(text, "")
	cleaned = markdownImagePattern.ReplaceAllString(cleaned, "")
	cleaned = markdownLinkPattern.ReplaceAllString(cleaned, "$1")
	cleaned = strings.ReplaceAll(cleaned, "()", "")
	cleaned = strings.ReplaceAll(cleaned, "[]", "")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return strings.TrimSpace(cleaned)
}

func (r *URLRegistry) CleanNodeLabel(rawLabel string, links []NodeLink, images []NodeImage) string {
	cleaned := r.StripPlaceholders(rawLabel)
	if cleaned != "" {
		return cleaned
	}
	if len(links) > 0 && strings.TrimSpace(links[0].Label) != "" {
		return links[0].Label
	}
	if len(images) > 0 {
		if strings.TrimSpace(images[0].Caption) != "" {
			return images[0].Caption
		}
		if strings.TrimSpace(images[0].Filename) != "" {
			return images[0].Filename
		}
	}
	return "ดำเนินการ"
}

func (r *URLRegistry) ReconcileSteps(steps []Step) []Step {
	if r == nil || len(steps) == 0 || len(r.lineHints) == 0 {
		return steps
	}

	usedTokens := make(map[string]bool)
	var markUsed func(list []Step)
	markUsed = func(list []Step) {
		for _, s := range list {
			for _, m := range anyTokenPattern.FindAllString(s.Label, -1) {
				usedTokens[m] = true
			}
			for _, l := range s.Links {
				for _, m := range placeholderPattern.FindAllString(l, -1) {
					usedTokens[m] = true
				}
				if ph, ok := r.urlToPlaceholder[l]; ok {
					usedTokens[ph] = true
				}
			}
			for _, img := range s.Images {
				for _, m := range imageTokenPattern.FindAllString(img, -1) {
					usedTokens[m] = true
				}
				if ph, ok := r.imageToPlaceholder[img]; ok {
					usedTokens[ph] = true
				}
			}
			for _, br := range s.Branches {
				for _, m := range anyTokenPattern.FindAllString(br.Label, -1) {
					usedTokens[m] = true
				}
				markUsed(br.Steps)
			}
		}
	}
	markUsed(steps)

	var stepPtrs []*Step
	var collectPtrs func(list []Step)
	collectPtrs = func(list []Step) {
		for i := range list {
			stepPtrs = append(stepPtrs, &list[i])
			for j := range list[i].Branches {
				collectPtrs(list[i].Branches[j].Steps)
			}
		}
	}
	collectPtrs(steps)
	if len(stepPtrs) == 0 {
		return steps
	}

	for _, hint := range r.lineHints {
		var missingURLs []string
		for _, ph := range hint.urlPlaceholders {
			if !usedTokens[ph] {
				missingURLs = append(missingURLs, ph)
			}
		}
		var missingImgs []string
		for _, ph := range hint.imgPlaceholders {
			if !usedTokens[ph] {
				missingImgs = append(missingImgs, ph)
			}
		}
		if len(missingURLs) == 0 && len(missingImgs) == 0 {
			continue
		}

		bestStep := findBestMatchingStep(stepPtrs, hint.cleanText, r)
		if bestStep == nil {
			bestStep = stepPtrs[0]
		}
		for _, ph := range missingURLs {
			bestStep.Links = appendUniqueString(bestStep.Links, ph)
			usedTokens[ph] = true
		}
		for _, ph := range missingImgs {
			bestStep.Images = appendUniqueString(bestStep.Images, ph)
			usedTokens[ph] = true
		}
	}

	return steps
}

func findBestMatchingStep(stepPtrs []*Step, hintText string, r *URLRegistry) *Step {
	hintLower := strings.ToLower(strings.TrimSpace(hintText))
	var best *Step
	bestScore := -1

	hintWords := strings.Fields(hintLower)
	for _, sp := range stepPtrs {
		stepClean := strings.ToLower(r.StripPlaceholders(sp.Label))
		if stepClean == "" {
			continue
		}
		score := 0
		if hintLower != "" && (strings.Contains(stepClean, hintLower) || strings.Contains(hintLower, stepClean)) {
			score += 100
		}
		for _, w := range hintWords {
			if len([]rune(w)) >= 2 && strings.Contains(stepClean, w) {
				score += 10
			}
		}
		if sp.Type != "decision" {
			score += 1
		}
		if score > bestScore {
			bestScore = score
			best = sp
		}
	}
	return best
}

func appendUniqueString(slice []string, val string) []string {
	for _, item := range slice {
		if item == val {
			return slice
		}
	}
	return append(slice, val)
}

func defaultCaptionFilename(caption, id string) string {
	trimmed := strings.TrimSpace(caption)
	if trimmed != "" {
		if strings.Contains(trimmed, ".") {
			return trimmed
		}
		return trimmed + ".png"
	}
	return id + ".png"
}

func buildNodeLink(rawURL string) NodeLink {
	return buildNodeLinkWithLabel(rawURL, "")
}

func buildNodeLinkWithLabel(rawURL, customLabel string) NodeLink {
	trimmedLabel := strings.TrimSpace(customLabel)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		if trimmedLabel == "" {
			trimmedLabel = "Reference Link"
		}
		return NodeLink{
			Label:  trimmedLabel,
			URL:    rawURL,
			Source: "external",
		}
	}

	splunkQuery := extractSplunkQuery(parsed)
	if splunkQuery != "" {
		label := trimmedLabel
		if label == "" || strings.EqualFold(label, rawURL) {
			label = "Splunk Query"
		}
		return NodeLink{
			Label:       label,
			URL:         rawURL,
			Source:      "splunk",
			SplunkQuery: splunkQuery,
		}
	}

	label := trimmedLabel
	if label == "" || strings.EqualFold(label, rawURL) {
		label = parsed.Host
	}
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