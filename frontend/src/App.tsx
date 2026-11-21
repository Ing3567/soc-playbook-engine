import { useState, useEffect, useCallback, useRef, useMemo, type ChangeEvent } from 'react'
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
  EdgeCondition,
  ExecutionMode,
  FlowNode,
  FlowEdge,
  PlaybookItem,
  RunStatus,
  ExecutionLog,
  ScenarioItem,
  ToolDefinition,
  ToolCategory,
  PayloadMode,
} from './types'
import {
  generateFlowchart,
  getPlaybooks,
  savePlaybook,
  deletePlaybook,
  executeStep,
  downloadExportFile,
  getTools,
  getScenarios,
  getRuntimeConfig,
} from './api'
import { computePipelineState, formatEdgeCondition, getDownstreamNodeIds, pickDecisionEdge } from './pipeline'
import { buildSopInputBlocks } from './runtimeParams'
import { FlowNodeCard, type ManualVerdict } from './FlowNodeCard'
import { ToolManagerModal } from './ToolManagerModal'
import { LookupPayloadModal } from './LookupPayloadModal'
import { EdgeConditionModal } from './EdgeConditionModal'

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
  const [scenarios, setScenarios] = useState<ScenarioItem[]>([])
  const [selectedScenarioId, setSelectedScenarioId] = useState('')
  const [toolModalOpen, setToolModalOpen] = useState(false)
  const [toolModalCategory, setToolModalCategory] = useState<ToolCategory | undefined>(undefined)
  const [toolModalEditId, setToolModalEditId] = useState<string | null>(null)
  const [editingPayloadNodeId, setEditingPayloadNodeId] = useState<string | null>(null)
  const [editingEdgeId, setEditingEdgeId] = useState<string | null>(null)
  const [alertContext, setAlertContext] = useState<Record<string, string>>({})
  const [runtimeInputs, setRuntimeInputs] = useState<Record<string, Record<string, string>>>({})
  const [sessionAuthKeys, setSessionAuthKeys] = useState<Record<string, string>>({})
  const [allowLiveExecution, setAllowLiveExecution] = useState(false)
  const [executionMode, setExecutionMode] = useState<ExecutionMode>('sandbox')
  const [statuses, setStatuses] = useState<Record<string, RunStatus>>({})
  const [nodeOutputs, setNodeOutputs] = useState<Record<string, Record<string, unknown>>>({})
  const [logs, setLogs] = useState<ExecutionLog[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [isRunningAll, setIsRunningAll] = useState(false)

  const [rfNodes, setRfNodes, onNodesChange] = useNodesState<RFNode>([])
  const [rfEdges, setRfEdges, onEdgesChange] = useEdgesState<RFEdge>([])

  const pipelineState = useMemo(
    () => computePipelineState(nodes, edges, statuses, nodeOutputs, alertContext),
    [nodes, edges, statuses, nodeOutputs, alertContext]
  )
  const sopInputData = useMemo(
    () => buildSopInputBlocks(nodes, edges, tools, runtimeInputs, alertContext),
    [nodes, edges, tools, runtimeInputs, alertContext]
  )
  const sessionAuthTools = useMemo(() => {
    const seen = new Set<string>()
    const list: ToolDefinition[] = []
    for (const n of nodes) {
      if (n.type !== 'lookup' && n.type !== 'action') continue
      const canonical = n.tool === 'jira_ticket' ? 'ticket' : n.tool
      if (!canonical || seen.has(canonical)) continue
      const def = tools.find((t) => t.id === canonical)
      if (
        def &&
        def.command_type === 'http-api' &&
        def.auth &&
        def.auth.type !== 'none' &&
        def.auth.allow_runtime &&
        !def.auth_configured
      ) {
        seen.add(canonical)
        list.push(def)
      }
    }
    return list
  }, [nodes, tools])
  const hasPendingReadyStep = useMemo(
    () =>
      Object.values(statuses).some((s) => s === 'success') &&
      nodes.some(
        (n) =>
          (n.type === 'lookup' || n.type === 'action') &&
          statuses[n.id] !== 'success' &&
          pipelineState.nodeStates[n.id]?.availability === 'ready'
      ),
    [nodes, statuses, pipelineState]
  )
  const activeScenario = useMemo(
    () => scenarios.find((s) => s.id === selectedScenarioId) ?? null,
    [scenarios, selectedScenarioId]
  )

  const nodesRef = useRef<FlowNode[]>([])
  nodesRef.current = nodes
  const edgesRef = useRef<FlowEdge[]>([])
  edgesRef.current = edges
  const toolsRef = useRef<ToolDefinition[]>([])
  toolsRef.current = tools
  const alertContextRef = useRef(alertContext)
  alertContextRef.current = alertContext
  const runtimeInputsRef = useRef(runtimeInputs)
  runtimeInputsRef.current = runtimeInputs
  const sessionAuthKeysRef = useRef(sessionAuthKeys)
  sessionAuthKeysRef.current = sessionAuthKeys
  const executionModeRef = useRef(executionMode)
  executionModeRef.current = executionMode
  const selectedScenarioIdRef = useRef(selectedScenarioId)
  selectedScenarioIdRef.current = selectedScenarioId
  const statusesRef = useRef(statuses)
  statusesRef.current = statuses
  const nodeOutputsRef = useRef(nodeOutputs)
  nodeOutputsRef.current = nodeOutputs
  const pipelineRef = useRef(pipelineState)
  pipelineRef.current = pipelineState
  const skipNextLayoutRef = useRef(false)

  const invalidateFromNode = useCallback((nodeId: string, includeSelf: boolean) => {
    const downstream = getDownstreamNodeIds(nodeId, edgesRef.current)
    const idsToClear = includeSelf ? [nodeId, ...downstream] : downstream
    if (idsToClear.length === 0) return
    setStatuses((prev) => {
      const next = { ...prev }
      for (const id of idsToClear) delete next[id]
      return next
    })
    setNodeOutputs((prev) => {
      const next = { ...prev }
      for (const id of idsToClear) delete next[id]
      return next
    })
  }, [])

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
    getScenarios(true).then(setScenarios)
    getRuntimeConfig().then((cfg) => {
      setAllowLiveExecution(cfg.allow_live_execution)
      if (!cfg.allow_live_execution) setExecutionMode('sandbox')
    })
  }, [reloadPlaybooks, reloadTools])

  const handleChangeRuntimeInput = useCallback((nodeId: string, key: string, value: string) => {
    setRuntimeInputs((prev) => ({
      ...prev,
      [nodeId]: {
        ...(prev[nodeId] || {}),
        [key]: value,
      },
    }))
  }, [])

  const changeNodeTool = useCallback(
    (nodeId: string, nextTool: string) => {
      skipNextLayoutRef.current = true
      invalidateFromNode(nodeId, true)
      const matched = toolsRef.current.find((t) => t.id === nextTool)
      const nextMode: PayloadMode | undefined = matched?.payload_mode || (matched?.params?.length ? 'dynamic' : undefined)
      const nextParamModes: Record<string, PayloadMode> | undefined = matched?.params?.length
        ? Object.fromEntries(
            matched.params.map((p) => [p.key, p.mode || matched.payload_mode || 'dynamic'])
          )
        : undefined
      const nextPayload: Record<string, string> | undefined = matched?.params?.length
        ? Object.fromEntries(
            matched.params
              .map((p) => {
                const mode = p.mode || matched.payload_mode || 'dynamic'
                const val = (p.value || p.default_value || '').trim()
                if (mode === 'fixed' || val.includes('{{')) {
                  return [p.key, val]
                }
                return [p.key, '']
              })
              .filter(([, v]) => v !== '')
          )
        : undefined

      setRuntimeInputs((prev) => {
        if (!(nodeId in prev)) return prev
        const next = { ...prev }
        delete next[nodeId]
        return next
      })

      setNodes((prev) =>
        prev.map((n) =>
          n.id === nodeId
            ? {
                ...n,
                tool: nextTool,
                tool_source: 'manual',
                payload_mode: nextMode,
                param_modes: nextParamModes,
                input_payload: nextPayload,
              }
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
                  param_modes: nextParamModes,
                  input_payload: nextPayload,
                },
              }
            : n
        )
      )
    },
    [setRfNodes, invalidateFromNode]
  )

  const updateNodePayload = useCallback(
    (nodeId: string, paramModes: Record<string, PayloadMode>, inputPayload: Record<string, string>) => {
      skipNextLayoutRef.current = true
      invalidateFromNode(nodeId, true)
      setNodes((prev) =>
        prev.map((n) => (n.id === nodeId ? { ...n, param_modes: paramModes, input_payload: inputPayload } : n))
      )
      setRfNodes((prev) =>
        prev.map((n) =>
          n.id === nodeId
            ? { ...n, data: { ...n.data, param_modes: paramModes, input_payload: inputPayload } }
            : n
        )
      )
    },
    [setRfNodes, invalidateFromNode]
  )

  const updateEdgeCondition = useCallback(
    (edgeId: string, nextLabel: string, nextCondition?: EdgeCondition) => {
      skipNextLayoutRef.current = true
      const targetEdge = edgesRef.current.find((e) => e.id === edgeId)
      if (targetEdge) {
        invalidateFromNode(targetEdge.source, false)
      }
      const condText = formatEdgeCondition(nextCondition)
      const displayLabel = condText ? (nextLabel ? `${nextLabel} (${condText})` : condText) : nextLabel
      setEdges((prev) =>
        prev.map((e) =>
          e.id === edgeId
            ? { ...e, label: nextLabel || undefined, condition: nextCondition }
            : e
        )
      )
      setRfEdges((prev) =>
        prev.map((e) => (e.id === edgeId ? { ...e, label: displayLabel || undefined } : e))
      )
    },
    [setRfEdges, invalidateFromNode]
  )

  const handleSelectScenario = useCallback(
    (scenarioId: string) => {
      setSelectedScenarioId(scenarioId)
      setStatuses({})
      setNodeOutputs({})
      setRuntimeInputs({})
      if (!scenarioId) {
        return
      }
      const found = scenarios.find((s) => s.id === scenarioId)
      if (!found) return
      const nextAlert: Record<string, string> = {}
      for (const [k, v] of Object.entries(found.alert || {})) {
        if (v !== null && v !== undefined) {
          nextAlert[k] = String(v)
        }
      }
      setAlertContext(nextAlert)
    },
    [scenarios]
  )

  const completeManualNode = useCallback(
    (nodeId: string, verdict: ManualVerdict, openedUrl?: string) => {
      const target = nodesRef.current.find((n) => n.id === nodeId)
      if (!target) return
      const pState = pipelineRef.current.nodeStates[nodeId]
      if (pState && (pState.availability === 'locked' || pState.availability === 'skipped')) {
        return
      }
      if (statusesRef.current[nodeId] === 'success') {
        invalidateFromNode(nodeId, false)
      }
      const canonicalTool = target.tool === 'jira_ticket' ? 'ticket' : target.tool
      const matchedTool = toolsRef.current.find((t) => t.id === canonicalTool)
      const verdictText =
        verdict === 'malicious'
          ? 'Malicious / Suspicious (Analyst Confirmed)'
          : verdict === 'clean'
            ? 'Clean / Benign (Analyst Confirmed)'
            : 'Completed (Manual Action)'
      const output: Record<string, unknown> = {
        mode: 'manual',
        tool: matchedTool?.name || canonicalTool,
        step: target.label,
        verdict: verdictText,
        is_malicious: verdict === 'malicious',
        completed: true,
        ...(openedUrl ? { opened_url: openedUrl } : {}),
      }
      setStatuses((prev) => ({ ...prev, [nodeId]: 'success' }))
      setNodeOutputs((prev) => ({ ...prev, [nodeId]: output }))
      setLogs((prev) => [
        {
          node_id: target.id,
          label: target.label,
          status: 'success',
          duration_ms: 0,
          timestamp: new Date().toTimeString().slice(0, 8),
          output,
        },
        ...prev,
      ])
    },
    [invalidateFromNode]
  )

  const runNode = useCallback(
    async (
      nodeId: string,
      extraContext?: Record<string, unknown>
    ): Promise<{ ok: boolean; output: Record<string, unknown> | null }> => {
      const target = nodesRef.current.find((n) => n.id === nodeId)
      if (!target) return { ok: false, output: null }

      const pState = pipelineRef.current.nodeStates[nodeId]
      if (!extraContext && pState && (pState.availability === 'locked' || pState.availability === 'skipped')) {
        return { ok: false, output: null }
      }

      if (!extraContext && statusesRef.current[nodeId] === 'success') {
        invalidateFromNode(nodeId, false)
      }

      setStatuses((prev) => ({ ...prev, [nodeId]: 'running' }))
      const runtimeCtx: Record<string, unknown> = {
        alert: alertContextRef.current,
        prev: extraContext?.prev ?? pState?.upstreamOutput ?? {},
        nodes: extraContext?.nodes ?? nodeOutputsRef.current,
      }
      const canonicalTool = target.tool === 'jira_ticket' ? 'ticket' : target.tool
      try {
        const res = await executeStep(target, runtimeCtx, runtimeInputsRef.current[nodeId], {
          runtimeAuth: sessionAuthKeysRef.current[canonicalTool],
          executionMode: executionModeRef.current,
          scenarioId: selectedScenarioIdRef.current || undefined,
        })
        const nextStatus: RunStatus = res.status === 'failed' ? 'failed' : 'success'
        setStatuses((prev) => ({ ...prev, [nodeId]: nextStatus }))
        if (nextStatus === 'success') {
          setNodeOutputs((prev) => ({ ...prev, [nodeId]: res.output ?? {} }))
        }
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
    [invalidateFromNode]
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
      completeManualNode,
      changeNodeTool,
      (id) => setEditingPayloadNodeId(id),
      (cat) => {
        setToolModalEditId(null)
        setToolModalCategory(cat)
        setToolModalOpen(true)
      },
      handleChangeRuntimeInput
    )
    setRfNodes(nextN)
    setRfEdges(nextE)
  }, [nodes, edges, runNode, completeManualNode, changeNodeTool, handleChangeRuntimeInput, setRfNodes, setRfEdges])

  useEffect(() => {
    setRfNodes((prev) =>
      prev.map((n) => {
        const pInfo = pipelineState.nodeStates[n.id]
        return {
          ...n,
          data: {
            ...n.data,
            status: statuses[n.id] || 'idle',
            availableTools: tools,
            runtimeInputs: runtimeInputs[n.id] || {},
            alertContext,
            sessionAuthKeys,
            pipelineAvailability: pInfo?.availability ?? 'ready',
            lockedReason: pInfo?.lockedReason ?? '',
          },
        }
      })
    )
    setRfEdges((prev) =>
      prev.map((e) => {
        const isTraversed = pipelineState.traversedEdgeIds.has(e.id)
        const isSkipped = pipelineState.skippedEdgeIds.has(e.id)
        const targetState = pipelineState.nodeStates[e.target]
        const isFrontier = isTraversed && targetState?.availability === 'ready' && statuses[e.target] !== 'success'
        const color = isSkipped ? '#cbd5e1' : isFrontier ? '#3b82f6' : isTraversed ? '#10b981' : '#94a3b8'
        return {
          ...e,
          animated: isFrontier,
          style: {
            stroke: color,
            strokeWidth: isTraversed ? 2.5 : 1.5,
            strokeDasharray: isSkipped ? '5 5' : undefined,
          },
          markerEnd: { type: MarkerType.ArrowClosed, color },
        }
      })
    )
  }, [statuses, tools, pipelineState, runtimeInputs, alertContext, sessionAuthKeys, setRfNodes, setRfEdges])

  async function submitGenerate(form: FormData, nextTitle?: string) {
    setLoading(true)
    setError('')
    try {
      const data = await generateFlowchart(form)
      setNodes(data.nodes || [])
      setEdges(data.edges || [])
      setEngine(data.engine || '-')
      setStatuses({})
      setNodeOutputs({})
      setRuntimeInputs({})
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
      const sanitizedNodes = nodes.map((n) => {
        if (!n.input_payload) return n
        const toolDef = tools.find((t) => t.id === n.tool)
        const authKeyLower = toolDef?.auth?.key_name?.trim().toLowerCase() || ''
        const cleaned: Record<string, string> = {}
        for (const [k, v] of Object.entries(n.input_payload)) {
          const val = v.trim()
          if (!val) continue
          const paramDef = toolDef?.params?.find((p) => p.key === k)
          if (paramDef?.is_secret || (authKeyLower && k.toLowerCase() === authKeyLower)) {
            continue
          }
          const mode = n.param_modes?.[k] || paramDef?.mode || n.payload_mode || toolDef?.payload_mode || 'dynamic'
          if (mode === 'fixed' || val.includes('{{')) {
            cleaned[k] = val
          }
        }
        return { ...n, input_payload: Object.keys(cleaned).length > 0 ? cleaned : undefined }
      })
      const saved = await savePlaybook({ id: sopId, title, raw_text: text, engine, nodes: sanitizedNodes, edges })
      setSopId(saved.id)
      reloadPlaybooks()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'บันทึกไม่สำเร็จ')
    }
  }

  async function handleRunAll(resume = false) {
    if (nodes.length === 0 || isRunningAll) return
    setIsRunningAll(true)

    const initialStatuses = resume ? { ...statusesRef.current } : {}
    const initialOutputs: Record<string, Record<string, unknown>> = resume ? { ...nodeOutputsRef.current } : {}
    if (!resume) {
      setStatuses({})
      setNodeOutputs({})
    }

    try {
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
      const localOutputs: Record<string, unknown> = { ...initialOutputs }
      let current: FlowNode | undefined = startNode
      let lastOutput: Record<string, unknown> | null = null

      while (current && !visited.has(current.id)) {
        visited.add(current.id)
        const outEdges = outgoing.get(current.id) ?? []

        if (current.type === 'lookup' || current.type === 'action') {
          if (resume && initialStatuses[current.id] === 'success') {
            const existingOut = initialOutputs[current.id] ?? null
            lastOutput = existingOut
            if (existingOut) {
              localOutputs[current.id] = existingOut
            }
          } else {
            const canonicalTool = current.tool === 'jira_ticket' ? 'ticket' : current.tool
            const toolDef = toolsRef.current.find((t) => t.id === canonicalTool)
            const isManualNode =
              toolDef?.command_type === 'manual' ||
              canonicalTool === 'manual_lookup' ||
              canonicalTool === 'manual_action'
            if (isManualNode) {
              setLogs((prev) => [
                {
                  node_id: current!.id,
                  label: current!.label,
                  status: 'idle',
                  duration_ms: 0,
                  timestamp: new Date().toTimeString().slice(0, 8),
                  output: {
                    mode: 'manual',
                    message: 'หยุดรอ Analyst ตรวจผ่านเบราว์เซอร์และกดยืนยันผลบนโหนดนี้ก่อนดำเนินการต่อ',
                  },
                },
                ...prev,
              ])
              break
            }

            const res = await runNode(current.id, {
              prev: lastOutput ?? {},
              nodes: localOutputs,
            })
            if (!res.ok) break
            lastOutput = res.output
            if (res.output) {
              localOutputs[current.id] = res.output
            }
          }
        } else if (current.type === 'end') {
          break
        }

        const nextEdge =
          current.type === 'decision'
            ? pickDecisionEdge(outEdges, lastOutput, {
                alert: alertContextRef.current,
                nodes: localOutputs as Record<string, Record<string, unknown>>,
              })
            : outEdges[0]
        current = nextEdge ? nodeMap.get(nextEdge.target) : undefined
      }
    } finally {
      setIsRunningAll(false)
    }
  }

  function handleResetExecution() {
    setStatuses({})
    setNodeOutputs({})
    setRuntimeInputs({})
    setAlertContext({})
    setSessionAuthKeys({})
    setSelectedScenarioId('')
  }

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-slate-50 text-slate-800">
      <aside className="no-print flex w-80 shrink-0 flex-col border-r border-slate-200 bg-white p-3 gap-3 overflow-y-auto">
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
              setNodeOutputs({})
              setRuntimeInputs({})
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
            className="col-span-2 rounded bg-emerald-600 py-1.5 text-xs font-bold text-white hover:bg-emerald-700"
          >
            Save (MongoDB)
          </button>
          <button
            type="button"
            onClick={() => {
              setToolModalEditId(null)
              setToolModalCategory(undefined)
              setToolModalOpen(true)
            }}
            className="col-span-2 rounded border border-sky-300 bg-sky-50 py-1.5 text-xs font-bold text-sky-900 hover:bg-sky-100"
          >
            ⚙ จัดการ Tools ({tools.filter((t) => t.enabled).length}/{tools.length} ใช้งาน)
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
          <details className="text-xs" open>
            <summary className="cursor-pointer font-bold text-slate-700">
              Run Session Inputs (ไม่บันทึกลง SOP)
            </summary>
            <p className="mt-1 text-[10px] text-slate-500">
              ค่าสำหรับเคสปัจจุบันเท่านั้น (เชื่อม 2 ทางกับบล็อกในกราฟ และไม่เซฟลง SOP)
            </p>

            {scenarios.length > 0 && (
              <div className="mt-2 rounded border border-indigo-200 bg-indigo-50/60 p-2 space-y-1.5">
                <div className="flex items-center justify-between gap-1">
                  <span className="text-[10px] font-bold text-indigo-950">🎯 Training Scenario (โจทย์จำลอง)</span>
                  {activeScenario && (
                    <span
                      className={`rounded px-1.5 py-0.5 text-[9px] font-bold uppercase ${
                        activeScenario.difficulty === 'hard'
                          ? 'bg-rose-100 text-rose-800'
                          : activeScenario.difficulty === 'medium'
                            ? 'bg-amber-100 text-amber-800'
                            : 'bg-emerald-100 text-emerald-800'
                      }`}
                    >
                      {activeScenario.difficulty}
                    </span>
                  )}
                </div>
                <select
                  value={selectedScenarioId}
                  onChange={(e) => handleSelectScenario(e.target.value)}
                  aria-label="Training Scenario"
                  className="w-full rounded border border-indigo-300 bg-white px-2 py-1 text-[11px] text-slate-800"
                >
                  <option value="">-- โหมดปกติ (ไม่ใช้ Scenario) --</option>
                  {scenarios.map((s) => (
                    <option key={s.id} value={s.id}>
                      [{s.difficulty}] {s.title}
                    </option>
                  ))}
                </select>
                {activeScenario && (
                  <p className="text-[10px] leading-snug text-indigo-900">{activeScenario.description}</p>
                )}
              </div>
            )}

            {allowLiveExecution && (
              <div className="mt-2 flex items-center justify-between rounded border border-slate-200 bg-slate-50 px-2 py-1">
                <span className="text-[10px] font-bold text-slate-700">โหมดรัน:</span>
                <div className="flex rounded border border-slate-300 bg-white p-0.5 text-[10px] font-bold">
                  <button
                    type="button"
                    onClick={() => setExecutionMode('sandbox')}
                    className={`rounded px-2 py-0.5 ${
                      executionMode === 'sandbox' ? 'bg-slate-800 text-white' : 'text-slate-600'
                    }`}
                  >
                    Sandbox
                  </button>
                  <button
                    type="button"
                    onClick={() => setExecutionMode('live')}
                    className={`rounded px-2 py-0.5 ${
                      executionMode === 'live' ? 'bg-emerald-600 text-white' : 'text-slate-600'
                    }`}
                  >
                    Live API
                  </button>
                </div>
              </div>
            )}

            {sessionAuthTools.length > 0 && (
              <div className="mt-2 rounded border border-amber-200 bg-amber-50/70 p-2">
                <div className="text-[10px] font-bold text-amber-900 mb-1">
                  🔑 Session API Keys (ชั่วคราว ไม่เซฟลง SOP)
                </div>
                <div className="space-y-1.5">
                  {sessionAuthTools.map((t) => (
                    <label key={`auth-${t.id}`} className="block text-[10px] font-semibold text-amber-950">
                      <div className="flex items-center justify-between gap-1">
                        <span className="truncate">{t.name}</span>
                        <span className="font-mono text-[9px] font-normal text-amber-700">
                          {t.auth?.env_var || t.auth?.key_name || t.auth?.type}
                        </span>
                      </div>
                      <input
                        type="password"
                        value={sessionAuthKeys[t.id] || ''}
                        placeholder={
                          t.auth?.type === 'basic'
                            ? 'user:password หรือ base64...'
                            : `กรอก API Key สำหรับ ${t.name}...`
                        }
                        onChange={(e) =>
                          setSessionAuthKeys((prev) => ({ ...prev, [t.id]: e.target.value }))
                        }
                        className="mt-0.5 w-full rounded border border-amber-300 bg-white px-1.5 py-1 font-mono text-[11px] font-normal"
                      />
                    </label>
                  ))}
                </div>
              </div>
            )}

            {nodes.length === 0 ? (
              <div className="mt-2 rounded border border-dashed border-slate-200 bg-slate-50 p-2 text-[11px] text-slate-500">
                สร้างหรือโหลด SOP เพื่อแสดงช่องกรอก Input ตามบล็อก
              </div>
            ) : sopInputData.blocks.length === 0 ? (
              <div className="mt-2 rounded border border-slate-200 bg-slate-50 p-2 text-[11px] text-slate-600">
                SOP นี้ไม่มีช่องที่ต้องกรอกเพิ่ม สามารถกดรันได้เลย
              </div>
            ) : (
              <div className="mt-2 space-y-2">
                {sopInputData.sharedAlertKeys.length > 0 && (
                  <div className="rounded border border-indigo-200 bg-indigo-50/70 p-2">
                    <div className="text-[10px] font-bold text-indigo-900 mb-1">
                      ค่าใช้ร่วมกัน (Shared)
                    </div>
                    <div className="grid grid-cols-1 gap-1.5">
                      {sopInputData.sharedAlertKeys.map((item) => (
                        <label key={`shared-${item.key}`} className="text-[10px] font-semibold text-indigo-900">
                          <div className="flex items-center justify-between">
                            <span className="font-mono">alert.{item.key}</span>
                            <span className="text-[9px] font-normal text-indigo-700">
                              ใช้ร่วมกัน {item.blockCount} บล็อก
                            </span>
                          </div>
                          <input
                            type="text"
                            value={alertContext[item.key] || ''}
                            placeholder={`กรอก ${item.key} (ใช้ทุกบล็อกที่ผูก)...`}
                            onChange={(e) =>
                              setAlertContext((p) => ({ ...p, [item.key]: e.target.value }))
                            }
                            className="mt-0.5 w-full rounded border border-indigo-200 bg-white px-1.5 py-1 font-mono text-[11px] font-normal"
                          />
                        </label>
                      ))}
                    </div>
                  </div>
                )}

                {sopInputData.blocks.map((block) => {
                  const nodeStatus = statuses[block.nodeId] || 'idle'
                  const isSkipped = pipelineState.nodeStates[block.nodeId]?.availability === 'skipped'
                  const statusDot =
                    nodeStatus === 'running'
                      ? 'bg-blue-500 animate-pulse'
                      : nodeStatus === 'success'
                        ? 'bg-emerald-500'
                        : nodeStatus === 'failed'
                          ? 'bg-rose-500'
                          : 'bg-slate-300'

                  return (
                    <div
                      key={`block-${block.nodeId}`}
                      className={`rounded border p-2 transition-opacity ${
                        isSkipped
                          ? 'border-slate-200 bg-slate-50 opacity-50'
                          : 'border-slate-200 bg-slate-50/80'
                      }`}
                    >
                      <div className="flex items-center justify-between gap-1.5 mb-1.5">
                        <div className="flex items-center gap-1 min-w-0">
                          <span
                            className={`shrink-0 rounded px-1 py-0.5 text-[8px] font-bold uppercase ${
                              block.nodeType === 'lookup'
                                ? 'bg-sky-200 text-sky-900'
                                : 'bg-slate-200 text-slate-800'
                            }`}
                          >
                            {block.nodeType}
                          </span>
                          <span className="truncate text-[11px] font-semibold text-slate-800" title={block.label}>
                            {block.stepIndex}. {block.label}
                          </span>
                        </div>
                        <div className="flex items-center gap-1 shrink-0">
                          <span className="text-[9px] font-medium text-slate-500">{block.toolName}</span>
                          <span className={`inline-block h-2 w-2 rounded-full ${statusDot}`} title={nodeStatus} />
                        </div>
                      </div>

                      <div className="space-y-1.5">
                        {block.fields.map((field) => {
                          const missingRequired =
                            field.required && !field.runtimeVal.trim() && !field.sharedHint
                          return (
                            <label key={`block-${block.nodeId}-${field.key}`} className="block text-[10px]">
                              <div className="flex items-center justify-between gap-1 font-mono font-semibold text-slate-700">
                                <span>
                                  {field.isSecret ? '🔑 ' : ''}
                                  {field.key}
                                  {field.required && <span className="text-rose-600">*</span>}
                                </span>
                                <span className="text-[9px] font-normal text-slate-400">
                                  {field.location}
                                </span>
                              </div>
                              <input
                                type={field.isSecret ? 'password' : 'text'}
                                value={field.runtimeVal}
                                disabled={isSkipped}
                                placeholder={
                                  field.sharedHint
                                    ? `Shared: ${field.sharedHint}`
                                    : `กรอก ${field.key}...`
                                }
                                onChange={(e) =>
                                  handleChangeRuntimeInput(block.nodeId, field.key, e.target.value)
                                }
                                className={`mt-0.5 w-full rounded border px-1.5 py-1 font-mono text-[11px] font-normal outline-none ${
                                  missingRequired
                                    ? 'border-rose-300 bg-rose-50/70 text-rose-900 placeholder:text-rose-400'
                                    : 'border-slate-300 bg-white text-slate-900 placeholder:text-slate-400 focus:border-blue-500'
                                }`}
                              />
                            </label>
                          )
                        })}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}

            <div className="mt-2 grid grid-cols-2 gap-1.5">
              <button
                type="button"
                onClick={() => handleRunAll(hasPendingReadyStep)}
                disabled={nodes.length === 0 || isRunningAll}
                className="rounded bg-indigo-600 py-1.5 text-[11px] font-bold text-white hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {isRunningAll ? 'กำลังรัน...' : hasPendingReadyStep ? '▶ รันต่อที่เหลือ' : '▶ รันทั้งหมด'}
              </button>
              <button
                type="button"
                onClick={handleResetExecution}
                disabled={
                  nodes.length === 0 &&
                  Object.keys(runtimeInputs).length === 0 &&
                  Object.keys(alertContext).length === 0 &&
                  Object.keys(sessionAuthKeys).length === 0
                }
                className="rounded border border-slate-300 bg-white py-1.5 text-[11px] font-bold text-slate-700 hover:bg-slate-100 disabled:opacity-50"
              >
                ↺ รีเซ็ต / ล้างค่า
              </button>
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
                    setNodeOutputs({})
                    setRuntimeInputs({})
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

      <main className="flex flex-1 min-w-0 flex-col overflow-hidden">
        <div className="flex-1 min-h-0 print-canvas">
          <ReactFlow
            nodes={rfNodes}
            edges={rfEdges}
            nodeTypes={nodeTypes}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onEdgeClick={(_event, clickedEdge) => {
              const sourceNode = nodesRef.current.find((n) => n.id === clickedEdge.source)
              if (sourceNode?.type === 'decision') {
                setEditingEdgeId(clickedEdge.id)
              }
            }}
            fitView
          >
            <Background gap={16} size={1} />
            <Controls className="no-print" />
          </ReactFlow>
        </div>

        <div className="no-print h-40 shrink-0 border-t border-slate-200 bg-slate-900 text-slate-100 p-2.5 overflow-y-auto font-mono text-xs">
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
              <pre className="text-[11px] text-slate-300 whitespace-pre-wrap break-all">
                {JSON.stringify(log.output, null, 2)}
              </pre>
            </div>
          ))}
        </div>
      </main>

      <ToolManagerModal
        open={toolModalOpen}
        tools={tools}
        loadError={toolsLoadError}
        initialCategory={toolModalCategory}
        initialEditToolId={toolModalEditId}
        runtimeContext={{ alert: alertContext }}
        onClose={() => {
          setToolModalOpen(false)
          setToolModalEditId(null)
          setToolModalCategory(undefined)
        }}
        onChanged={reloadTools}
      />

      <LookupPayloadModal
        node={nodes.find((n) => n.id === editingPayloadNodeId) ?? null}
        tool={tools.find((t) => t.id === nodes.find((n) => n.id === editingPayloadNodeId)?.tool)}
        runtimeContext={{
          alert: alertContext,
          prev: editingPayloadNodeId
            ? (pipelineState.nodeStates[editingPayloadNodeId]?.upstreamOutput ?? {})
            : {},
          nodes: nodeOutputs,
        }}
        runtimeInputs={editingPayloadNodeId ? runtimeInputs[editingPayloadNodeId] : undefined}
        onClose={() => setEditingPayloadNodeId(null)}
        onSave={updateNodePayload}
        onEditTool={(toolId) => {
          setToolModalCategory(undefined)
          setToolModalEditId(toolId)
          setToolModalOpen(true)
        }}
      />

      <EdgeConditionModal
        edge={edges.find((e) => e.id === editingEdgeId) ?? null}
        sourceLabel={nodes.find((n) => n.id === edges.find((e) => e.id === editingEdgeId)?.source)?.label}
        targetLabel={nodes.find((n) => n.id === edges.find((e) => e.id === editingEdgeId)?.target)?.label}
        onClose={() => setEditingEdgeId(null)}
        onSave={updateEdgeCondition}
      />
    </div>
  )
}