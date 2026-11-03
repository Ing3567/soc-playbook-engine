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

type Line struct {
    Level int    `json:"level"`
    Text  string `json:"text"`
}

var bullet = regexp.MustCompile(`^\s*(\d+[\.\)]|[-*•●○▪])\s*`)

func attrInt(e xml.StartElement, name string) int {
    for _, a := range e.Attr {
        if a.Name.Local == name {
            n, _ := strconv.Atoi(a.Value)
            return n
        }
    }
    return 0
}


func docxLines(data []byte) ([]Line, error) {
    zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
    if err != nil {
        return nil, errors.New("not a docx")
    }
    var doc *zip.File
    for _, f := range zr.File {
        if f.Name == "word/document.xml" {
            doc = f
        }
    }
    if doc == nil {
        return nil, errors.New("word/document.xml not found")
    }
    rc, err := doc.Open()
    if err != nil {
        return nil, err
    }
    defer rc.Close()

    dec := xml.NewDecoder(io.LimitReader(rc, 20<<20)) 
    var lines []Line
    var cur strings.Builder
    level, hasIlvl, inText := 0, false, false

    for {
        tok, err := dec.Token()
        if err == io.EOF {
            break
        }
        if err != nil {
            return nil, err
        }
        switch t := tok.(type) {
        case xml.StartElement:
            switch t.Name.Local {
            case "p":
                cur.Reset()
                level, hasIlvl = 0, false
            case "ilvl": 
                level, hasIlvl = attrInt(t, "val"), true
            case "ind": 
                if !hasIlvl {
                    l := attrInt(t, "left")
                    if s := attrInt(t, "start"); s > l {
                        l = s
                    }
                    level = l / 360
                }
            case "t":
                inText = true
            }
        case xml.CharData:
            if inText {
                cur.Write(t)
            }
        case xml.EndElement:
            switch t.Name.Local {
            case "t":
                inText = false
            case "p":
                text := strings.TrimSpace(bullet.ReplaceAllString(cur.String(), ""))
                if text != "" {
                    lines = append(lines, Line{Level: level, Text: text})
                }
            }
        }
    }
    return lines, nil
}