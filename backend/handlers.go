package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ApiHandler struct {
	repo     *PlaybookRepository
	toolRepo *ToolRepository
}

func NewApiHandler(repo *PlaybookRepository, toolRepo *ToolRepository) *ApiHandler {
	return &ApiHandler{repo: repo, toolRepo: toolRepo}
}

func (h *ApiHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/api/config", h.GetRuntimeConfig)
	r.POST("/api/generate", h.GenerateFlowchart)
	r.GET("/api/sops", h.ListPlaybooks)
	r.POST("/api/sops", h.SavePlaybook)
	r.DELETE("/api/sops/:id", h.DeletePlaybook)
	r.GET("/api/tools", h.ListTools)
	r.POST("/api/tools", h.CreateTool)
	r.POST("/api/tools/test", h.TestToolDraft)
	r.PUT("/api/tools/:id", h.UpdateTool)
	r.DELETE("/api/tools/:id", h.DeleteTool)
	r.POST("/api/execute", h.ExecuteStep)
	r.POST("/api/export/bpmn", h.ExportBpmn)
	r.POST("/api/export/cacao", h.ExportCacao)
	r.POST("/api/export/markdown", h.ExportMarkdown)
}

func (h *ApiHandler) GetRuntimeConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"allow_live_execution": Config.AllowLiveExecution,
	})
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
		data, err := io.ReadAll(io.LimitReader(f, 10<<20))
		if err != nil {
			fail(http.StatusBadRequest, "อ่านไฟล์ไม่สำเร็จ")
			return
		}
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

	tools := h.loadEnabledTools(c.Request.Context())
	steps, engine := ConvertLinesToSteps(lines, tools)
	nodes, edges := BuildCustomFlowchart(steps, tools)
	c.JSON(http.StatusOK, gin.H{"nodes": nodes, "edges": edges, "engine": engine, "lines": lines})
}

func (h *ApiHandler) loadEnabledTools(parentCtx context.Context) []ToolDefinition {
	if h.toolRepo == nil {
		return DefaultBuiltinTools()
	}
	ctx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
	defer cancel()
	tools, err := h.toolRepo.ListEnabled(ctx)
	if err != nil {
		return DefaultBuiltinTools()
	}
	return tools
}

func (h *ApiHandler) loadToolMap(parentCtx context.Context) map[string]ToolDefinition {
	result := make(map[string]ToolDefinition)
	if h.toolRepo == nil {
		for _, t := range DefaultBuiltinTools() {
			result[t.ID] = t
		}
		return result
	}
	ctx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
	defer cancel()
	tools, err := h.toolRepo.ListAll(ctx)
	if err != nil {
		for _, t := range DefaultBuiltinTools() {
			result[t.ID] = t
		}
		return result
	}
	for _, t := range tools {
		result[t.ID] = t
	}
	return result
}

func (h *ApiHandler) ListTools(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	items, err := h.toolRepo.ListAll(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ดึงรายการ Tools จาก MongoDB ไม่สำเร็จ"})
		return
	}
	for i := range items {
		PopulateToolAuthStatus(&items[i])
	}
	c.JSON(http.StatusOK, items)
}

func (h *ApiHandler) CreateTool(c *gin.Context) {
	var input ToolDefinition
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูล Tool ไม่ถูกต้อง"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	created, err := h.toolRepo.Create(ctx, input)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidTool):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, ErrToolAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"error": "Tool ID นี้มีอยู่แล้ว"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "สร้าง Tool ไม่สำเร็จ"})
		}
		return
	}
	PopulateToolAuthStatus(&created)
	c.JSON(http.StatusCreated, created)
}

func (h *ApiHandler) UpdateTool(c *gin.Context) {
	var input ToolDefinition
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูล Tool ไม่ถูกต้อง"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	updated, err := h.toolRepo.Update(ctx, c.Param("id"), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrToolNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบ Tool นี้"})
		case errors.Is(err, ErrInvalidTool):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "อัปเดต Tool ไม่สำเร็จ"})
		}
		return
	}
	PopulateToolAuthStatus(&updated)
	c.JSON(http.StatusOK, updated)
}

func (h *ApiHandler) DeleteTool(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	id := c.Param("id")
	if err := h.toolRepo.Delete(ctx, id); err != nil {
		switch {
		case errors.Is(err, ErrToolNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบ Tool นี้"})
		case errors.Is(err, ErrBuiltinToolProtected):
			c.JSON(http.StatusBadRequest, gin.H{"error": "ไม่สามารถลบ Built-in Tool ได้ (สามารถปิดการใช้งานแทนได้)"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ลบ Tool ไม่สำเร็จ"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

type ToolTestRequest struct {
	Tool           ToolDefinition `json:"tool"`
	RuntimeContext map[string]any `json:"runtime_context,omitempty"`
	Label          string         `json:"label,omitempty"`
}

func (h *ApiHandler) TestToolDraft(c *gin.Context) {
	var req ToolTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลทดสอบ Tool ไม่ถูกต้อง"})
		return
	}
	normalized, err := NormalizeAndValidateTool(req.Tool, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		label = "Dry Run: " + normalized.Name
	}
	execReq := ExecutionRequest{
		NodeID:         "dry_run",
		Type:           normalized.Category,
		Label:          label,
		Tool:           normalized.ID,
		RuntimeContext: req.RuntimeContext,
	}
	built := BuildToolRequest(&normalized, execReq)
	result := RunSandboxStep(execReq, &normalized)
	c.JSON(http.StatusOK, gin.H{
		"request": built,
		"result":  result,
	})
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
	toolMap := h.loadToolMap(c.Request.Context())
	doc = SanitizePlaybookForSave(doc, toolMap)
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
		if errors.Is(err, ErrPlaybookNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบ Playbook นี้"})
			return
		}
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
	req.Tool = NormalizeLegacyToolID(req.Tool)
	var toolDef *ToolDefinition
	if h.toolRepo != nil && req.Tool != "" && req.Tool != "none" {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		if def, err := h.toolRepo.GetByID(ctx, req.Tool); err == nil {
			toolDef = def
		}
	}
	if strings.EqualFold(strings.TrimSpace(req.ExecutionMode), "live") {
		c.JSON(http.StatusOK, RunLiveHTTPStep(req, toolDef))
		return
	}
	c.JSON(http.StatusOK, RunSandboxStep(req, toolDef))
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
	toolMap := h.loadToolMap(c.Request.Context())
	data, err := BuildCacaoJSON(req.Title, req.Nodes, req.Edges, toolMap)
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
