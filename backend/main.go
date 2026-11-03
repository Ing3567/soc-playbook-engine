package main

import (
    "fmt"
    "io"
    "net/http"
    "path/filepath"
    "strings"

    "github.com/gin-gonic/gin"
)

type Node struct {
    ID    string `json:"id"`
    Type  string `json:"type"` 
    Label string `json:"label"`
}

type Edge struct {
    ID     string `json:"id"`
    Source string `json:"source"`
    Target string `json:"target"`
    Label  string `json:"label,omitempty"`
}

type builder struct {
    nodes []Node
    edges []Edge
}

var validType = map[string]bool{"lookup": true, "decision": true, "action": true, "end": true}

func (b *builder) add(t, label string) string {
    id := fmt.Sprintf("n%d", len(b.nodes)+1)
    b.nodes = append(b.nodes, Node{ID: id, Type: t, Label: label})
    return id
}


func (b *builder) link(from, to, label string) {
    if to == "" {
        return
    }
    for i := range b.edges {
        e := &b.edges[i]
        if e.Source == from && e.Target == to {
            if label != "" {
                if e.Label == "" {
                    e.Label = label
                } else {
                    e.Label += " / " + label
                }
            }
            return
        }
    }
    b.edges = append(b.edges, Edge{ID: fmt.Sprintf("e%d", len(b.edges)+1), Source: from, Target: to, Label: label})
}


func (b *builder) seq(steps []Step, next string) string {
    for i, s := range steps { 
        if s.Type == "end" {
            steps = steps[:i+1]
            break
        }
    }
    if len(steps) == 0 {
        if next != "" {
            return next
        }
        return b.add("terminal", "จบ") 
    }
    entry := next
    for i := len(steps) - 1; i >= 0; i-- {
        s := steps[i]
        if !validType[s.Type] {
            s.Type = "action"
        }
        if s.Type == "decision" && len(s.Branches) == 0 {
            s.Type = "action"
        }
        id := b.add(s.Type, s.Label)
        switch s.Type {
        case "decision":
            for _, br := range s.Branches {
                b.link(id, b.seq(br.Steps, entry), br.Label)
            }
        case "end":
        default:
            b.link(id, entry, "")
        }
        entry = id
    }
    return entry
}

func buildFlow(steps []Step) ([]Node, []Edge) {
    b := &builder{}
    start := b.add("terminal", "เริ่ม")
    b.link(start, b.seq(steps, ""), "")
    return b.nodes, b.edges
}

func main() {
    r := gin.Default()

    r.POST("/api/generate", func(c *gin.Context) {
        fail := func(code int, msg string) { c.JSON(code, gin.H{"error": msg}) }
        var lines []Line

        if fh, err := c.FormFile("file"); err == nil { 
            switch strings.ToLower(filepath.Ext(fh.Filename)) {
            case ".docx":
            case ".doc":
                fail(http.StatusBadRequest, "ไฟล์ .doc รุ่นเก่าไม่รองรับ กรุณา Save As เป็น .docx ก่อน")
                return
            default:
                fail(http.StatusBadRequest, "รองรับเฉพาะไฟล์ Word (.docx) เท่านั้น")
                return
            }
            if fh.Size > 10<<20 {
                fail(http.StatusRequestEntityTooLarge, "ไฟล์ใหญ่เกิน 10MB")
                return
            }
            f, err := fh.Open()
            if err != nil {
                fail(http.StatusBadRequest, "เปิดไฟล์ไม่ได้")
                return
            }
            defer f.Close()
            data, _ := io.ReadAll(io.LimitReader(f, 10<<20))
            if lines, err = docxLines(data); err != nil {
                fail(http.StatusBadRequest, "ไฟล์ไม่ใช่ Word (.docx) ที่ถูกต้อง")
                return
            }
        } else if t := strings.TrimSpace(c.PostForm("text")); t != "" { 
            if len(t) > 50000 {
                fail(http.StatusRequestEntityTooLarge, "ข้อความยาวเกิน 50,000 ตัวอักษร")
                return
            }
            lines = textLines(t)
        } else {
            fail(http.StatusBadRequest, "กรุณาพิมพ์ SOP หรืออัปโหลดไฟล์ Word (.docx)")
            return
        }

        if len(lines) == 0 {
            fail(http.StatusBadRequest, "ไม่พบข้อความ SOP")
            return
        }

        steps, engine := toSteps(lines)
        nodes, edges := buildFlow(steps)
        c.JSON(http.StatusOK, gin.H{"nodes": nodes, "edges": edges, "engine": engine, "lines": lines})
    })

    r.Run(":8080")
}