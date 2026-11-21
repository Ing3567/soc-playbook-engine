import { MarkerType, type Node as RFNode, type Edge as RFEdge } from '@xyflow/react'
import dagre from '@dagrejs/dagre'
import type { FlowEdge, FlowNode, RunStatus, ToolCategory, ToolDefinition } from './types'
import { formatEdgeCondition } from './pipeline'
import type { ManualVerdict } from './FlowNodeCard'

export const ACTIVE_RUN_STORAGE_KEY = 'soc_active_run_id'

export const SAMPLE_SOP = `ตรวจสอบ IP การเข้าระบบผ่านแพลตฟอร์ม VirusTotal
ถ้าพบว่าเป็น IP อันตรายมากกว่า 10 รายการ
  ระงับบัญชีผู้ใช้งานและบล็อก IP บน Firewall
  เปิดเคสแจ้งเตือนไปยัง Tier 2
ไม่เช่นนั้นถ้าพบประวัติเคสเก่าใน Jira
  ปิดเคส
ไม่เช่นนั้น
  เฝ้าระวังพฤติกรรม 24 ชั่วโมง
บันทึกผลลงระบบ Ticket`

export function buildGraphLayout(
  nodes: FlowNode[],
  edges: FlowEdge[],
  statuses: Record<string, RunStatus>,
  availableTools: ToolDefinition[],
  onRunNode: (id: string) => void,
  onManualComplete: (id: string, verdict: ManualVerdict, openedUrl?: string) => void,
  onChangeTool: (id: string, nextTool: string) => void,
  onEditPayload: (id: string) => void,
  onCreateTool: (category: ToolCategory) => void,
  onChangeRuntimeInput: (id: string, key: string, value: string) => void
) {
  const g = new dagre.graphlib.Graph()
  g.setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: 'TB', nodesep: 60, ranksep: 90 })

  const nodeHeight = (n: FlowNode) => {
    if (n.type === 'lookup') return 164
    if (n.type === 'action') return 148
    return 116
  }

  nodes.forEach((n) => g.setNode(n.id, { width: 256, height: nodeHeight(n) }))
  edges.forEach((e) => g.setEdge(e.source, e.target))
  dagre.layout(g)

  const rfNodes: RFNode[] = nodes.map((n) => {
    const pos = g.node(n.id)
    const h = nodeHeight(n)
    return {
      id: n.id,
      type: 'socNode',
      position: { x: (pos?.x ?? 0) - 128, y: (pos?.y ?? 0) - h / 2 },
      data: {
        ...n,
        status: statuses[n.id] || 'idle',
        availableTools,
        onRun: () => onRunNode(n.id),
        onManualComplete: (verdict: ManualVerdict, openedUrl?: string) => onManualComplete(n.id, verdict, openedUrl),
        onChangeTool: (nextTool: string) => onChangeTool(n.id, nextTool),
        onEditPayload: () => onEditPayload(n.id),
        onCreateTool: (cat: ToolCategory) => onCreateTool(cat),
        onChangeRuntimeInput: (key: string, value: string) => onChangeRuntimeInput(n.id, key, value),
      },
    }
  })

  const rfEdges: RFEdge[] = edges.map((e) => {
    const condText = formatEdgeCondition(e.condition)
    const displayLabel = condText ? (e.label ? `${e.label} (${condText})` : condText) : e.label
    return {
      id: e.id,
      source: e.source,
      target: e.target,
      label: displayLabel,
      style: { stroke: '#64748b', strokeWidth: 2 },
      labelStyle: { fill: '#0f172a', fontWeight: 600, fontSize: 12 },
      markerEnd: { type: MarkerType.ArrowClosed, color: '#64748b' },
    }
  })

  return { rfNodes, rfEdges }
}
