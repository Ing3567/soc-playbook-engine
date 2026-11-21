import React, { useState, useEffect, useCallback, useRef } from 'react'
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
  NodeImage,
  PlaybookItem,
  RunStatus,
  ExecutionLog,
  ToolInputField,
  ToolStatusSummary,
} from './types'
import {
  getTools,
  generateFlowchart,
  getPlaybooks,
  savePlaybook,
  deletePlaybook,
  executeStep,
  downloadExportFile,
} from './api'
import { FlowNodeCard } from './FlowNodeCard'

const nodeTypes = { socNode: FlowNodeCard }

const SAMPLE_SOP = `ตรวจสอบ IP การเข้าระบบผ่านแพลตฟอร์ม VirusTotal
ถ้าพบว่าเป็น IP อันตรายมากกว่า 10 รายการ
  ระงับบัญชีผู้ใช้งานและบล็อก IP บน Firewall
  เปิดเคสแจ้งเตือนไปยัง Tier 2
ไม่เช่นนั้นถ้าพบประวัติเคสเก่าในระบบ Ticket
  ปิดเคส
ไม่เช่นนั้น
  เฝ้าระวังพฤติกรรม 24 ชั่วโมง
บันทึกผลลงระบบ Ticket`

function buildInitialNodeInputs(nodes: FlowNode[]): Record<string, Record<string, string>> {
  const next: Record<string, Record<string, string>> = {}
  for (const n of nodes) {
    const splunkLink = (n.links || []).find((l) => l.splunk_query)
    if (splunkLink?.splunk_query) {
      next[n.id] = { query: splunkLink.splunk_query }
    }
  }
  return next
}

function estimateNodeDimensions(n: FlowNode, inputFields: ToolInputField[]): { width: number; height: number } {
  const links = n.links || []
  const images = n.images || []
  const canRun = n.type === 'lookup' || n.type === 'action'
  const fieldCount = canRun ? inputFields.length : 0
  const hasRichContent = links.length > 0 || images.length > 0 || fieldCount > 0
  const width = hasRichContent ? 260 : 230

  const extraTextLines = Math.max(0, Math.ceil((n.label || '').length / 30) - 2)
  let height = 86 + extraTextLines * 16

  if (fieldCount > 0) {
    height += 12 + fieldCount * 42
  }

  if (links.length > 0) {
    height += 12
    for (const l of links) {
      height += l.splunk_query ? 52 : 28
    }
  }

  if (images.length > 0) {
    const rows = images.length > 1 ? Math.ceil(images.length / 2) : 1
    height += 12 + rows * 108
  }

  return { width, height }
}

function buildGraphLayout(
  nodes: FlowNode[],
  edges: FlowEdge[],
  toolsById: Record<string, ToolStatusSummary>,
  statuses: Record<string, RunStatus>,
  nodeInputs: Record<string, Record<string, string>>,
  onRunNode: (id: string) => void,
  onInputChange: (nodeId: string, key: string, value: string) => void,
  onPreviewImage: (img: NodeImage) => void
) {
  const g = new dagre.graphlib.Graph()
  g.setDefaultEdgeLabel(() => ({}))
  g.setGraph({ rankdir: 'TB', nodesep: 55, ranksep: 70 })

  const dimsById: Record<string, { width: number; height: number }> = {}
  nodes.forEach((n) => {
    const fields = toolsById[n.tool]?.input_fields || []
    const dims = estimateNodeDimensions(n, fields)
    dimsById[n.id] = dims
    g.setNode(n.id, dims)
  })
  edges.forEach((e) => g.setEdge(e.source, e.target))
  dagre.layout(g)

  const rfNodes: RFNode[] = nodes.map((n) => {
    const pos = g.node(n.id)
    const dims = dimsById[n.id] || { width: 230, height: 86 }
    const fields = toolsById[n.tool]?.input_fields || []
    return {
      id: n.id,
      type: 'socNode',
      position: { x: (pos?.x ?? 0) - dims.width / 2, y: (pos?.y ?? 0) - dims.height / 2 },
      data: {
        ...n,
        status: statuses[n.id] || 'idle',
        inputFields: fields,
        inputValues: nodeInputs[n.id] || {},
        onRun: () => onRunNode(n.id),
        onInputChange: (key: string, value: string) => onInputChange(n.id, key, value),
        onPreviewImage,
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
  const [toolsById, setToolsById] = useState<Record<string, ToolStatusSummary>>({})
  const [statuses, setStatuses] = useState<Record<string, RunStatus>>({})
  const [nodeInputs, setNodeInputs] = useState<Record<string, Record<string, string>>>({})
  const [logs, setLogs] = useState<ExecutionLog[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [previewImage, setPreviewImage] = useState<NodeImage | null>(null)

  const [rfNodes, setRfNodes, onNodesChange] = useNodesState<RFNode>([])
  const [rfEdges, setRfEdges, onEdgesChange] = useEdgesState<RFEdge>([])

  const nodesRef = useRef<FlowNode[]>([])
  const statusesRef = useRef<Record<string, RunStatus>>({})
  const nodeInputsRef = useRef<Record<string, Record<string, string>>>({})
  useEffect(() => {
    nodesRef.current = nodes
  }, [nodes])
  useEffect(() => {
    statusesRef.current = statuses
  }, [statuses])
  useEffect(() => {
    nodeInputsRef.current = nodeInputs
  }, [nodeInputs])

  const reloadPlaybooks = useCallback(async () => {
    setSavedList(await getPlaybooks())
  }, [])

  useEffect(() => {
    let active = true
    void getPlaybooks().then((items) => {
      if (active) setSavedList(items)
    })
    void getTools().then((tools) => {
      if (!active) return
      const map: Record<string, ToolStatusSummary> = {}
      for (const t of tools) {
        map[t.id] = t
      }
      setToolsById(map)
    })
    return () => {
      active = false
    }
  }, [])

  useEffect(() => {
    if (!previewImage) return
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setPreviewImage(null)
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [previewImage])

  const handleNodeInputChange = useCallback((nodeId: string, key: string, value: string) => {
    setNodeInputs((prev) => ({
      ...prev,
      [nodeId]: {
        ...(prev[nodeId] || {}),
        [key]: value,
      },
    }))
  }, [])

  const runNode = useCallback(async (nodeId: string) => {
    const target = nodesRef.current.find((n) => n.id === nodeId)
    if (!target) return
    setStatuses((prev) => ({ ...prev, [nodeId]: 'running' }))
    try {
      const rawInputs = nodeInputsRef.current[nodeId] || {}
      const inputs: Record<string, string> = {}
      for (const [k, v] of Object.entries(rawInputs)) {
        const trimmed = v.trim()
        if (trimmed) inputs[k] = trimmed
      }
      const res = await executeStep(target, inputs)
      const nextStatus: RunStatus = res.status === 'failed' ? 'failed' : 'success'
      setStatuses((prev) => ({ ...prev, [nodeId]: nextStatus }))
      setLogs((prev) => [
        {
          node_id: target.id,
          label: target.label,
          status: nextStatus,
          mode: res.mode,
          duration_ms: res.duration_ms,
          timestamp: res.timestamp,
          output: res.output,
          error: res.error,
        },
        ...prev,
      ])
    } catch {
      setStatuses((prev) => ({ ...prev, [nodeId]: 'failed' }))
    }
  }, [])

  // Structural Dagre layout only runs when graph nodes/edges or tool schemas change
  useEffect(() => {
    if (nodes.length === 0) return
    const { rfNodes: nextN, rfEdges: nextE } = buildGraphLayout(
      nodes,
      edges,
      toolsById,
      statusesRef.current,
      nodeInputsRef.current,
      runNode,
      handleNodeInputChange,
      setPreviewImage
    )
    setRfNodes(nextN)
    setRfEdges(nextE)
  }, [nodes, edges, toolsById, runNode, handleNodeInputChange, setRfNodes, setRfEdges])

  // Lightweight state sync for statuses and inline node inputs without re-running Dagre
  useEffect(() => {
    setRfNodes((prev) =>
      prev.map((rfNode) => ({
        ...rfNode,
        data: {
          ...rfNode.data,
          status: statuses[rfNode.id] || 'idle',
          inputValues: nodeInputs[rfNode.id] || {},
        },
      }))
    )
  }, [statuses, nodeInputs, setRfNodes])

  async function submitGenerate(form: FormData, nextTitle?: string) {
    setLoading(true)
    setError('')
    try {
      const data = await generateFlowchart(form)
      const nextNodes = data.nodes || []
      setNodes(nextNodes)
      setEdges(data.edges || [])
      setEngine(data.engine || '-')
      setStatuses({})
      setNodeInputs(buildInitialNodeInputs(nextNodes))
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

  async function handleFileUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    const form = new FormData()
    form.append('file', file)
    setSopId('')
    const data = await submitGenerate(form, file.name.replace(/\.(docx|xlsx)$/i, ''))
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

  async function handleRunAll() {
    for (const n of nodes) {
      if (n.type === 'lookup' || n.type === 'action') {
        await runNode(n.id)
      }
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
              setNodeInputs({})
              setStatuses({})
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
          อัปโหลดไฟล์ Word (.docx) / Excel (.xlsx)
          <input type="file" accept=".docx,.xlsx,.doc,.xls" onChange={handleFileUpload} className="hidden" />
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
              onClick={() => downloadExportFile('bpmn', title, nodes, edges)}
              className="rounded bg-amber-600 py-1.5 text-[11px] font-semibold text-white hover:bg-amber-700"
            >
              2. BPMN 2.0 XML
            </button>
            <button
              type="button"
              onClick={() => downloadExportFile('markdown', title, nodes, edges)}
              className="rounded bg-purple-600 py-1.5 text-[11px] font-semibold text-white hover:bg-purple-700"
            >
              3. Markdown (.md)
            </button>
            <button
              type="button"
              onClick={() => downloadExportFile('cacao', title, nodes, edges)}
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
          <div className="text-xs font-bold mb-1">รายการที่บันทึกไว้ ({savedList.length})</div>
          <div className="space-y-1 max-h-40 overflow-y-auto">
            {savedList.map((item) => (
              <div key={item.id} className="flex items-center justify-between rounded border px-2 py-1 text-xs bg-slate-50">
                <button
                  type="button"
                  onClick={() => {
                    const loadedNodes = item.nodes || []
                    setSopId(item.id)
                    setTitle(item.title)
                    setText(item.raw_text)
                    setEngine(item.engine)
                    setNodes(loadedNodes)
                    setEdges(item.edges || [])
                    setStatuses({})
                    setNodeInputs(buildInitialNodeInputs(loadedNodes))
                  }}
                  className="truncate text-left flex-1 font-medium hover:text-blue-600"
                >
                  {item.title}
                </button>
                <button
                  type="button"
                  onClick={async () => {
                    await deletePlaybook(item.id)
                    reloadPlaybooks()
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
          {logs.map((log, i) => {
            const isFailed = log.status === 'failed'
            const modeLabel = (log.mode || 'sandbox').toUpperCase()
            let modeBadgeStyle = 'bg-slate-700 text-slate-300'
            if (log.mode === 'real') modeBadgeStyle = 'bg-emerald-900/80 text-emerald-300 border border-emerald-600'
            else if (log.mode === 'mockup') modeBadgeStyle = 'bg-indigo-900/80 text-indigo-300 border border-indigo-500'
            else if (log.mode === 'sandbox_fallback') modeBadgeStyle = 'bg-amber-900/80 text-amber-300 border border-amber-600'

            return (
              <div
                key={i}
                className={`mb-1.5 rounded bg-slate-800 p-1.5 border ${
                  isFailed ? 'border-rose-500/80' : 'border-slate-700'
                }`}
              >
                <div className="flex items-center justify-between gap-2">
                  <div className={`font-semibold ${isFailed ? 'text-rose-400' : 'text-sky-300'}`}>
                    [{log.timestamp}] {log.label} ({log.duration_ms} ms)
                  </div>
                  <span className={`rounded px-1.5 py-0.5 text-[9px] font-bold ${modeBadgeStyle}`}>
                    {modeLabel}
                  </span>
                </div>
                <pre className="mt-1 whitespace-pre-wrap break-all text-[11px] text-slate-300">
                  {JSON.stringify(log.output)}
                </pre>
              </div>
            )
          })}
        </div>
      </main>

      {previewImage && (
        <div
          className="no-print fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-4"
          onClick={() => setPreviewImage(null)}
        >
          <div
            className="flex max-h-[90vh] max-w-4xl flex-col overflow-hidden rounded-xl bg-white shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between border-b border-slate-200 px-4 py-2.5">
              <div className="truncate pr-4">
                <div className="text-sm font-bold text-slate-800">
                  {previewImage.caption || previewImage.filename || 'Image Preview'}
                </div>
                {previewImage.filename && previewImage.caption && previewImage.filename !== previewImage.caption && (
                  <div className="text-[11px] text-slate-500">{previewImage.filename}</div>
                )}
              </div>
              <div className="flex items-center gap-2 shrink-0">
                {(previewImage.url.startsWith('/api/images/') || /^https?:\/\//i.test(previewImage.url)) && (
                  <a
                    href={previewImage.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="rounded bg-blue-50 px-2.5 py-1 text-xs font-semibold text-blue-700 hover:bg-blue-100"
                  >
                    เปิดรูปเต็ม ↗
                  </a>
                )}
                <button
                  type="button"
                  onClick={() => setPreviewImage(null)}
                  className="rounded bg-slate-100 px-2.5 py-1 text-xs font-bold text-slate-700 hover:bg-slate-200"
                >
                  ✕ ปิด
                </button>
              </div>
            </div>
            <div className="flex-1 overflow-auto bg-slate-900/5 p-4 flex items-center justify-center">
              <img
                src={previewImage.url}
                alt={previewImage.caption || previewImage.filename}
                className="max-h-[75vh] max-w-full rounded object-contain shadow"
              />
            </div>
          </div>
        </div>
      )}
    </div>
  )
}