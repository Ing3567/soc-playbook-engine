package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var (
	outlineNumberPrefix  = regexp.MustCompile(`^\s*(\d+(?:\.\d+)+)[\.\)]?\s+`)
	listBulletPrefix     = regexp.MustCompile(`^\s*(\d+[\.\)]|[-*•●○▪])\s*`)
	pureStepNumberRegex  = regexp.MustCompile(`^\s*(\d+(?:\.\d+)*)[\.\)]?\s*$`)
	instrHyperlinkRegex  = regexp.MustCompile(`(?i)HYPERLINK\s+"(https?://[^"]+)"`)
	commonHeaderRowRegex = regexp.MustCompile(`(?i)^(step|no\.?|#|ลำดับ|ขั้นตอน|รายละเอียด|action|description|tool|หมายเหตุ|remark|screenshot|รูปภาพ|\s|\|)+$`)
)

type docxRelationships struct {
	Relationships []docxRelationship `xml:"Relationship"`
}

type docxRelationship struct {
	ID         string `xml:"Id,attr"`
	Type       string `xml:"Type,attr"`
	Target     string `xml:"Target,attr"`
	TargetMode string `xml:"TargetMode,attr"`
}

type docxRelMap struct {
	hyperlinks map[string]string
	images     map[string]string
}

func parseXmlIntAttribute(element xml.StartElement, name string) int {
	for _, attr := range element.Attr {
		if attr.Name.Local == name {
			v, _ := strconv.Atoi(attr.Value)
			return v
		}
	}
	return 0
}

func getXmlAttr(element xml.StartElement, localNames ...string) string {
	for _, want := range localNames {
		for _, attr := range element.Attr {
			if attr.Name.Local == want {
				return attr.Value
			}
		}
	}
	return ""
}

func ExtractDocxText(data []byte) (string, error) {
	lines, err := ExtractLinesFromDocx(data)
	if err != nil {
		return "", err
	}
	var out []string
	for _, l := range lines {
		out = append(out, strings.Repeat("  ", l.Level)+l.Text)
	}
	return strings.Join(out, "\n"), nil
}

func ExtractLinesFromDocx(data []byte) ([]ExtractedLine, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("not a valid docx archive")
	}

	filesByName := make(map[string]*zip.File, len(reader.File))
	for _, f := range reader.File {
		filesByName[f.Name] = f
	}

	docFile, ok := filesByName["word/document.xml"]
	if !ok {
		return nil, errors.New("word/document.xml not found in docx")
	}

	rels := loadDocxRelationships(filesByName)

	rc, openErr := docFile.Open()
	if openErr != nil {
		return nil, openErr
	}
	defer rc.Close()

	return parseDocxDocumentXML(io.LimitReader(rc, 25<<20), rels, filesByName)
}

func loadDocxRelationships(filesByName map[string]*zip.File) docxRelMap {
	result := docxRelMap{
		hyperlinks: make(map[string]string),
		images:     make(map[string]string),
	}

	relFile, ok := filesByName["word/_rels/document.xml.rels"]
	if !ok {
		return result
	}
	rc, err := relFile.Open()
	if err != nil {
		return result
	}
	defer rc.Close()

	var rels docxRelationships
	if decodeErr := xml.NewDecoder(rc).Decode(&rels); decodeErr != nil {
		return result
	}

	for _, rel := range rels.Relationships {
		target := strings.TrimSpace(rel.Target)
		if target == "" {
			continue
		}
		lowerType := strings.ToLower(rel.Type)
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasSuffix(lowerType, "/hyperlink") {
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
				result.hyperlinks[rel.ID] = target
			}
			continue
		}
		if strings.HasSuffix(lowerType, "/image") || isMediaTarget(target) {
			zipPath := resolveZipRelativePath("word", target)
			result.images[rel.ID] = zipPath
		}
	}

	return result
}

func isMediaTarget(target string) bool {
	lower := strings.ToLower(target)
	if strings.Contains(lower, "media/") {
		return true
	}
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg", ".tif", ".tiff"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func resolveZipRelativePath(baseDir, target string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(target, "\\", "/"))
	if strings.HasPrefix(trimmed, "/") {
		return strings.TrimPrefix(path.Clean(trimmed), "/")
	}
	return path.Clean(baseDir + "/" + trimmed)
}

func readZipMediaImage(filesByName map[string]*zip.File, zipPath, caption string) (ExtractedImage, bool) {
	zf, ok := filesByName[zipPath]
	if !ok {
		return ExtractedImage{}, false
	}
	rc, err := zf.Open()
	if err != nil {
		return ExtractedImage{}, false
	}
	defer rc.Close()

	imgBytes, err := io.ReadAll(io.LimitReader(rc, 20<<20))
	if err != nil || len(imgBytes) == 0 {
		return ExtractedImage{}, false
	}
	filename := path.Base(zipPath)
	return ExtractedImage{
		Filename:    filename,
		ContentType: sanitizeImageContentType("", filename, imgBytes),
		Data:        imgBytes,
		Caption:     strings.TrimSpace(caption),
	}, true
}

func parseDocxDocumentXML(reader io.Reader, rels docxRelMap, filesByName map[string]*zip.File) ([]ExtractedLine, error) {
	decoder := xml.NewDecoder(reader)

	var lines []ExtractedLine

	var paragraphBuilder strings.Builder
	var hyperlinkBuilder strings.Builder
	var paraLinks []ExtractedLink
	var paraImages []ExtractedImage
	level, hasListLevel := 0, false
	inHyperlink := false
	activeHyperlinkURL := ""
	fieldHyperlinkURL := ""
	inFieldResult := false
	var fieldResultBuilder strings.Builder
	pendingImageCaption := ""

	inTableRow := false
	inTableCell := false
	tableRowIndex := 0
	paraDepth := 0
	var currentCellLines []ExtractedLine
	var currentRowCells [][]ExtractedLine

	flushActiveHyperlink := func(targetURL, label string) {
		label = strings.TrimSpace(label)
		targetURL = strings.TrimSpace(targetURL)
		if targetURL != "" {
			paraLinks = append(paraLinks, ExtractedLink{
				Label: label,
				URL:   targetURL,
			})
			if label != "" && !strings.Contains(label, targetURL) {
				paragraphBuilder.WriteString(fmt.Sprintf("[%s](%s)", label, targetURL))
			} else {
				paragraphBuilder.WriteString(targetURL)
			}
		} else if label != "" {
			paragraphBuilder.WriteString(label)
		}
	}

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch element := tok.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "tbl":
				tableRowIndex = 0
			case "tr":
				inTableRow = true
				currentRowCells = nil
			case "tc":
				if inTableRow {
					inTableCell = true
					currentCellLines = nil
				}
			case "p":
				paraDepth++
				if paraDepth > 1 {
					continue
				}
				paragraphBuilder.Reset()
				hyperlinkBuilder.Reset()
				fieldResultBuilder.Reset()
				paraLinks = nil
				paraImages = nil
				level, hasListLevel = 0, false
				inHyperlink = false
				activeHyperlinkURL = ""
				fieldHyperlinkURL = ""
				inFieldResult = false
				pendingImageCaption = ""

			case "ilvl":
				if paraDepth == 1 {
					level, hasListLevel = parseXmlIntAttribute(element, "val"), true
				}

			case "ind":
				if paraDepth == 1 && !hasListLevel {
					left := parseXmlIntAttribute(element, "left")
					if s := parseXmlIntAttribute(element, "start"); s > left {
						left = s
					}
					level = left / 360
				}

			case "hyperlink":
				inHyperlink = true
				hyperlinkBuilder.Reset()
				relID := getXmlAttr(element, "id")
				activeHyperlinkURL = rels.hyperlinks[relID]

			case "fldSimple":
				instr := getXmlAttr(element, "instr")
				if m := instrHyperlinkRegex.FindStringSubmatch(instr); len(m) >= 2 {
					inHyperlink = true
					hyperlinkBuilder.Reset()
					activeHyperlinkURL = strings.TrimSpace(m[1])
				}

			case "fldChar":
				fldType := getXmlAttr(element, "fldCharType")
				switch fldType {
				case "begin":
					fieldHyperlinkURL = ""
					inFieldResult = false
					fieldResultBuilder.Reset()
				case "separate":
					if fieldHyperlinkURL != "" {
						inFieldResult = true
						fieldResultBuilder.Reset()
					}
				case "end":
					if fieldHyperlinkURL != "" {
						flushActiveHyperlink(fieldHyperlinkURL, fieldResultBuilder.String())
					}
					fieldHyperlinkURL = ""
					inFieldResult = false
					fieldResultBuilder.Reset()
				}

			case "instrText":
				var instr string
				if decodeErr := decoder.DecodeElement(&instr, &element); decodeErr == nil {
					if m := instrHyperlinkRegex.FindStringSubmatch(instr); len(m) >= 2 {
						fieldHyperlinkURL = strings.TrimSpace(m[1])
					}
				}

			case "docPr", "cNvPr":
				descr := strings.TrimSpace(getXmlAttr(element, "descr", "title", "name"))
				if descr != "" {
					pendingImageCaption = descr
				}

			case "blip":
				relID := getXmlAttr(element, "embed", "link")
				if zipPath, ok := rels.images[relID]; ok {
					if img, found := readZipMediaImage(filesByName, zipPath, pendingImageCaption); found {
						paraImages = append(paraImages, img)
					}
				}
				pendingImageCaption = ""

			case "imagedata":
				relID := getXmlAttr(element, "id")
				caption := getXmlAttr(element, "title")
				if caption == "" {
					caption = pendingImageCaption
				}
				if zipPath, ok := rels.images[relID]; ok {
					if img, found := readZipMediaImage(filesByName, zipPath, caption); found {
						paraImages = append(paraImages, img)
					}
				}
				pendingImageCaption = ""

			case "t":
				var text string
				if decodeErr := decoder.DecodeElement(&text, &element); decodeErr != nil {
					return nil, decodeErr
				}
				if inHyperlink {
					hyperlinkBuilder.WriteString(text)
				} else if inFieldResult {
					fieldResultBuilder.WriteString(text)
				} else {
					paragraphBuilder.WriteString(text)
				}

			case "br", "cr":
				if inHyperlink {
					hyperlinkBuilder.WriteString(" ")
				} else if inFieldResult {
					fieldResultBuilder.WriteString(" ")
				} else {
					paragraphBuilder.WriteString(" ")
				}

			case "tab":
				if !inHyperlink && !inFieldResult {
					paragraphBuilder.WriteString("\t")
				}
			}

		case xml.EndElement:
			switch element.Name.Local {
			case "hyperlink", "fldSimple":
				if inHyperlink {
					flushActiveHyperlink(activeHyperlinkURL, hyperlinkBuilder.String())
					inHyperlink = false
					activeHyperlinkURL = ""
					hyperlinkBuilder.Reset()
				}

			case "p":
				if paraDepth > 1 {
					paraDepth--
					paragraphBuilder.WriteString(" ")
					continue
				}
				paraDepth = 0
				if inFieldResult && fieldHyperlinkURL != "" {
					flushActiveHyperlink(fieldHyperlinkURL, fieldResultBuilder.String())
					fieldHyperlinkURL = ""
					inFieldResult = false
					fieldResultBuilder.Reset()
				}
				rawPara := strings.TrimSpace(paragraphBuilder.String())
				normalizedLevel, cleanedText := normalizeOutlineAndBullet(level, rawPara)
				if cleanedText == "" && len(paraLinks) == 0 && len(paraImages) == 0 {
					continue
				}
				item := ExtractedLine{
					Level:  normalizedLevel,
					Text:   cleanedText,
					Links:  paraLinks,
					Images: paraImages,
				}
				if inTableCell {
					currentCellLines = append(currentCellLines, item)
				} else {
					lines = appendOrMergeExtractedLine(lines, item)
				}

			case "tc":
				if inTableCell {
					currentRowCells = append(currentRowCells, currentCellLines)
					currentCellLines = nil
					inTableCell = false
				}

			case "tr":
				if inTableRow {
					lines = mergeTableRowIntoLines(lines, currentRowCells, tableRowIndex)
					tableRowIndex++
					currentRowCells = nil
					inTableRow = false
				}
			}
		}
	}

	return lines, nil
}

func normalizeOutlineAndBullet(level int, rawText string) (int, string) {
	trimmed := strings.TrimSpace(rawText)
	if trimmed == "" {
		return level, ""
	}
	if m := outlineNumberPrefix.FindStringSubmatch(trimmed); len(m) >= 2 {
		dots := strings.Count(m[1], ".")
		if level == 0 && dots > 0 {
			level = dots
		}
		trimmed = strings.TrimSpace(trimmed[len(m[0]):])
		return level, trimmed
	}
	cleaned := strings.TrimSpace(listBulletPrefix.ReplaceAllString(trimmed, ""))
	return level, cleaned
}

func appendOrMergeExtractedLine(lines []ExtractedLine, item ExtractedLine) []ExtractedLine {
	if len(lines) > 0 {
		prev := &lines[len(lines)-1]
		if item.Text == "" && (len(item.Images) > 0 || len(item.Links) > 0) {
			prev.Links = append(prev.Links, item.Links...)
			prev.Images = append(prev.Images, item.Images...)
			return lines
		}
		if isBareURLOrAssetOnlyText(item.Text) {
			prev.Text = strings.TrimSpace(prev.Text + " " + item.Text)
			prev.Links = append(prev.Links, item.Links...)
			prev.Images = append(prev.Images, item.Images...)
			return lines
		}
	}
	return append(lines, item)
}

func isBareURLOrAssetOnlyText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	withoutAssets := markdownImagePattern.ReplaceAllString(trimmed, "")
	withoutAssets = rawURLPattern.ReplaceAllString(withoutAssets, "")
	return strings.TrimSpace(withoutAssets) == ""
}

func mergeTableRowIntoLines(lines []ExtractedLine, rowCells [][]ExtractedLine, rowIdx int) []ExtractedLine {
	var textParts []string
	var rowLinks []ExtractedLink
	var rowImages []ExtractedImage
	inferredLevel := 0
	hasExplicitLevel := false

	for cellIdx, cellLines := range rowCells {
		var cellTexts []string
		for _, cl := range cellLines {
			rowLinks = append(rowLinks, cl.Links...)
			rowImages = append(rowImages, cl.Images...)
			if cl.Level > 0 && !hasExplicitLevel {
				inferredLevel = cl.Level
				hasExplicitLevel = true
			}
			t := strings.TrimSpace(cl.Text)
			if t == "" {
				continue
			}
			if cellIdx == 0 && pureStepNumberRegex.MatchString(t) {
				m := pureStepNumberRegex.FindStringSubmatch(t)
				if len(m) >= 2 {
					dots := strings.Count(m[1], ".")
					if dots > 0 && !hasExplicitLevel {
						inferredLevel = dots
						hasExplicitLevel = true
					}
				}
				continue
			}
			cellTexts = append(cellTexts, t)
		}
		if len(cellTexts) > 0 {
			textParts = append(textParts, strings.Join(cellTexts, " "))
		}
	}

	joinedText := strings.TrimSpace(strings.Join(textParts, " "))
	if joinedText == "" && len(rowLinks) == 0 && len(rowImages) == 0 {
		return lines
	}

	if rowIdx == 0 && len(rowLinks) == 0 && len(rowImages) == 0 && commonHeaderRowRegex.MatchString(joinedText) {
		return lines
	}

	lvl, cleanedText := normalizeOutlineAndBullet(inferredLevel, joinedText)
	return appendOrMergeExtractedLine(lines, ExtractedLine{
		Level:  lvl,
		Text:   cleanedText,
		Links:  rowLinks,
		Images: rowImages,
	})
}

func ExtractLinesFromText(raw string) []ExtractedLine {
	var lines []ExtractedLine
	for _, rawLine := range strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n") {
		spaces := 0
		for _, ch := range rawLine {
			if ch == ' ' {
				spaces++
			} else if ch == '\t' {
				spaces += 4
			} else {
				break
			}
		}
		lvl, cleaned := normalizeOutlineAndBullet(spaces/2, strings.TrimSpace(rawLine))
		if cleaned != "" {
			lines = appendOrMergeExtractedLine(lines, ExtractedLine{
				Level: lvl,
				Text:  cleaned,
			})
		}
	}
	return lines
}