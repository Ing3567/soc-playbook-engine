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
	"sort"
	"strconv"
	"strings"
)

var (
	excelCellRefRegex     = regexp.MustCompile(`^([A-Za-z]+)(\d+)`)
	excelHyperlinkFnRegex = regexp.MustCompile(`(?i)HYPERLINK\s*\(\s*"(https?://[^"]+)"(?:\s*[,;]\s*"([^"]*)")?\s*\)`)
)

type xlsxWorkbook struct {
	Sheets []xlsxSheetEntry `xml:"sheets>sheet"`
}

type xlsxSheetEntry struct {
	Name string `xml:"name,attr"`
	ID   string `xml:"id,attr"`
}

type xlsxCellData struct {
	col      int
	text     string
	indent   int
	links    []ExtractedLink
	images   []ExtractedImage
}

type xlsxRowData struct {
	rowNum int
	cells  []xlsxCellData
	images []ExtractedImage
	links  []ExtractedLink
}

func ExtractLinesFromXlsx(data []byte) ([]ExtractedLine, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("not a valid xlsx archive")
	}

	filesByName := make(map[string]*zip.File, len(reader.File))
	for _, f := range reader.File {
		filesByName[f.Name] = f
	}

	sharedStrings := loadXlsxSharedStrings(filesByName)
	styleIndents := loadXlsxStyleIndents(filesByName)
	richValueImages := loadXlsxRichValueImages(filesByName)
	sheetPaths := resolveXlsxSheetPaths(filesByName)
	if len(sheetPaths) == 0 {
		return nil, errors.New("no worksheets found in xlsx")
	}

	var allLines []ExtractedLine

	for _, sheetPath := range sheetPaths {
		sheetLines, err := parseXlsxWorksheet(sheetPath, filesByName, sharedStrings, styleIndents, richValueImages)
		if err != nil {
			continue
		}
		for _, item := range sheetLines {
			allLines = appendOrMergeExtractedLine(allLines, item)
		}
	}

	return allLines, nil
}

func resolveXlsxSheetPaths(filesByName map[string]*zip.File) []string {
	wbFile, hasWB := filesByName["xl/workbook.xml"]
	if hasWB {
		wbRels := loadGenericRels(filesByName, "xl/_rels/workbook.xml.rels")
		if rc, err := wbFile.Open(); err == nil {
			defer rc.Close()
			var wb xlsxWorkbook
			if xml.NewDecoder(rc).Decode(&wb) == nil && len(wb.Sheets) > 0 {
				var paths []string
				for _, s := range wb.Sheets {
					if rel, ok := wbRels[s.ID]; ok {
						p := resolveZipRelativePath("xl", rel.Target)
						if _, exists := filesByName[p]; exists {
							paths = append(paths, p)
						}
					}
				}
				if len(paths) > 0 {
					return paths
				}
			}
		}
	}

	var fallback []string
	for name := range filesByName {
		if strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml") {
			fallback = append(fallback, name)
		}
	}
	sort.Strings(fallback)
	return fallback
}

func loadGenericRels(filesByName map[string]*zip.File, relZipPath string) map[string]docxRelationship {
	res := make(map[string]docxRelationship)
	zf, ok := filesByName[relZipPath]
	if !ok {
		return res
	}
	rc, err := zf.Open()
	if err != nil {
		return res
	}
	defer rc.Close()

	var rels docxRelationships
	if xml.NewDecoder(rc).Decode(&rels) != nil {
		return res
	}
	for _, r := range rels.Relationships {
		res[r.ID] = r
	}
	return res
}

func loadXlsxSharedStrings(filesByName map[string]*zip.File) []string {
	zf, ok := filesByName["xl/sharedStrings.xml"]
	if !ok {
		return nil
	}
	rc, err := zf.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()

	decoder := xml.NewDecoder(io.LimitReader(rc, 20<<20))
	var result []string
	var current strings.Builder
	inSI := false
	inRPh := false

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "si":
				inSI = true
				current.Reset()
			case "rPh":
				inRPh = true
			case "t":
				var t string
				if decoder.DecodeElement(&t, &el) == nil && inSI && !inRPh {
					current.WriteString(t)
				}
			}
		case xml.EndElement:
			switch el.Name.Local {
			case "rPh":
				inRPh = false
			case "si":
				if inSI {
					result = append(result, current.String())
					inSI = false
				}
			}
		}
	}
	return result
}

func loadXlsxStyleIndents(filesByName map[string]*zip.File) []int {
	zf, ok := filesByName["xl/styles.xml"]
	if !ok {
		return nil
	}
	rc, err := zf.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()

	decoder := xml.NewDecoder(io.LimitReader(rc, 10<<20))
	var indents []int
	inCellXfs := false
	currentIndent := 0

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "cellXfs":
				inCellXfs = true
			case "xf":
				if inCellXfs {
					currentIndent = 0
				}
			case "alignment":
				if inCellXfs {
					currentIndent = parseXmlIntAttribute(el, "indent")
				}
			}
		case xml.EndElement:
			switch el.Name.Local {
			case "xf":
				if inCellXfs {
					indents = append(indents, currentIndent)
				}
			case "cellXfs":
				inCellXfs = false
			}
		}
	}
	return indents
}

func loadXlsxRichValueImages(filesByName map[string]*zip.File) map[int]ExtractedImage {
	result := make(map[int]ExtractedImage)
	relMap := loadGenericRels(filesByName, "xl/richData/_rels/richValueRel.xml.rels")
	if len(relMap) == 0 {
		return result
	}

	relOrderFile, ok := filesByName["xl/richData/richValueRel.xml"]
	if !ok {
		return result
	}
	rc, err := relOrderFile.Open()
	if err != nil {
		return result
	}
	defer rc.Close()

	var orderedImages []ExtractedImage
	decoder := xml.NewDecoder(rc)
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if el, isStart := tok.(xml.StartElement); isStart && el.Name.Local == "rel" {
			rID := getXmlAttr(el, "id")
			if rel, found := relMap[rID]; found {
				zipPath := resolveZipRelativePath("xl/richData", rel.Target)
				if img, ok := readZipMediaImage(filesByName, zipPath, ""); ok {
					orderedImages = append(orderedImages, img)
				} else {
					orderedImages = append(orderedImages, ExtractedImage{})
				}
			}
		}
	}
	if len(orderedImages) == 0 {
		return result
	}

	rvToRelIdx := loadXlsxRichValueRelIndices(filesByName)
	vmToRvIdx := loadXlsxValueMetadataIndices(filesByName)
	if len(vmToRvIdx) > 0 {
		for vm, rvIdx := range vmToRvIdx {
			relIdx := rvIdx
			if mapped, exists := rvToRelIdx[rvIdx]; exists {
				relIdx = mapped
			}
			if relIdx >= 0 && relIdx < len(orderedImages) && len(orderedImages[relIdx].Data) > 0 {
				result[vm] = orderedImages[relIdx]
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	for idx, img := range orderedImages {
		if len(img.Data) > 0 {
			result[idx+1] = img
		}
	}
	return result
}

func loadXlsxValueMetadataIndices(filesByName map[string]*zip.File) map[int]int {
	res := make(map[int]int)
	zf, ok := filesByName["xl/metadata.xml"]
	if !ok {
		return res
	}
	rc, err := zf.Open()
	if err != nil {
		return res
	}
	defer rc.Close()

	decoder := xml.NewDecoder(io.LimitReader(rc, 5<<20))
	inValueMeta := false
	vmIdx := 0
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "valueMetadata":
				inValueMeta = true
			case "bk":
				if inValueMeta {
					vmIdx++
				}
			case "rc":
				if inValueMeta && vmIdx > 0 {
					res[vmIdx] = parseXmlIntAttribute(el, "v")
				}
			}
		case xml.EndElement:
			if el.Name.Local == "valueMetadata" {
				inValueMeta = false
			}
		}
	}
	return res
}

func loadXlsxRichValueRelIndices(filesByName map[string]*zip.File) map[int]int {
	res := make(map[int]int)
	zf, ok := filesByName["xl/richData/rdrichvalue.xml"]
	if !ok {
		return res
	}
	rc, err := zf.Open()
	if err != nil {
		return res
	}
	defer rc.Close()

	decoder := xml.NewDecoder(io.LimitReader(rc, 5<<20))
	rvIdx := -1
	capturedFirstVal := false
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if el, ok := tok.(xml.StartElement); ok {
			switch el.Name.Local {
			case "rv":
				rvIdx++
				capturedFirstVal = false
			case "v":
				if rvIdx >= 0 && !capturedFirstVal {
					var val string
					if decoder.DecodeElement(&val, &el) == nil {
						if parsed, convErr := strconv.Atoi(strings.TrimSpace(val)); convErr == nil {
							res[rvIdx] = parsed
							capturedFirstVal = true
						}
					}
				}
			}
		}
	}
	return res
}

func parseXlsxWorksheet(
	sheetPath string,
	filesByName map[string]*zip.File,
	sharedStrings []string,
	styleIndents []int,
	richValueImages map[int]ExtractedImage,
) ([]ExtractedLine, error) {
	zf, ok := filesByName[sheetPath]
	if !ok {
		return nil, errors.New("sheet not found")
	}

	sheetDir := path.Dir(sheetPath)
	sheetBase := path.Base(sheetPath)
	sheetRelsPath := path.Join(sheetDir, "_rels", sheetBase+".rels")
	sheetRels := loadGenericRels(filesByName, sheetRelsPath)

	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	rowsByNum := make(map[int]*xlsxRowData)
	getOrCreateRow := func(rNum int) *xlsxRowData {
		if rNum <= 0 {
			rNum = 1
		}
		r, exists := rowsByNum[rNum]
		if !exists {
			r = &xlsxRowData{rowNum: rNum}
			rowsByNum[rNum] = r
		}
		return r
	}

	decoder := xml.NewDecoder(io.LimitReader(rc, 25<<20))
	currentRowNum := 0
	currentColIdx := 0
	cellType := ""
	cellStyleIdx := 0
	cellVM := 0
	cellRef := ""
	cellFormula := ""
	cellValue := ""
	var inlineBuilder strings.Builder
	inCell := false
	inInlineStr := false
	var drawingRelIDs []string

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "row":
				rAttr := parseXmlIntAttribute(el, "r")
				if rAttr > 0 {
					currentRowNum = rAttr
				} else {
					currentRowNum++
				}
				currentColIdx = 0

			case "c":
				inCell = true
				cellRef = getXmlAttr(el, "r")
				cellType = getXmlAttr(el, "t")
				cellStyleIdx = parseXmlIntAttribute(el, "s")
				cellVM = parseXmlIntAttribute(el, "vm")
				cellFormula = ""
				cellValue = ""
				inlineBuilder.Reset()
				if col, row, ok := parseExcelCellRef(cellRef); ok {
					currentColIdx = col
					if row > 0 {
						currentRowNum = row
					}
				}

			case "f":
				if inCell {
					_ = decoder.DecodeElement(&cellFormula, &el)
				}

			case "v":
				if inCell {
					_ = decoder.DecodeElement(&cellValue, &el)
				}

			case "is":
				if inCell {
					inInlineStr = true
				}

			case "t":
				if inCell && inInlineStr {
					var t string
					if decoder.DecodeElement(&t, &el) == nil {
						inlineBuilder.WriteString(t)
					}
				}

			case "hyperlink":
				ref := getXmlAttr(el, "ref")
				rID := getXmlAttr(el, "id")
				display := strings.TrimSpace(getXmlAttr(el, "display"))
				location := strings.TrimSpace(getXmlAttr(el, "location"))
				targetURL := ""
				if rel, found := sheetRels[rID]; found {
					targetURL = strings.TrimSpace(rel.Target)
				}
				if targetURL == "" && (strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://")) {
					targetURL = location
				}
				if strings.HasPrefix(targetURL, "http://") || strings.HasPrefix(targetURL, "https://") {
					col, row, ok := parseExcelCellRef(strings.Split(ref, ":")[0])
					if !ok || row <= 0 {
						row = currentRowNum
					}
					rowObj := getOrCreateRow(row)
					attached := false
					for i := range rowObj.cells {
						if rowObj.cells[i].col == col {
							lbl := strings.TrimSpace(rowObj.cells[i].text)
							if lbl == "" {
								lbl = display
							}
							rowObj.cells[i].links = append(rowObj.cells[i].links, ExtractedLink{
								Label: lbl,
								URL:   targetURL,
							})
							attached = true
							break
						}
					}
					if !attached {
						rowObj.links = append(rowObj.links, ExtractedLink{
							Label: display,
							URL:   targetURL,
						})
					}
				}

			case "drawing", "legacyDrawing":
				if rID := getXmlAttr(el, "id"); rID != "" {
					drawingRelIDs = append(drawingRelIDs, rID)
				}
			}

		case xml.EndElement:
			switch el.Name.Local {
			case "is":
				inInlineStr = false

			case "c":
				if !inCell {
					continue
				}
				inCell = false

				resolvedText := resolveExcelCellText(cellType, cellValue, inlineBuilder.String(), sharedStrings)
				var cellLinks []ExtractedLink
				if cellFormula != "" {
					if m := excelHyperlinkFnRegex.FindStringSubmatch(cellFormula); len(m) >= 2 {
						u := strings.TrimSpace(m[1])
						lbl := ""
						if len(m) >= 3 {
							lbl = strings.TrimSpace(m[2])
						}
						if lbl == "" {
							lbl = resolvedText
						}
						if resolvedText == "" {
							resolvedText = lbl
						}
						cellLinks = append(cellLinks, ExtractedLink{Label: lbl, URL: u})
					}
				}

				var cellImages []ExtractedImage
				if cellVM > 0 {
					if img, found := richValueImages[cellVM]; found && len(img.Data) > 0 {
						cellImages = append(cellImages, img)
					}
				}

				indent := 0
				if cellStyleIdx >= 0 && cellStyleIdx < len(styleIndents) {
					indent = styleIndents[cellStyleIdx]
				}

				if resolvedText != "" || len(cellLinks) > 0 || len(cellImages) > 0 {
					rowObj := getOrCreateRow(currentRowNum)
					rowObj.cells = append(rowObj.cells, xlsxCellData{
						col:    currentColIdx,
						text:   resolvedText,
						indent: indent,
						links:  cellLinks,
						images: cellImages,
					})
				}
				currentColIdx++
			}
		}
	}

	for _, dRelID := range drawingRelIDs {
		rel, ok := sheetRels[dRelID]
		if !ok {
			continue
		}
		drawingPath := resolveZipRelativePath(sheetDir, rel.Target)
		parseXlsxDrawing(drawingPath, filesByName, getOrCreateRow)
	}

	return convertXlsxRowsToExtractedLines(rowsByNum), nil
}

func parseXlsxDrawing(
	drawingPath string,
	filesByName map[string]*zip.File,
	getOrCreateRow func(int) *xlsxRowData,
) {
	zf, ok := filesByName[drawingPath]
	if !ok {
		return
	}
	drawingDir := path.Dir(drawingPath)
	drawingBase := path.Base(drawingPath)
	drawingRels := loadGenericRels(filesByName, path.Join(drawingDir, "_rels", drawingBase+".rels"))

	rc, err := zf.Open()
	if err != nil {
		return
	}
	defer rc.Close()

	decoder := xml.NewDecoder(io.LimitReader(rc, 20<<20))
	inFrom := false
	inRowTag := false
	anchorRow := 1
	caption := ""
	var anchorImages []ExtractedImage
	var anchorLinks []ExtractedLink

	flushAnchor := func() {
		if len(anchorImages) == 0 && len(anchorLinks) == 0 {
			return
		}
		rowObj := getOrCreateRow(anchorRow)
		rowObj.images = append(rowObj.images, anchorImages...)
		rowObj.links = append(rowObj.links, anchorLinks...)
		anchorImages = nil
		anchorLinks = nil
		caption = ""
	}

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "twoCellAnchor", "oneCellAnchor", "absoluteAnchor":
				anchorRow = 1
				caption = ""
				anchorImages = nil
				anchorLinks = nil
			case "from":
				inFrom = true
			case "row":
				if inFrom {
					inRowTag = true
				}
			case "cNvPr":
				descr := strings.TrimSpace(getXmlAttr(el, "descr", "title", "name"))
				if descr != "" {
					caption = descr
				}
			case "hlinkClick":
				rID := getXmlAttr(el, "id")
				if rel, found := drawingRels[rID]; found {
					u := strings.TrimSpace(rel.Target)
					if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
						anchorLinks = append(anchorLinks, ExtractedLink{Label: caption, URL: u})
					}
				}
			case "blip", "imagedata":
				rID := getXmlAttr(el, "embed", "link", "id")
				if rel, found := drawingRels[rID]; found {
					mediaPath := resolveZipRelativePath(drawingDir, rel.Target)
					if img, ok := readZipMediaImage(filesByName, mediaPath, caption); ok {
						anchorImages = append(anchorImages, img)
					}
				}
			}
		case xml.CharData:
			if inFrom && inRowTag {
				if v, err := strconv.Atoi(strings.TrimSpace(string(el))); err == nil && v >= 0 {
					anchorRow = v + 1
				}
			}
		case xml.EndElement:
			switch el.Name.Local {
			case "row":
				inRowTag = false
			case "from":
				inFrom = false
			case "twoCellAnchor", "oneCellAnchor", "absoluteAnchor":
				flushAnchor()
			}
		}
	}
}

func convertXlsxRowsToExtractedLines(rowsByNum map[int]*xlsxRowData) []ExtractedLine {
	if len(rowsByNum) == 0 {
		return nil
	}

	var rowNums []int
	for num := range rowsByNum {
		rowNums = append(rowNums, num)
	}
	sort.Ints(rowNums)

	baseTextCol := -1
	for idx, rNum := range rowNums {
		r := rowsByNum[rNum]
		sort.Slice(r.cells, func(i, j int) bool {
			return r.cells[i].col < r.cells[j].col
		})
		if idx == 0 && isLikelyXlsxHeaderRow(r) {
			continue
		}
		for _, c := range r.cells {
			t := strings.TrimSpace(c.text)
			if t == "" || pureStepNumberRegex.MatchString(t) {
				continue
			}
			if baseTextCol == -1 || c.col < baseTextCol {
				baseTextCol = c.col
			}
			break
		}
	}
	if baseTextCol < 0 {
		baseTextCol = 0
	}

	var lines []ExtractedLine

	for idx, rNum := range rowNums {
		r := rowsByNum[rNum]
		if idx == 0 && isLikelyXlsxHeaderRow(r) {
			continue
		}

		var textParts []string
		var rowLinks []ExtractedLink
		var rowImages []ExtractedImage
		inferredLevel := 0
		hasExplicitLevel := false
		firstTextCol := -1

		for cellIdx, c := range r.cells {
			for _, l := range c.links {
				rowLinks = append(rowLinks, l)
			}
			rowImages = append(rowImages, c.images...)

			rawCell := strings.TrimSpace(c.text)
			if rawCell == "" {
				continue
			}

			if cellIdx == 0 && pureStepNumberRegex.MatchString(rawCell) {
				m := pureStepNumberRegex.FindStringSubmatch(rawCell)
				if len(m) >= 2 {
					dots := strings.Count(m[1], ".")
					if dots > 0 {
						inferredLevel = dots
						hasExplicitLevel = true
					}
				}
				continue
			}

			if firstTextCol == -1 {
				firstTextCol = c.col
				if c.indent > 0 && !hasExplicitLevel {
					inferredLevel = c.indent
					hasExplicitLevel = true
				}
				leadingSpaces := countLeadingIndentationSpaces(c.text)
				if leadingSpaces >= 2 && !hasExplicitLevel {
					inferredLevel = leadingSpaces / 2
					hasExplicitLevel = true
				}
			}

			cellTextFormatted := rawCell
			if len(c.links) == 1 && c.links[0].URL != "" && !strings.Contains(rawCell, c.links[0].URL) {
				cellTextFormatted = fmt.Sprintf("[%s](%s)", rawCell, c.links[0].URL)
			}
			textParts = append(textParts, cellTextFormatted)
		}

		rowLinks = append(rowLinks, r.links...)
		rowImages = append(rowImages, r.images...)

		if !hasExplicitLevel && firstTextCol > baseTextCol {
			colDiff := firstTextCol - baseTextCol
			if colDiff > 4 {
				colDiff = 4
			}
			inferredLevel = colDiff
		}

		joinedText := strings.TrimSpace(strings.Join(textParts, " "))
		if joinedText == "" && len(rowLinks) == 0 && len(rowImages) == 0 {
			continue
		}

		lvl, cleanedText := normalizeOutlineAndBullet(inferredLevel, joinedText)
		lines = appendOrMergeExtractedLine(lines, ExtractedLine{
			Level:  lvl,
			Text:   cleanedText,
			Links:  rowLinks,
			Images: rowImages,
		})
	}

	return lines
}

func isLikelyXlsxHeaderRow(r *xlsxRowData) bool {
	if len(r.images) > 0 || len(r.links) > 0 {
		return false
	}
	var parts []string
	for _, c := range r.cells {
		if len(c.links) > 0 || len(c.images) > 0 {
			return false
		}
		if t := strings.TrimSpace(c.text); t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return false
	}
	joined := strings.Join(parts, " ")
	return commonHeaderRowRegex.MatchString(joined)
}

func resolveExcelCellText(cellType, rawVal, inlineStr string, sharedStrings []string) string {
	switch cellType {
	case "s":
		idx, err := strconv.Atoi(strings.TrimSpace(rawVal))
		if err == nil && idx >= 0 && idx < len(sharedStrings) {
			return sharedStrings[idx]
		}
		return ""
	case "inlineStr":
		return inlineStr
	case "e":
		return ""
	default:
		if inlineStr != "" {
			return inlineStr
		}
		return rawVal
	}
}

func parseExcelCellRef(ref string) (col int, row int, ok bool) {
	m := excelCellRefRegex.FindStringSubmatch(strings.TrimSpace(ref))
	if len(m) < 3 {
		return 0, 0, false
	}
	colLetters := strings.ToUpper(m[1])
	colVal := 0
	for _, ch := range colLetters {
		colVal = colVal*26 + int(ch-'A'+1)
	}
	rowVal, err := strconv.Atoi(m[2])
	if err != nil {
		return 0, 0, false
	}
	return colVal - 1, rowVal, true
}

func countLeadingIndentationSpaces(s string) int {
	spaces := 0
	for _, ch := range s {
		if ch == ' ' {
			spaces++
		} else if ch == '\t' {
			spaces += 4
		} else {
			break
		}
	}
	return spaces
}
