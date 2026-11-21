package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

type docxRelationships struct {
	Relationships []docxRelationship `xml:"Relationship"`
}

type docxRelationship struct {
	ID         string `xml:"Id,attr"`
	Target     string `xml:"Target,attr"`
	TargetMode string `xml:"TargetMode,attr"`
}

func ExtractDocxText(data []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}

	hyperlinkTargets := loadHyperlinkTargets(reader.File)

	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			rc, openErr := file.Open()
			if openErr != nil {
				return "", openErr
			}
			defer rc.Close()
			return parseDocumentXML(rc, hyperlinkTargets)
		}
	}

	return "", errors.New("word/document.xml not found in docx")
}

func loadHyperlinkTargets(files []*zip.File) map[string]string {
	targets := make(map[string]string)

	for _, file := range files {
		if file.Name != "word/_rels/document.xml.rels" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return targets
		}
		defer rc.Close()

		var rels docxRelationships
		if decodeErr := xml.NewDecoder(rc).Decode(&rels); decodeErr != nil {
			return targets
		}

		for _, rel := range rels.Relationships {
			trimmed := strings.TrimSpace(rel.Target)
			if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
				targets[rel.ID] = trimmed
			}
		}
	}

	return targets
}

func parseDocumentXML(reader io.Reader, hyperlinkTargets map[string]string) (string, error) {
	decoder := xml.NewDecoder(reader)
	var paragraphs []string
	var paragraphBuilder strings.Builder
	var hyperlinkBuilder strings.Builder
	inHyperlink := false
	activeRelID := ""

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		switch element := tok.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "hyperlink":
				inHyperlink = true
				activeRelID = ""
				hyperlinkBuilder.Reset()
				for _, attr := range element.Attr {
					if attr.Name.Local == "id" {
						activeRelID = attr.Value
						break
					}
				}
			case "t":
				var text string
				if decodeErr := decoder.DecodeElement(&text, &element); decodeErr != nil {
					return "", decodeErr
				}
				if inHyperlink {
					hyperlinkBuilder.WriteString(text)
				} else {
					paragraphBuilder.WriteString(text)
				}
			case "br", "cr":
				if inHyperlink {
					hyperlinkBuilder.WriteString(" ")
				} else {
					paragraphBuilder.WriteString("\n")
				}
			case "tab":
				if !inHyperlink {
					paragraphBuilder.WriteString("\t")
				}
			}

		case xml.EndElement:
			switch element.Name.Local {
			case "hyperlink":
				label := strings.TrimSpace(hyperlinkBuilder.String())
				targetURL := hyperlinkTargets[activeRelID]
				if targetURL != "" {
					if label != "" && !strings.Contains(label, targetURL) {
						paragraphBuilder.WriteString(fmt.Sprintf("%s %s", label, targetURL))
					} else {
						paragraphBuilder.WriteString(targetURL)
					}
				} else if label != "" {
					paragraphBuilder.WriteString(label)
				}
				inHyperlink = false
				activeRelID = ""
				hyperlinkBuilder.Reset()

			case "p":
				line := strings.TrimSpace(paragraphBuilder.String())
				if line != "" {
					paragraphs = append(paragraphs, line)
				}
				paragraphBuilder.Reset()
			}
		}
	}

	return strings.Join(paragraphs, "\n"), nil
}