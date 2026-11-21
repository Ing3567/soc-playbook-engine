import type { EdgeCondition, FlowEdge, FlowNode, RunStatus } from './types'

export type PipelineNodeAvailability = 'ready' | 'locked' | 'skipped'

export interface PipelineNodeState {
  availability: PipelineNodeAvailability
  upstreamOutput: Record<string, unknown> | null
  lockedReason?: string
}

export interface PipelineState {
  nodeStates: Record<string, PipelineNodeState>
  traversedEdgeIds: Set<string>
  skippedEdgeIds: Set<string>
}

export interface DecisionContext {
  alert?: Record<string, unknown>
  nodes?: Record<string, Record<string, unknown>>
}

function toFiniteNumber(v: unknown): number | null {
  if (typeof v === 'number') return Number.isFinite(v) ? v : null
  if (typeof v === 'string') {
    const trimmed = v.trim()
    if (!trimmed) return null
    const parsed = Number(trimmed)
    return Number.isFinite(parsed) ? parsed : null
  }
  return null
}

function isTruthyVal(v: unknown): boolean {
  if (v == null) return false
  if (typeof v === 'boolean') return v
  if (typeof v === 'number') return v !== 0
  if (typeof v === 'string') {
    const s = v.trim().toLowerCase()
    return s !== '' && s !== 'false' && s !== '0' && s !== 'no'
  }
  if (Array.isArray(v)) return v.length > 0
  if (typeof v === 'object') return Object.keys(v).length > 0
  return true
}

function lookupConditionField(
  field: string,
  prevOutput: Record<string, unknown> | null,
  fullCtx?: DecisionContext
): { found: boolean; value: unknown } {
  const path = field.trim()
  if (!path) return { found: false, value: undefined }

  if (!path.includes('.') && prevOutput && path in prevOutput) {
    return { found: true, value: prevOutput[path] }
  }

  const root: Record<string, unknown> = {
    ...(fullCtx?.alert ? { alert: fullCtx.alert } : {}),
    ...(fullCtx?.nodes ? { nodes: fullCtx.nodes } : {}),
    ...(prevOutput ? { prev: prevOutput } : {}),
  }

  let cur: unknown = root
  for (const part of path.split('.')) {
    if (cur == null || typeof cur !== 'object' || !(part in (cur as Record<string, unknown>))) {
      return { found: false, value: undefined }
    }
    cur = (cur as Record<string, unknown>)[part]
  }
  return { found: true, value: cur }
}

function valuesEqual(a: unknown, b: unknown): boolean {
  if (a == null && b == null) return true
  if (a == null || b == null) return false
  if (typeof a === 'boolean') {
    if (typeof b === 'boolean') return a === b
    if (typeof b === 'string') return String(a) === b.trim().toLowerCase()
  }
  const na = toFiniteNumber(a)
  const nb = toFiniteNumber(b)
  if (na !== null && nb !== null) return na === nb
  return String(a).trim().toLowerCase() === String(b).trim().toLowerCase()
}

export function evaluateEdgeCondition(
  cond: EdgeCondition | undefined,
  prevOutput: Record<string, unknown> | null,
  fullCtx?: DecisionContext
): boolean {
  if (!cond || !cond.field.trim()) return false
  const { found, value: actual } = lookupConditionField(cond.field, prevOutput, fullCtx)
  const op = cond.op

  switch (op) {
    case 'is_true':
      return found && isTruthyVal(actual)
    case 'is_false':
      return !found || !isTruthyVal(actual)
    case '==':
      return found && valuesEqual(actual, cond.value)
    case '!=':
      return !found ? cond.value != null : !valuesEqual(actual, cond.value)
    case '>': {
      const a = toFiniteNumber(actual)
      const b = toFiniteNumber(cond.value)
      return found && a !== null && b !== null && a > b
    }
    case '>=': {
      const a = toFiniteNumber(actual)
      const b = toFiniteNumber(cond.value)
      return found && a !== null && b !== null && a >= b
    }
    case '<': {
      const a = toFiniteNumber(actual)
      const b = toFiniteNumber(cond.value)
      return found && a !== null && b !== null && a < b
    }
    case '<=': {
      const a = toFiniteNumber(actual)
      const b = toFiniteNumber(cond.value)
      return found && a !== null && b !== null && a <= b
    }
    case 'contains': {
      if (!found || actual == null || cond.value == null) return false
      const needle = String(cond.value).trim().toLowerCase()
      if (!needle) return false
      if (Array.isArray(actual)) {
        return actual.some((el) => String(el).trim().toLowerCase().includes(needle))
      }
      return String(actual).toLowerCase().includes(needle)
    }
    default:
      return false
  }
}

export function formatEdgeCondition(cond: EdgeCondition | undefined): string {
  if (!cond || !cond.field.trim()) return ''
  const field = cond.field.replace(/^prev\./, '')
  if (cond.op === 'is_true') return `${field} is true`
  if (cond.op === 'is_false') return `${field} is false`
  return `${field} ${cond.op} ${cond.value ?? ''}`
}

export function pickDecisionEdge(
  outEdges: FlowEdge[],
  lastOutput: Record<string, unknown> | null,
  fullCtx?: DecisionContext
): FlowEdge | undefined {
  if (outEdges.length === 0) return undefined
  if (outEdges.length === 1) return outEdges[0]

  const hasStructured = outEdges.some((e) => Boolean(e.condition && e.condition.field.trim()))
  if (hasStructured) {
    let defaultEdge: FlowEdge | undefined
    for (const e of outEdges) {
      if (e.condition && e.condition.field.trim()) {
        if (evaluateEdgeCondition(e.condition, lastOutput, fullCtx)) {
          return e
        }
      } else if (!defaultEdge) {
        defaultEdge = e
      }
    }
    return defaultEdge ?? outEdges[outEdges.length - 1]
  }

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
  return (
    outEdges.find((e) => e.label === 'ไม่ใช่' || e.label?.toLowerCase() === 'no') ??
    outEdges[outEdges.length - 1]
  )
}

export function getDownstreamNodeIds(startId: string, edges: FlowEdge[]): string[] {
  const outgoing = new Map<string, string[]>()
  for (const e of edges) {
    const list = outgoing.get(e.source) ?? []
    list.push(e.target)
    outgoing.set(e.source, list)
  }

  const visited = new Set<string>()
  const queue: string[] = [...(outgoing.get(startId) ?? [])]
  while (queue.length > 0) {
    const cur = queue.shift()!
    if (cur === startId || visited.has(cur)) continue
    visited.add(cur)
    for (const next of outgoing.get(cur) ?? []) {
      if (!visited.has(next)) queue.push(next)
    }
  }
  return Array.from(visited)
}

export function computePipelineState(
  nodes: FlowNode[],
  edges: FlowEdge[],
  statuses: Record<string, RunStatus>,
  nodeOutputs: Record<string, Record<string, unknown>>,
  alertContext?: Record<string, unknown>
): PipelineState {
  const nodeStates: Record<string, PipelineNodeState> = {}
  const traversedEdgeIds = new Set<string>()
  const skippedEdgeIds = new Set<string>()

  if (nodes.length === 0) {
    return { nodeStates, traversedEdgeIds, skippedEdgeIds }
  }

  const nodeMap = new Map<string, FlowNode>()
  const outgoing = new Map<string, FlowEdge[]>()
  const inDegree = new Map<string, number>()

  for (const n of nodes) {
    nodeMap.set(n.id, n)
    inDegree.set(n.id, 0)
  }
  for (const e of edges) {
    const list = outgoing.get(e.source) ?? []
    list.push(e)
    outgoing.set(e.source, list)
    inDegree.set(e.target, (inDegree.get(e.target) ?? 0) + 1)
  }

  const startNode = nodes.find((n) => (inDegree.get(n.id) ?? 0) === 0) ?? nodes[0]
  const visitedActive = new Set<string>()
  const blockingLabels = new Map<string, string>()

  interface QueueItem {
    id: string
    lastOut: Record<string, unknown> | null
  }

  const queue: QueueItem[] = [{ id: startNode.id, lastOut: null }]

  while (queue.length > 0) {
    const { id, lastOut } = queue.shift()!
    const current = nodeMap.get(id)
    if (!current || visitedActive.has(id)) continue
    visitedActive.add(id)

    const outEdges = outgoing.get(id) ?? []

    if (current.type === 'terminal') {
      nodeStates[id] = { availability: 'ready', upstreamOutput: lastOut }
      for (const e of outEdges) {
        traversedEdgeIds.add(e.id)
        queue.push({ id: e.target, lastOut })
      }
      continue
    }

    if (current.type === 'end') {
      nodeStates[id] = { availability: 'ready', upstreamOutput: lastOut }
      continue
    }

    if (current.type === 'decision') {
      nodeStates[id] = { availability: 'ready', upstreamOutput: lastOut }
      const chosen = pickDecisionEdge(outEdges, lastOut, {
        alert: alertContext,
        nodes: nodeOutputs,
      })
      for (const e of outEdges) {
        if (chosen && e.id === chosen.id) {
          traversedEdgeIds.add(e.id)
          queue.push({ id: e.target, lastOut })
        } else {
          skippedEdgeIds.add(e.id)
        }
      }
      continue
    }

    // lookup or action node
    nodeStates[id] = {
      availability: 'ready',
      upstreamOutput: lastOut,
    }

    if (statuses[id] === 'success') {
      const myOut = nodeOutputs[id] ?? lastOut
      for (const e of outEdges) {
        traversedEdgeIds.add(e.id)
        queue.push({ id: e.target, lastOut: myOut })
      }
    } else {
      // Mark downstream nodes reachable from this blocked frontier with a helpful reason
      const downstream = getDownstreamNodeIds(id, edges)
      for (const dId of downstream) {
        if (!blockingLabels.has(dId)) {
          blockingLabels.set(dId, current.label)
        }
      }
    }
  }

  // Determine which unvisited nodes are truly skipped vs locked waiting on an active frontier
  const reachableWithoutSkipped = new Set<string>([startNode.id])
  const reachQueue: string[] = [startNode.id]
  while (reachQueue.length > 0) {
    const cur = reachQueue.shift()!
    for (const e of outgoing.get(cur) ?? []) {
      if (skippedEdgeIds.has(e.id)) continue
      if (!reachableWithoutSkipped.has(e.target)) {
        reachableWithoutSkipped.add(e.target)
        reachQueue.push(e.target)
      }
    }
  }

  for (const n of nodes) {
    if (nodeStates[n.id]) continue
    if (!reachableWithoutSkipped.has(n.id)) {
      nodeStates[n.id] = {
        availability: 'skipped',
        upstreamOutput: null,
        lockedReason: 'กิ่งนี้ไม่ถูกเลือกตามผลลัพธ์ของโหนดก่อนหน้า',
      }
    } else {
      const blocker = blockingLabels.get(n.id)
      nodeStates[n.id] = {
        availability: 'locked',
        upstreamOutput: null,
        lockedReason: blocker ? `ต้องรัน "${blocker}" ให้สำเร็จก่อน` : 'ต้องรันโหนดด้านบนให้สำเร็จก่อน',
      }
    }
  }

  return { nodeStates, traversedEdgeIds, skippedEdgeIds }
}
