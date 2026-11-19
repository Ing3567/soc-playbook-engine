import { useState, useEffect, useCallback, useRef, type ChangeEvent } from 'react'
import {
  ReactFlow,
  Background,
  Controls,
  MarkerType,
  useNodesState,
  useEdgesState,
  type Node as RFNode,
  type Edge as RFEdge,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import dagre from '@dagrejs/dagre'

import type {
  FlowNode,
  FlowEdge,
  PlaybookItem,
  RunStatus,
  ExecutionLog,
  ToolDefinition,
  PayloadMode,
} from './types'
import { generateFlowchart, getPlaybooks, savePlaybook, deletePlaybook, executeStep, downloadExportFile, getTools } from './api'
import { FlowNodeCard } from './FlowNodeCard'
import { ToolManagerModal } from './ToolManagerModal'
import { LookupPayloadModal } from './LookupPayloadModal'

const nodeTypes = { socNode: FlowNodeCard }

const SAMPLE_SOP = `ตรวจสอบ IP การเข้าระบบผ่านแพลตฟอร์ม VirusTotal
ถ้าพบว่าเป็น IP อันตรายมากกว่า 10 รายการ
  ระงับบัญชีผู้ใช้งานและบล็อก IP บน Firewall
  เปิดเคสแจ้งเตือนไปยัง Tier 2
ไม่เช่นนั้นถ้าพบประวัติเคสเก่าใน Jira
  ปิดเคส
ไม่เช่นนั้น
  เฝ้าระวังพฤติกรรม 24 ชั่วโมง
บันทึกผลลงระบบ Ticket`

function buildGraphLayout(
  nodes: FlowNode[],
  edges: FlowEdge[],
  statuses: Record<string, RunStatus>,
  availableTools: ToolDefinition[],
  onRunNode: (id: string) => void,
  onChangeTool: (id: string, nextTool: string) => void,
  onEditPayload: (id: string) => void
) {
  const g = new dagre.graphlib.Graph()
  g.setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: 'TB', nodesep: 60, ranksep: 90 })

  nodes.forEach((n) => g.setNode(n.id, { width: 256, height: n.type === 'lookup' ? 136 : 116 }))
  edges.forEach((e) => g.setEdge(e.source, e.target))
  dagre.layout(g)

  const rfNodes: RFNode[] = nodes.map((n) => {
    const pos = g.node(n.id)
    const h = n.type === 'lookup' ? 136 : 116
    return {
      id: n.id,
      type: 'socNode',
      position: { x: (pos?.x ?? 0) - 128, y: (pos?.y ?? 0) - h / 2 },
      data: {
        ...n,
        status: statuses[n.id] || 'idle',
        availableTools,
        onRun: () => onRunNode(n.id),
        onChangeTool: (nextTool: string) => onChangeTool(n.id, nextTool),
        onEditPayload: () => onEditPayload(n.id),
      },
    }
  })

  const rfEdges: RFEdge[] = edges.map((e) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    label: e.label,
    style: { stroke: '#64748b', strokeWidth: 2 },
    labelStyle: { fill: '#0f172a', fontWeight: 600, fontSize: 12 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#64748b' },
  }))

  return { rfNodes, rfEdges }
}

export default function App() {
  const [sopId, setSopId] = useState('')
  const [title, setTitle] = useState('SOP ตรวจสอบ IP ต้องสงสัย')
  const [text, setText] = useState(SAMPLE_SOP)
  const [engine, setEngine] = useState('-')
  const [nodes, setNodes] = useState<FlowNode[]>([])
  const [edges, setEdges] = useState<FlowEdge[]>([])
  const [savedList, setSavedList] = useState<PlaybookItem[]>([])
  const [tools, setTools] = useState<ToolDefinition[]>([])
  const [toolsLoadError, setToolsLoadError] = useState('')
  const [toolModalOpen, setToolModalOpen] = useState(false)
  const [editingPayloadNodeId, setEditingPayloadNodeId] = useState<string | null>(null)
  const [alertContext, setAlertContext] = useState({
    src_ip: '198.51.100.42',
    domain: 'malicious.example.com',
    file_hash: '44d88612fea8a8f36de82e1278abb02f',
    username: 'jdoe',
  })
  const [statuses, setStatuses] = useState<Record<string, RunStatus>>({})
  const [logs, setLogs] = useState<ExecutionLog[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const [rfNodes, setRfNodes, onNodesChange] = useNodesState<RFNode>([])
  const [rfEdges, setRfEdges, onEdgesChange] = useEdgesState<RFEdge>([])

  const nodesRef = useRef<FlowNode[]>([])
  nodesRef.current = nodes
  const toolsRef = useRef<ToolDefinition[]>([])
  toolsRef.current = tools
  const alertContextRef = useRef(alertContext)
  alertContextRef.current = alertContext
  const skipNextLayoutRef = useRef(false)

  const reloadPlaybooks = useCallback(async () => {
    setSavedList(await getPlaybooks())
  }, [])

  const reloadTools = useCallback(async () => {
    try {
      const list = await getTools()
      setTools(list)
      setToolsLoadError('')
    } catch (e) {
      setToolsLoadError(e instanceof Error ? e.message : 'โหลดรายการ Tools ไม่สำเร็จ')
    }
  }, [])

  useEffect(() => {
    reloadPlaybooks()
    reloadTools()
  }, [reloadPlaybooks, reloadTools])

  const changeNodeTool = useCallback(
    (nodeId: string, nextTool: string) => {
      skipNextLayoutRef.current = true
      const matched = toolsRef.current.find((t) => t.id === nextTool)
      const nextMode: PayloadMode | undefined = matched?.payload_mode || (matched?.params?.length ? 'dynamic' : undefined)
      const nextPayload: Record<string, string> | undefined = matched?.params?.length
        ? Object.fromEntries(
            matched.params.map((p) => [p.key, p.value || p.default_value || ''])
          )
        : undefined

      setNodes((prev) =>
        prev.map((n) =>
          n.id === nodeId
            ? { ...n, tool: nextTool, tool_source: 'manual', payload_mode: nextMode, input_payload: nextPayload }
            : n
        )
      )
      setRfNodes((prev) =>
        prev.map((n) =>
          n.id === nodeId
            ? {
                ...n,
                data: {
                  ...n.data,
                  tool: nextTool,
                  tool_source: 'manual',
                  payload_mode: nextMode,
                  input_payload: nextPayload,
                },
              }
            : n
        )
      )
    },
    [setRfNodes]
  )

  const updateNodePayload = useCallback(
    (nodeId: string, payloadMode: PayloadMode, inputPayload: Record<string, string>) => {
      skipNextLayoutRef.current = true
      setNodes((prev) =>
        prev.map((n) => (n.id === nodeId ? { ...n, payload_mode: payloadMode, input_payload: inputPayload } : n))
      )
      setRfNodes((prev) =>
        prev.map((n) =>
          n.id === nodeId
            ? { ...n, data: { ...n.data, payload_mode: payloadMode, input_payload: inputPayload } }
            : n
        )
      )
    },
    [setRfNodes]
  )

  const runNode = useCallback(
    async (
      nodeId: string,
      extraContext?: Record<string, unknown>
    ): Promise<{ ok: boolean; output: Record<string, unknown> | null }> => {
      const target = nodesRef.current.find((n) => n.id === nodeId)
      if (!target) return { ok: false, output: null }
      setStatuses((prev) => ({ ...prev, [nodeId]: 'running' }))
      const runtimeCtx: Record<string, unknown> = {
        alert: alertContextRef.current,
        ...extraContext,
      }
      try {
        const res = await executeStep(target, runtimeCtx)
        const nextStatus: RunStatus = res.status === 'failed' ? 'failed' : 'success'
        setStatuses((prev) => ({ ...prev, [nodeId]: nextStatus }))
        setLogs((prev) => [
          {
            node_id: target.id,
            label: target.label,
            status: nextStatus,
            duration_ms: res.duration_ms,
            timestamp: res.timestamp,
            output: res.output,
          },
          ...prev,
        ])
        return { ok: nextStatus === 'success', output: res.output ?? null }
      } catch (e) {
        setStatuses((prev) => ({ ...prev, [nodeId]: 'failed' }))
        setLogs((prev) => [
          {
            node_id: target.id,
            label: target.label,
            status: 'failed',
            duration_ms: 0,
            timestamp: new Date().toTimeString().slice(0, 8),
            output: { error: e instanceof Error ? e.message : 'Execution failed' },
          },
          ...prev,
        ])
        return { ok: false, output: null }
      }
    },
    []
  )

  useEffect(() => {
    if (skipNextLayoutRef.current) {
      skipNextLayoutRef.current = false
      return
    }
    if (nodes.length === 0) {
      setRfNodes([])
      setRfEdges([])
      return
    }
    const { rfNodes: nextN, rfEdges: nextE } = buildGraphLayout(
      nodes,
      edges,
      {},
      toolsRef.current,
      runNode,
      changeNodeTool,
      (id) => setEditingPayloadNodeId(id)
    )
    setRfNodes(nextN)
    setRfEdges(nextE)
  }, [nodes, edges, runNode, changeNodeTool, setRfNodes, setRfEdges])

  useEffect(() => {
    setRfNodes((prev) =>
      prev.map((n) => ({
        ...n,
        data: { ...n.data, status: statuses[n.id] || 'idle', availableTools: tools },
      }))
    )
  }, [statuses, tools, setRfNodes])

  async function submitGenerate(form: FormData, nextTitle?: string) {
    setLoading(true)
    setError('')
    try {
      const data = await generateFlowchart(form)
      setNodes(data.nodes || [])
      setEdges(data.edges || [])
      setEngine(data.engine || '-')
      setStatuses({})
      if (nextTitle) setTitle(nextTitle)
      return data
    } catch (e) {
      setError(e instanceof Error ? e.message : 'เกิดข้อผิดพลาด')
      return null
    } finally {
      setLoading(false)
    }
  }

  async function handleGenerateText() {
    const form = new FormData()
    form.append('text', text)
    await submitGenerate(form)
  }

  async function handleFileUpload(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    const form = new FormData()
    form.append('file', file)
    setSopId('')
    const data = await submitGenerate(form, file.name.replace(/\.docx$/i, ''))
    if (data?.lines) {
      setText(data.lines.map((l) => '  '.repeat(l.level) + l.text).join('\n'))
    }
  }

  async function handleSave() {
    if (nodes.length === 0) return setError('กรุณาสร้าง Flowchart ก่อนบันทึก')
    try {
      const saved = await savePlaybook({ id: sopId, title, raw_text: text, engine, nodes, edges })
      setSopId(saved.id)
      reloadPlaybooks()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'บันทึกไม่สำเร็จ')
    }
  }

  function pickDecisionEdge(outEdges: FlowEdge[], lastOutput: Record<string, unknown> | null): FlowEdge | undefined {
    if (outEdges.length === 0) return undefined
    if (outEdges.length === 1) return outEdges[0]

    const isPositive =
      lastOutput != null &&
      ((typeof lastOutput.malicious_count === 'number' && lastOutput.malicious_count > 0) ||
        (typeof lastOutput.confidence_score === 'number' && lastOutput.confidence_score >= 50) ||
        (typeof lastOutput.risk_score === 'number' && lastOutput.risk_score >= 50) ||
        lastOutput.is_malicious === true ||
        lastOutput.alert === true ||
        lastOutput.matched === true ||
        (Array.isArray(lastOutput.matched_cases) && lastOutput.matched_cases.length > 0))

    if (isPositive) {
      return outEdges.find((e) => e.label === 'ใช่' || e.label?.toLowerCase() === 'yes') ?? outEdges[0]
    }
    return outEdges.find((e) => e.label === 'ไม่ใช่' || e.label?.toLowerCase() === 'no') ?? outEdges[outEdges.length - 1]
  }

  async function handleRunAll() {
    if (nodes.length === 0) return
    setStatuses({})

    const nodeMap = new Map(nodes.map((n) => [n.id, n]))
    const outgoing = new Map<string, FlowEdge[]>()
    const inDegree = new Map<string, number>()
    for (const n of nodes) inDegree.set(n.id, 0)
    for (const e of edges) {
      const list = outgoing.get(e.source) ?? []
      list.push(e)
      outgoing.set(e.source, list)
      inDegree.set(e.target, (inDegree.get(e.target) ?? 0) + 1)
    }

    const startNode = nodes.find((n) => inDegree.get(n.id) === 0) ?? nodes[0]
    const visited = new Set<string>()
    const nodeOutputs: Record<string, unknown> = {}
    let current: FlowNode | undefined = startNode
    let lastOutput: Record<string, unknown> | null = null

    while (current && !visited.has(current.id)) {
      visited.add(current.id)
      const outEdges = outgoing.get(current.id) ?? []

      if (current.type === 'lookup' || current.type === 'action') {
        const res = await runNode(current.id, {
          prev: lastOutput ?? {},
          nodes: nodeOutputs,
        })
        if (!res.ok) break
        lastOutput = res.output
        if (res.output) {
          nodeOutputs[current.id] = res.output
        }
      } else if (current.type === 'end') {
        break
      }

      const nextEdge =
        current.type === 'decision' ? pickDecisionEdge(outEdges, lastOutput) : outEdges[0]
      current = nextEdge ? nodeMap.get(nextEdge.target) : undefined
    }
  }

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-slate-50 text-slate-800">
      <aside className="no-print flex w-80 flex-col border-r border-slate-200 bg-white p-3 gap-3 overflow-y-auto">
        <div className="flex items-center justify-between">
          <h1 className="text-base font-bold">SOC Playbook Engine</h1>
          <button
            type="button"
            onClick={() => {
              setSopId('')
              setTitle('New SOP')
              setText(SAMPLE_SOP)
              setEngine('-')
              setNodes([])
              setEdges([])
              setStatuses({})
              setLogs([])
              setError('')
            }}
            className="text-xs text-blue-600 hover:underline"
          >
            + สร้างใหม่
          </button>
        </div>

        <input
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          className="w-full rounded border border-slate-300 px-2.5 py-1.5 text-xs font-semibold"
          placeholder="ชื่อ SOP"
        />

        <label className="flex cursor-pointer items-center justify-center rounded border-2 border-dashed border-blue-400 bg-blue-50 py-2 text-xs font-semibold text-blue-700 hover:bg-blue-100">
          อัปโหลดไฟล์ Word (.docx)
          <input type="file" accept=".docx,.doc" onChange={handleFileUpload} className="hidden" />
        </label>

        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          className="flex-1 min-h-[160px] w-full rounded border border-slate-300 p-2 font-mono text-xs"
        />

        {error && <div className="rounded bg-rose-50 p-2 text-xs text-rose-700 border border-rose-200">{error}</div>}

        <div className="grid grid-cols-2 gap-1.5">
          <button
            type="button"
            onClick={handleGenerateText}
            disabled={loading}
            className="col-span-2 rounded bg-blue-600 py-2 text-xs font-bold text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {loading ? 'กำลังสร้างกราฟ...' : 'Generate Flowchart'}
          </button>
          <button
            type="button"
            onClick={handleSave}
            className="rounded bg-emerald-600 py-1.5 text-xs font-bold text-white hover:bg-emerald-700"
          >
            Save (MongoDB)
          </button>
          <button
            type="button"
            onClick={handleRunAll}
            className="rounded bg-indigo-600 py-1.5 text-xs font-bold text-white hover:bg-indigo-700"
          >
            Run Playbook
          </button>
          <button
            type="button"
            onClick={() => setToolModalOpen(true)}
            className="col-span-2 rounded border border-slate-300 bg-slate-100 py-1.5 text-xs font-bold text-slate-800 hover:bg-slate-200"
          >
            จัดการ Tools ({tools.filter((t) => t.enabled).length}/{tools.length})
          </button>
        </div>

        <div className="border-t pt-2">
          <div className="text-xs font-bold text-slate-700 mb-1.5">Export มาตรฐาน (4 รูปแบบ)</div>
          <div className="grid grid-cols-2 gap-1.5">
            <button
              type="button"
              onClick={() => window.print()}
              className="rounded bg-rose-600 py-1.5 text-[11px] font-semibold text-white hover:bg-rose-700"
            >
              1. PDF (โครงสร้างเรา)
            </button>
            <button
              type="button"
              onClick={() =>
                downloadExportFile('bpmn', title, nodes, edges).catch((e) =>
                  setError(e instanceof Error ? e.message : 'ส่งออก BPMN ไม่สำเร็จ')
                )
              }
              className="rounded bg-amber-600 py-1.5 text-[11px] font-semibold text-white hover:bg-amber-700"
            >
              2. BPMN 2.0 XML
            </button>
            <button
              type="button"
              onClick={() =>
                downloadExportFile('markdown', title, nodes, edges).catch((e) =>
                  setError(e instanceof Error ? e.message : 'ส่งออก Markdown ไม่สำเร็จ')
                )
              }
              className="rounded bg-purple-600 py-1.5 text-[11px] font-semibold text-white hover:bg-purple-700"
            >
              3. Markdown (.md)
            </button>
            <button
              type="button"
              onClick={() =>
                downloadExportFile('cacao', title, nodes, edges).catch((e) =>
                  setError(e instanceof Error ? e.message : 'ส่งออก CACAO ไม่สำเร็จ')
                )
              }
              className="rounded bg-teal-600 py-1.5 text-[11px] font-semibold text-white hover:bg-teal-700"
            >
              4. OASIS CACAO
            </button>
          </div>
        </div>

        <div className="text-[11px] text-slate-500">
          Engine (.env): <strong>{engine}</strong> · {nodes.length} nodes
        </div>

        <div className="border-t pt-2">
          <details className="text-xs">
            <summary className="cursor-pointer font-bold text-slate-700">
              Dynamic Payload Context (Test Inputs)
            </summary>
            <div className="mt-2 grid grid-cols-2 gap-1.5">
              <label className="text-[10px] font-semibold text-slate-600">
                alert.src_ip
                <input
                  type="text"
                  value={alertContext.src_ip}
                  onChange={(e) => setAlertContext((p) => ({ ...p, src_ip: e.target.value }))}
                  className="mt-0.5 w-full rounded border border-slate-300 px-1.5 py-1 font-mono text-[11px] font-normal"
                />
              </label>
              <label className="text-[10px] font-semibold text-slate-600">
                alert.domain
                <input
                  type="text"
                  value={alertContext.domain}
                  onChange={(e) => setAlertContext((p) => ({ ...p, domain: e.target.value }))}
                  className="mt-0.5 w-full rounded border border-slate-300 px-1.5 py-1 font-mono text-[11px] font-normal"
                />
              </label>
              <label className="text-[10px] font-semibold text-slate-600">
                alert.file_hash
                <input
                  type="text"
                  value={alertContext.file_hash}
                  onChange={(e) => setAlertContext((p) => ({ ...p, file_hash: e.target.value }))}
                  className="mt-0.5 w-full rounded border border-slate-300 px-1.5 py-1 font-mono text-[11px] font-normal"
                />
              </label>
              <label className="text-[10px] font-semibold text-slate-600">
                alert.username
                <input
                  type="text"
                  value={alertContext.username}
                  onChange={(e) => setAlertContext((p) => ({ ...p, username: e.target.value }))}
                  className="mt-0.5 w-full rounded border border-slate-300 px-1.5 py-1 font-mono text-[11px] font-normal"
                />
              </label>
            </div>
          </details>
        </div>

        <div className="border-t pt-2">
          <div className="text-xs font-bold mb-1">รายการที่บันทึกไว้ ({savedList.length})</div>
          <div className="space-y-1 max-h-40 overflow-y-auto">
            {savedList.map((item) => (
              <div key={item.id} className="flex items-center justify-between rounded border px-2 py-1 text-xs bg-slate-50">
                <button
                  type="button"
                  onClick={() => {
                    setSopId(item.id)
                    setTitle(item.title)
                    setText(item.raw_text)
                    setEngine(item.engine)
                    setNodes(item.nodes || [])
                    setEdges(item.edges || [])
                    setStatuses({})
                  }}
                  className="truncate text-left flex-1 font-medium hover:text-blue-600"
                >
                  {item.title}
                </button>
                <button
                  type="button"
                  onClick={async () => {
                    if (!window.confirm(`ลบ "${item.title}" ใช่หรือไม่?`)) return
                    try {
                      await deletePlaybook(item.id)
                      reloadPlaybooks()
                    } catch (e) {
                      setError(e instanceof Error ? e.message : 'ลบไม่สำเร็จ')
                    }
                  }}
                  className="ml-2 text-rose-500 font-bold"
                >
                  ✕
                </button>
              </div>
            ))}
          </div>
        </div>
      </aside>

      <main className="flex flex-1 flex-col">
        <div className="flex-1 print-canvas">
          <ReactFlow
            nodes={rfNodes}
            edges={rfEdges}
            nodeTypes={nodeTypes}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            fitView
          >
            <Background gap={16} size={1} />
            <Controls className="no-print" />
          </ReactFlow>
        </div>

        <div className="no-print h-40 border-t border-slate-200 bg-slate-900 text-slate-100 p-2.5 overflow-y-auto font-mono text-xs">
          <div className="flex items-center justify-between mb-1">
            <span className="font-bold text-emerald-400">Execution Logs</span>
            <button type="button" onClick={() => setLogs([])} className="text-slate-400 text-[11px]">
              Clear
            </button>
          </div>
          {logs.map((log, i) => (
            <div key={i} className="mb-1.5 rounded bg-slate-800 p-1.5 border border-slate-700">
              <div className="text-sky-300 font-semibold">
                [{log.timestamp}] {log.label} ({log.duration_ms} ms)
              </div>
              <pre className="text-[11px] text-slate-300">{JSON.stringify(log.output)}</pre>
            </div>
          ))}
        </div>
      </main>

      <ToolManagerModal
        open={toolModalOpen}
        tools={tools}
        loadError={toolsLoadError}
        onClose={() => setToolModalOpen(false)}
        onChanged={reloadTools}
      />

      <LookupPayloadModal
        node={nodes.find((n) => n.id === editingPayloadNodeId) ?? null}
        tool={tools.find((t) => t.id === nodes.find((n) => n.id === editingPayloadNodeId)?.tool)}
        runtimeContext={{ alert: alertContext }}
        onClose={() => setEditingPayloadNodeId(null)}
        onSave={updateNodePayload}
      />
    </div>
  )
}