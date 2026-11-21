package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ScoreSimulationRun computes a 0-100 rule-based score across 4 dimensions:
// Verdict Accuracy (40), Step Coverage (35), Decision Path (15), and MTTR Speed (10).
func ScoreSimulationRun(run *SimulationRun, scenario *Scenario, completedAt time.Time) RunScore {
	submitted := strings.ToLower(strings.TrimSpace(run.FinalVerdict))
	durationSec := int(math.Max(0, math.Round(completedAt.Sub(run.StartedAt).Seconds())))

	score := RunScore{
		DurationSec:      durationSec,
		SubmittedVerdict: submitted,
		Feedback:         []string{},
	}

	var gt *ScenarioGroundTruth
	if scenario != nil && scenario.GroundTruth != nil {
		gt = scenario.GroundTruth
		score.ExpectedVerdict = strings.ToLower(strings.TrimSpace(gt.ExpectedVerdict))
		score.MaxDurationSec = gt.MaxDurationSec
	}

	// 1. Verdict Accuracy (40 pts)
	if score.ExpectedVerdict != "" {
		switch {
		case submitted == score.ExpectedVerdict:
			score.VerdictPoints = 40
			score.Feedback = append(score.Feedback, fmt.Sprintf("✓ สรุปผลได้ถูกต้อง (%s)", submitted))
		case (submitted == "suspicious" && score.ExpectedVerdict == "malicious") ||
			(submitted == "malicious" && score.ExpectedVerdict == "suspicious"):
			score.VerdictPoints = 20
			score.Feedback = append(score.Feedback, fmt.Sprintf("△ สรุปผลใกล้เคียง (%s) แต่เฉลยคือ %s", submitted, score.ExpectedVerdict))
		default:
			score.VerdictPoints = 0
			score.Feedback = append(score.Feedback, fmt.Sprintf("✕ สรุปผลคลาดเคลื่อน: ตอบ %s แต่เฉลยคือ %s", submitted, score.ExpectedVerdict))
		}
	} else if submitted != "" {
		score.VerdictPoints = 40
		score.Feedback = append(score.Feedback, fmt.Sprintf("✓ บันทึกผลสรุปเคส (%s)", submitted))
	}

	graphState := ComputeRunGraphState(run.Nodes, run.Edges, run.NodeStatuses, run.NodeOutputs, run.Alert)
	nodeLabels := make(map[string]string, len(run.Nodes))
	for _, n := range run.Nodes {
		nodeLabels[n.ID] = n.Label
	}

	// 2. Step Coverage (35 pts)
	var requiredNodes []string
	if gt != nil && len(gt.RequiredNodes) > 0 {
		requiredNodes = gt.RequiredNodes
	} else {
		for _, n := range run.Nodes {
			if n.Type != "lookup" && n.Type != "action" {
				continue
			}
			if st, ok := graphState.NodeStates[n.ID]; ok && st.Availability == "skipped" {
				continue
			}
			requiredNodes = append(requiredNodes, n.ID)
		}
	}

	if len(requiredNodes) == 0 {
		score.CoveragePoints = 35
	} else {
		done := 0
		for _, nID := range requiredNodes {
			if run.NodeStatuses[nID] == "success" {
				done++
			} else {
				lbl := nodeLabels[nID]
				if lbl == "" {
					lbl = nID
				}
				score.MissedNodes = append(score.MissedNodes, lbl)
			}
		}
		score.CoveragePoints = int(math.Round(35.0 * float64(done) / float64(len(requiredNodes))))
		if len(score.MissedNodes) == 0 {
			score.Feedback = append(score.Feedback, fmt.Sprintf("✓ ทำตามขั้นตอนครบทุกขั้น (%d/%d)", done, len(requiredNodes)))
		} else {
			score.Feedback = append(score.Feedback, fmt.Sprintf("✕ ข้ามขั้นตอนสำคัญ %d รายการ: %s", len(score.MissedNodes), strings.Join(score.MissedNodes, ", ")))
		}
	}

	// 3. Decision Path (15 pts)
	traversedSet := make(map[string]bool, len(graphState.TraversedEdges))
	for _, eID := range graphState.TraversedEdges {
		traversedSet[eID] = true
	}

	if gt != nil && len(gt.ExpectedEdges) > 0 {
		matched := 0
		for _, eID := range gt.ExpectedEdges {
			if traversedSet[eID] {
				matched++
			} else {
				score.MissedEdges = append(score.MissedEdges, eID)
			}
		}
		score.PathPoints = int(math.Round(15.0 * float64(matched) / float64(len(gt.ExpectedEdges))))
		if len(score.MissedEdges) == 0 {
			score.Feedback = append(score.Feedback, "✓ เลือกเส้นทางตัดสินใจตรงตามมาตรฐาน SOP")
		} else {
			score.Feedback = append(score.Feedback, fmt.Sprintf("✕ เส้นทางตัดสินใจไม่ตรงกับที่คาดหวัง (%d/%d)", matched, len(gt.ExpectedEdges)))
		}
	} else {
		reachedEnd := false
		hasEnd := false
		for _, n := range run.Nodes {
			if n.Type == "end" {
				hasEnd = true
				if st, ok := graphState.NodeStates[n.ID]; ok && st.Availability == "ready" {
					reachedEnd = true
				}
			}
		}
		if (hasEnd && reachedEnd) || (!hasEnd && len(score.MissedNodes) == 0) {
			score.PathPoints = 15
			score.Feedback = append(score.Feedback, "✓ ดำเนินขั้นตอนจนจบเส้นทาง SOP")
		} else {
			score.PathPoints = 0
			score.Feedback = append(score.Feedback, "✕ ยังทำไม่ถึงจุดสิ้นสุดของ SOP ก่อนส่งคำตอบ")
		}
	}

	// 4. MTTR / Speed (10 pts)
	if score.MaxDurationSec <= 0 || durationSec <= score.MaxDurationSec {
		score.SpeedPoints = 10
		if score.MaxDurationSec > 0 {
			score.Feedback = append(score.Feedback, fmt.Sprintf("✓ ใช้เวลา %d วินาที (ผ่านเกณฑ์ไม่เกิน %d วินาที)", durationSec, score.MaxDurationSec))
		}
	} else if float64(durationSec) <= float64(score.MaxDurationSec)*1.5 {
		score.SpeedPoints = 5
		score.Feedback = append(score.Feedback, fmt.Sprintf("△ ใช้เวลา %d วินาที (เกินเกณฑ์ %d วินาทีเล็กน้อย)", durationSec, score.MaxDurationSec))
	} else {
		score.SpeedPoints = 0
		score.Feedback = append(score.Feedback, fmt.Sprintf("✕ ใช้เวลา %d วินาที (เกินเกณฑ์ %d วินาที)", durationSec, score.MaxDurationSec))
	}

	score.Total = score.VerdictPoints + score.CoveragePoints + score.PathPoints + score.SpeedPoints
	return score
}
