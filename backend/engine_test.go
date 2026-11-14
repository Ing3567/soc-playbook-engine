package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenCaseNotConfusedWithCloseCase(t *testing.T) {
	if got := inferStepType("เปิดเคสแจ้งเตือนไปยัง Tier 2"); got != "action" {
		t.Fatalf("inferStepType(เปิดเคส) = %q, want action", got)
	}
	if got := inferStepType("ปิดเคส"); got != "end" {
		t.Fatalf("inferStepType(ปิดเคส) = %q, want end", got)
	}
	if got := MatchToolByKeywords("action", "เปิดเคสแจ้งเตือนไปยัง Tier 2"); got != "jira_ticket" {
		t.Fatalf("MatchToolByKeywords(เปิดเคส) = %q, want jira_ticket", got)
	}
}

func TestSandboxTriggers(t *testing.T) {
	res := RunSandboxStep(ExecutionRequest{Tool: "abuseipdb", Label: "ตรวจสอบ failed login จาก IP"})
	if res.Status != "success" {
		t.Fatalf("expected normal 'failed login' to succeed, got %q", res.Status)
	}

	failRes := RunSandboxStep(ExecutionRequest{Tool: "abuseipdb", Label: "ตรวจสอบ IP [simulate-fail]"})
	if failRes.Status != "failed" {
		t.Fatalf("expected [simulate-fail] to fail, got %q", failRes.Status)
	}

	abnormal := RunSandboxStep(ExecutionRequest{Tool: "virustotal", Label: "ตรวจสอบพฤติกรรมผิดปกติ VirusTotal"})
	if abnormal.Output["verdict"] == "Clean / No Detections" {
		t.Fatalf("expected 'ผิดปกติ' not to be treated as clean, got %v", abnormal.Output["verdict"])
	}
}

func TestAllPathsEndPrunesOrphanEndNode(t *testing.T) {
	steps := ParseLinesByIndentation(ExtractLinesFromText("ถ้า IP อันตราย\n  ปิดเคส\nไม่เช่นนั้น\n  ปิดเคส"))
	nodes, edges := BuildCustomFlowchart(steps)

	xmlBytes, err := BuildBpmnXML("test", nodes, edges)
	if err != nil {
		t.Fatalf("BuildBpmnXML error: %v", err)
	}
	if count := strings.Count(string(xmlBytes), "<bpmn:startEvent"); count != 1 {
		t.Fatalf("expected 1 BPMN startEvent, got %d", count)
	}
}

func TestCacaoExportValidStructure(t *testing.T) {
	steps := ParseLinesByIndentation(ExtractLinesFromText("ตรวจสอบ IP ผ่าน VirusTotal\nถ้าพบอันตราย\n  บล็อก IP\nไม่เช่นนั้น\n  ปิดเคส"))
	nodes, edges := BuildCustomFlowchart(steps)
	data, err := BuildCacaoJSON("test", nodes, edges)
	if err != nil {
		t.Fatalf("BuildCacaoJSON error: %v", err)
	}
	var pb map[string]any
	if err := json.Unmarshal(data, &pb); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	wf := pb["workflow"].(map[string]any)
	startID, _ := pb["workflow_start"].(string)
	if _, ok := wf[startID]; !ok {
		t.Fatalf("workflow_start %q not found in workflow", startID)
	}
	if pb["created_by"] == nil || pb["playbook_types"] == nil {
		t.Fatalf("missing required CACAO 2.0 fields")
	}
}
