package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name,omitempty"`
	Role        string `json:"role,omitempty"`
}

func (h *ApiHandler) Login(c *gin.Context) {
	if h.userRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ระบบผู้ใช้ยังไม่พร้อมใช้งาน"})
		return
	}
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลเข้าสู่ระบบไม่ถูกต้อง"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	user, err := h.userRepo.FindByUsername(ctx, req.Username)
	if err != nil || !VerifyPassword(req.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง"})
		return
	}
	claims := AuthClaims{
		UserID:      user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        user.Role,
	}
	token, err := SignAuthToken(claims, ActiveAuthSecret(), defaultTokenTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "สร้าง token ไม่สำเร็จ"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  user,
	})
}

func (h *ApiHandler) Register(c *gin.Context) {
	if h.userRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ระบบผู้ใช้ยังไม่พร้อมใช้งาน"})
		return
	}
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลสมัครสมาชิกไม่ถูกต้อง"})
		return
	}

	role := "trainee"
	if strings.EqualFold(strings.TrimSpace(req.Role), "instructor") {
		if caller, ok := GetAuthClaims(c); ok && caller.Role == "instructor" {
			role = "instructor"
		} else {
			c.JSON(http.StatusForbidden, gin.H{"error": "เฉพาะ Instructor เท่านั้นที่สร้างบัญชี Instructor ได้"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	created, err := h.userRepo.Create(ctx, req.Username, req.Password, req.DisplayName, role)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidUser):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, ErrUserAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"error": "ชื่อผู้ใช้นี้ถูกใช้ไปแล้ว"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "สร้างบัญชีไม่สำเร็จ"})
		}
		return
	}
	claims := AuthClaims{
		UserID:      created.ID,
		Username:    created.Username,
		DisplayName: created.DisplayName,
		Role:        created.Role,
	}
	token, err := SignAuthToken(claims, ActiveAuthSecret(), defaultTokenTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "สร้าง token ไม่สำเร็จ"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"token": token,
		"user":  created,
	})
}

func (h *ApiHandler) GetMe(c *gin.Context) {
	claims, ok := GetAuthClaims(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ไม่ได้เข้าสู่ระบบ"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":           claims.UserID,
		"username":     claims.Username,
		"display_name": claims.DisplayName,
		"role":         claims.Role,
		"auth_enabled": Config.AuthEnabled,
	})
}

func (h *ApiHandler) loadScenarioByID(parentCtx context.Context, id string) *Scenario {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	if h.scenarioRepo != nil {
		ctx, cancel := context.WithTimeout(parentCtx, 3*time.Second)
		defer cancel()
		if s, err := h.scenarioRepo.GetByID(ctx, id); err == nil {
			return s
		}
	}
	return FindBuiltinScenario(id)
}

func (h *ApiHandler) StartRun(c *gin.Context) {
	if h.runRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Run repository ยังไม่พร้อมใช้งาน"})
		return
	}
	var req StartRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลเริ่มรอบฝึกไม่ถูกต้อง"})
		return
	}

	claims, _ := GetAuthClaims(c)
	if claims.UserID == "" {
		claims = defaultDevClaims
	}

	mode := "sandbox"
	if strings.EqualFold(strings.TrimSpace(req.Mode), "live") {
		if Config.AuthEnabled && claims.Role != "instructor" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Live API execution อนุญาตเฉพาะ Instructor เท่านั้น"})
			return
		}
		if Config.AllowLiveExecution {
			mode = "live"
		}
	}

	nodes := req.Nodes
	edges := req.Edges
	playbookID := strings.TrimSpace(req.PlaybookID)
	playbookVersion := req.PlaybookVersion
	playbookTitle := strings.TrimSpace(req.PlaybookTitle)

	if len(nodes) == 0 && playbookID != "" && h.repo != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		pb, err := h.repo.GetByID(ctx, playbookID)
		if err != nil {
			if errors.Is(err, ErrPlaybookNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบ Playbook ที่ระบุ"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลด Playbook ไม่สำเร็จ"})
			return
		}
		nodes = pb.Nodes
		edges = pb.Edges
		playbookVersion = pb.Version
		if playbookTitle == "" {
			playbookTitle = pb.Title
		}
	}

	if len(nodes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาสร้างหรือเลือก Playbook ก่อนเริ่มรอบฝึก"})
		return
	}
	if playbookTitle == "" {
		playbookTitle = "Untitled SOP"
	}

	scenario := h.loadScenarioByID(c.Request.Context(), req.ScenarioID)
	alert := map[string]any{}
	scenarioTitle := ""
	if scenario != nil {
		scenarioTitle = scenario.Title
		for k, v := range scenario.Alert {
			alert[k] = v
		}
	}
	for k, v := range req.Alert {
		alert[k] = v
	}

	now := time.Now()
	traineeName := claims.DisplayName
	if traineeName == "" {
		traineeName = claims.Username
	}
	run := SimulationRun{
		ID:              GeneratePrefixedID("run"),
		ScenarioID:      strings.TrimSpace(req.ScenarioID),
		ScenarioTitle:   scenarioTitle,
		PlaybookID:      playbookID,
		PlaybookVersion: playbookVersion,
		PlaybookTitle:   playbookTitle,
		Nodes:           nodes,
		Edges:           edges,
		Alert:           alert,
		TraineeID:       claims.UserID,
		TraineeName:     traineeName,
		Mode:            mode,
		Status:          "in_progress",
		NodeStatuses:    map[string]string{},
		NodeOutputs:     map[string]map[string]any{},
		StartedAt:       now,
		UpdatedAt:       now,
	}

	graphState := ComputeRunGraphState(run.Nodes, run.Edges, run.NodeStatuses, run.NodeOutputs, run.Alert)
	run.TraversedEdges = graphState.TraversedEdges

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	created, err := h.runRepo.CreateRun(ctx, run)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "เริ่มรอบฝึกไม่สำเร็จ"})
		return
	}

	startEvent, _ := h.runRepo.AppendEvent(ctx, RunEvent{
		RunID:     created.ID,
		Action:    "run_started",
		Status:    "in_progress",
		Input:     map[string]any{"scenario_id": created.ScenarioID, "playbook_title": created.PlaybookTitle, "mode": created.Mode},
		Timestamp: now,
	})

	c.JSON(http.StatusCreated, gin.H{
		"run":         created,
		"events":      []RunEvent{startEvent},
		"graph_state": graphState,
	})
}

func (h *ApiHandler) ListRuns(c *gin.Context) {
	if h.runRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Run repository ยังไม่พร้อมใช้งาน"})
		return
	}
	claims, _ := GetAuthClaims(c)
	traineeFilter := ""
	if claims.Role != "instructor" {
		traineeFilter = claims.UserID
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	items, err := h.runRepo.ListRuns(ctx, traineeFilter, 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ดึงรายการรอบฝึกไม่สำเร็จ"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *ApiHandler) GetRun(c *gin.Context) {
	if h.runRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Run repository ยังไม่พร้อมใช้งาน"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	run, err := h.runRepo.GetRunByID(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, ErrRunNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบรอบฝึกนี้"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดรอบฝึกไม่สำเร็จ"})
		return
	}
	claims, _ := GetAuthClaims(c)
	if claims.Role != "instructor" && run.TraineeID != claims.UserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "ไม่มีสิทธิ์เข้าถึงรอบฝึกของผู้อื่น"})
		return
	}
	events, err := h.runRepo.ListEventsByRun(ctx, run.ID)
	if err != nil {
		events = []RunEvent{}
	}
	graphState := ComputeRunGraphState(run.Nodes, run.Edges, run.NodeStatuses, run.NodeOutputs, run.Alert)
	c.JSON(http.StatusOK, gin.H{
		"run":         run,
		"events":      events,
		"graph_state": graphState,
	})
}

func (h *ApiHandler) ExecuteRunStep(c *gin.Context) {
	if h.runRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Run repository ยังไม่พร้อมใช้งาน"})
		return
	}
	var stepReq RunStepRequest
	if err := c.ShouldBindJSON(&stepReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลขั้นตอนไม่ถูกต้อง"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	run, err := h.runRepo.GetRunByID(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, ErrRunNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบรอบฝึกนี้"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดรอบฝึกไม่สำเร็จ"})
		return
	}
	claims, _ := GetAuthClaims(c)
	if claims.Role != "instructor" && run.TraineeID != claims.UserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "ไม่มีสิทธิ์ดำเนินการในรอบฝึกของผู้อื่น"})
		return
	}

	nodeID := c.Param("nodeId")
	var toolDef *ToolDefinition
	for _, n := range run.Nodes {
		if n.ID == nodeID {
			toolID := NormalizeLegacyToolID(n.Tool)
			if h.toolRepo != nil && toolID != "" && toolID != "none" {
				if def, getErr := h.toolRepo.GetByID(ctx, toolID); getErr == nil {
					toolDef = def
				}
			}
			break
		}
	}

	scenario := h.loadScenarioByID(ctx, run.ScenarioID)
	allowLive := Config.AllowLiveExecution && (!Config.AuthEnabled || claims.Role == "instructor")

	ev, graphState, err := ExecuteRunNode(run, nodeID, stepReq, toolDef, scenario, allowLive)
	if err != nil {
		switch {
		case errors.Is(err, ErrRunAlreadyClosed):
			c.JSON(http.StatusConflict, gin.H{"error": "รอบฝึกนี้จบไปแล้ว"})
		case errors.Is(err, ErrRunNodeNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบโหนดนี้ในรอบฝึก"})
		case errors.Is(err, ErrRunNodeNotReady), errors.Is(err, ErrRunInvalidRequest):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "รันขั้นตอนไม่สำเร็จ"})
		}
		return
	}

	if err := h.runRepo.UpdateRun(ctx, *run); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "บันทึกสถานะรอบฝึกไม่สำเร็จ"})
		return
	}
	savedEv, _ := h.runRepo.AppendEvent(ctx, ev)
	c.JSON(http.StatusOK, gin.H{
		"run":         run,
		"event":       savedEv,
		"graph_state": graphState,
	})
}

func (h *ApiHandler) CompleteRun(c *gin.Context) {
	if h.runRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Run repository ยังไม่พร้อมใช้งาน"})
		return
	}
	var req CompleteRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ข้อมูลส่งคำตอบไม่ถูกต้อง"})
		return
	}
	verdict := strings.ToLower(strings.TrimSpace(req.FinalVerdict))
	if !validScenarioVerdicts[verdict] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "กรุณาเลือก final_verdict เป็น clean, suspicious หรือ malicious"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	run, err := h.runRepo.GetRunByID(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, ErrRunNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "ไม่พบรอบฝึกนี้"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "โหลดรอบฝึกไม่สำเร็จ"})
		return
	}
	claims, _ := GetAuthClaims(c)
	if claims.Role != "instructor" && run.TraineeID != claims.UserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "ไม่มีสิทธิ์ปิดรอบฝึกของผู้อื่น"})
		return
	}
	if run.Status != "in_progress" {
		c.JSON(http.StatusConflict, gin.H{"error": "รอบฝึกนี้จบไปแล้ว"})
		return
	}

	now := time.Now()
	run.FinalVerdict = verdict
	run.Summary = strings.TrimSpace(req.Summary)
	scenario := h.loadScenarioByID(ctx, run.ScenarioID)
	score := ScoreSimulationRun(run, scenario, now)
	run.Score = &score
	run.Status = "completed"
	run.CompletedAt = &now
	run.UpdatedAt = now

	if err := h.runRepo.UpdateRun(ctx, *run); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "บันทึกผลคะแนนไม่สำเร็จ"})
		return
	}
	ev, _ := h.runRepo.AppendEvent(ctx, RunEvent{
		RunID:     run.ID,
		Action:    "run_completed",
		Status:    "completed",
		Verdict:   verdict,
		Notes:     run.Summary,
		Output:    map[string]any{"score": score},
		Timestamp: now,
	})
	events, _ := h.runRepo.ListEventsByRun(ctx, run.ID)
	c.JSON(http.StatusOK, gin.H{
		"run":    run,
		"score":  score,
		"event":  ev,
		"events": events,
	})
}
