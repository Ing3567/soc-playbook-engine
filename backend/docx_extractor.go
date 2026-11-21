package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var listBulletPrefix = regexp.MustCompile(`^\s*(\d+[\.\)]|[-*•●○▪])\s*`)

func parseXmlIntAttribute(element xml.StartElement, name string) int {
	for _, attr := range element.Attr {
		if attr.Name.Local == name {
			v, _ := strconv.Atoi(attr.Value)
			return v
		}
	}
	return 0
}

func ExtractLinesFromDocx(data []byte) ([]Line, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("not a docx")
	}
	var docFile *zip.File
	for _, f := range zipReader.File {
		if f.Name == "word/document.xml" {
			docFile = f
		}
	}
	if docFile == nil {
		return nil, errors.New("word/document.xml not found")
	}
	reader, err := docFile.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	decoder := xml.NewDecoder(io.LimitReader(reader, 20<<20))
	var lines []Line
	var textBuffer strings.Builder
	level, hasListLevel, inTextTag := 0, false, false

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				textBuffer.Reset()
				level, hasListLevel = 0, false
			case "ilvl":
				level, hasListLevel = parseXmlIntAttribute(t, "val"), true
			case "ind":
				if !hasListLevel {
					left := parseXmlIntAttribute(t, "left")
					if s := parseXmlIntAttribute(t, "start"); s > left {
						left = s
					}
					level = left / 360
				}
			case "t":
				inTextTag = true
			}
		case xml.CharData:
			if inTextTag {
				textBuffer.Write(t)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inTextTag = false
			case "p":
				cleaned := strings.TrimSpace(listBulletPrefix.ReplaceAllString(textBuffer.String(), ""))
				if cleaned != "" {
					lines = append(lines, Line{Level: level, Text: cleaned})
				}
			}
		}
	}
	return lines, nil
}

func ExtractLinesFromText(raw string) []Line {
	var lines []Line
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
		cleaned := strings.TrimSpace(listBulletPrefix.ReplaceAllString(strings.TrimSpace(rawLine), ""))
		if cleaned != "" {
			lines = append(lines, Line{Level: spaces / 2, Text: cleaned})
		}
	}
	return lines
}
