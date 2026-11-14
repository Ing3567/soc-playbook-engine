package main

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ApiHandler struct {
	repo *PlaybookRepository
}

func NewApiHandler(repo *PlaybookRepository) *ApiHandler {
	return &ApiHandler{repo: repo}
}

func (h *ApiHandler) RegisterRoutes(r *gin.Engine) {
	r.POST("/api/generate", h.GenerateFlowchart)
	r.GET("/api/sops", h.ListPlaybooks)
	r.POST("/api/sops", h.SavePlaybook)
	r.DELETE("/api/sops/:id", h.DeletePlaybook)
	r.POST("/api/execute", h.ExecuteStep)
	r.POST("/api/export/bpmn", h.ExportBpmn)
	r.POST("/api/export/cacao", h.ExportCacao)
	r.POST("/api/export/markdown", h.ExportMarkdown)
}

func (h *ApiHandler) GenerateFlowchart(c *gin.Context) {
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
		if lines, err = ExtractLinesFromDocx(data); err != nil {
			fail(http.StatusBadRequest, "ไฟล์ไม่ใช่ Word (.docx) ที่ถูกต้อง")
			return
		}
	} else if t := strings.TrimSpace(c.PostForm("text")); t != "" {
		if len(t) > 50000 {
			fail(http.StatusRequestEntityTooLarge, "ข้อความยาวเกิน 50,000 ตัวอักษร")
			return
		}
		lines = ExtractLinesFromText(t)
	} else {
		fail(http.StatusBadRequest, "กรุณาพิมพ์ SOP หรืออัปโหลดไฟล์ Word (.docx)")
		return
	}

	if len(lines) == 0 {
		fail(http.StatusBadRequest, "ไม่พบข้อความ SOP")
		return
	}

	steps, engine := ConvertLinesToSteps(lines)
	nodes, edges := BuildCustomFlowchart(steps)
	c.JSON(http.StatusOK, gin.H{"nodes": nodes, "edges": edges, "engine": engine, "lines": lines})
}

func (h *ApiHandler) ListPlaybooks(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	items, err := h.repo.ListAll(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ดึงข้อมูลจาก MongoDB ไม่สำเร็จ"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *ApiHandler) SavePlaybook(c *gin.Context) {
	var doc PlaybookDocument
	if err := c.ShouldBindJSON(&doc); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลไม่ถูกต้อง"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	saved, err := h.repo.SaveOne(ctx, doc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "บันทึกลง MongoDB ไม่สำเร็จ"})
		return
	}
	c.JSON(http.StatusOK, saved)
}

func (h *ApiHandler) DeletePlaybook(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	id := c.Param("id")
	if err := h.repo.DeleteOne(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ลบไม่สำเร็จ"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (h *ApiHandler) ExecuteStep(c *gin.Context) {
	var req ExecutionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "คำสั่งรันไม่ถูกต้อง"})
		return
	}
	c.JSON(http.StatusOK, RunSandboxStep(req))
}

func (h *ApiHandler) ExportBpmn(c *gin.Context) {
	var req ExportRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Nodes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่มีข้อมูลกราฟ"})
		return
	}
	data, err := BuildBpmnXML(req.Title, req.Nodes, req.Edges)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "แปลง BPMN ไม่สำเร็จ"})
		return
	}
	c.Data(http.StatusOK, "application/xml; charset=utf-8", data)
}

func (h *ApiHandler) ExportCacao(c *gin.Context) {
	var req ExportRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Nodes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่มีข้อมูลกราฟ"})
		return
	}
	data, err := BuildCacaoJSON(req.Title, req.Nodes, req.Edges)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "แปลง CACAO ไม่สำเร็จ"})
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", data)
}

func (h *ApiHandler) ExportMarkdown(c *gin.Context) {
	var req ExportRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Nodes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่มีข้อมูลกราฟ"})
		return
	}
	data := BuildMarkdownMermaid(req.Title, req.Nodes, req.Edges)
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", data)
}