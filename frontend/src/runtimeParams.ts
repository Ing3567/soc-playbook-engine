import type { FlowEdge, FlowNode, PayloadMode, ToolDefinition } from './types'

export interface NodeParamItem {
  key: string
  label: string
  location: string
  mode: PayloadMode
  blueprintVal: string
  runtimeVal: string
  sharedHint: string
  alertKeys: string[]
  required: boolean
  isSecret: boolean
  isPipeTemplate: boolean
}

export interface SopInputBlock {
  nodeId: string
  stepIndex: number
  nodeType: 'lookup' | 'action'
  label: string
  toolId: string
  toolName: string
  fields: NodeParamItem[]
}

export interface SharedAlertKeyInfo {
  key: string
  blockCount: number
}

export interface SopInputSummary {
  blocks: SopInputBlock[]
  sharedAlertKeys: SharedAlertKeyInfo[]
}

const ALERT_VAR_REGEX = /\{\{\s*alert\.([a-zA-Z0-9_]+)\s*\}\}/g

function extractAlertKeys(expr: string): string[] {
  const keys: string[] = []
  const seen = new Set<string>()
  let match: RegExpExecArray | null
  const rx = new RegExp(ALERT_VAR_REGEX.source, 'g')
  while ((match = rx.exec(expr)) !== null) {
    const k = match[1]
    if (k && !seen.has(k)) {
      seen.add(k)
      keys.push(k)
    }
  }
  return keys
}

function resolveAlertHint(expr: string, sharedAlertContext: Record<string, string>): string {
  if (!expr.includes('{{alert.')) return ''
  let anyFilled = false
  const resolved = expr.replace(ALERT_VAR_REGEX, (_, k: string) => {
    const v = sharedAlertContext[k]?.trim() || ''
    if (v) anyFilled = true
    return v
  })
  return anyFilled ? resolved : ''
}

export function getNodeParamItems(
  node: Pick<FlowNode, 'type' | 'tool' | 'payload_mode' | 'param_modes' | 'input_payload'>,
  tools: ToolDefinition[],
  nodeRuntimeInputs: Record<string, string> = {},
  sharedAlertContext: Record<string, string> = {}
): {
  paramItems: NodeParamItem[]
  fixedOrPipeItems: NodeParamItem[]
  runtimeInputItems: NodeParamItem[]
} {
  if (node.type !== 'lookup' && node.type !== 'action') {
    return { paramItems: [], fixedOrPipeItems: [], runtimeInputItems: [] }
  }

  const canonicalToolId = node.tool === 'jira_ticket' ? 'ticket' : node.tool
  const matchedTool = tools.find((t) => t.id === canonicalToolId)
  const effectiveMode: PayloadMode = node.payload_mode || matchedTool?.payload_mode || 'dynamic'
  const paramModes = node.param_modes || {}
  const inputPayload = node.input_payload || {}

  const paramItems: NodeParamItem[] = []
  const seen = new Set<string>()

  for (const p of matchedTool?.params || []) {
    seen.add(p.key)
    const pMode: PayloadMode = paramModes[p.key] || p.mode || effectiveMode
    const rawDef = p.value ?? p.default_value ?? ''
    const blueprintVal =
      inputPayload[p.key] ??
      (pMode === 'fixed' || (rawDef.includes('{{') && rawDef.includes('}}')) ? rawDef : '')
    const runtimeVal = nodeRuntimeInputs[p.key] ?? ''
    const isPipeTemplate = pMode === 'dynamic' && blueprintVal.includes('{{prev.')
    const sharedHint = pMode === 'dynamic' && !isPipeTemplate ? resolveAlertHint(blueprintVal, sharedAlertContext) : ''
    const alertKeys = pMode === 'dynamic' && !isPipeTemplate ? extractAlertKeys(blueprintVal) : []

    paramItems.push({
      key: p.key,
      label: p.label || p.key,
      location: p.in.toUpperCase(),
      mode: pMode,
      blueprintVal,
      runtimeVal,
      sharedHint,
      alertKeys,
      required: Boolean(p.required),
      isSecret: Boolean(p.is_secret),
      isPipeTemplate,
    })
  }

  for (const [k, v] of Object.entries(inputPayload)) {
    if (seen.has(k)) continue
    seen.add(k)
    const pMode: PayloadMode = paramModes[k] || effectiveMode
    const isPipeTemplate = pMode === 'dynamic' && v.includes('{{prev.')
    const sharedHint = pMode === 'dynamic' && !isPipeTemplate ? resolveAlertHint(v, sharedAlertContext) : ''
    const alertKeys = pMode === 'dynamic' && !isPipeTemplate ? extractAlertKeys(v) : []

    paramItems.push({
      key: k,
      label: k,
      location: 'PARAM',
      mode: pMode,
      blueprintVal: v,
      runtimeVal: nodeRuntimeInputs[k] ?? '',
      sharedHint,
      alertKeys,
      required: false,
      isSecret: false,
      isPipeTemplate,
    })
  }

  return {
    paramItems,
    fixedOrPipeItems: paramItems.filter((item) => item.mode === 'fixed' || item.isPipeTemplate),
    runtimeInputItems: paramItems.filter((item) => item.mode === 'dynamic' && !item.isPipeTemplate),
  }
}

export function buildNodeResolvedUrl(
  tool: ToolDefinition | undefined,
  paramItems: NodeParamItem[]
): string {
  const endpoint = tool?.endpoint?.trim() || ''
  if (!endpoint) return ''

  let url = endpoint
  const queryPairs: Array<[string, string]> = []

  for (const item of paramItems) {
    const val =
      item.mode === 'fixed'
        ? item.blueprintVal.trim()
        : item.runtimeVal.trim() || item.sharedHint.trim() || ''

    if (item.location === 'PATH') {
      if (val) {
        url = url.replaceAll(`{{${item.key}}}`, encodeURIComponent(val))
      }
    } else if (item.location === 'QUERY' && val) {
      queryPairs.push([item.key, val])
    }
  }

  if (queryPairs.length > 0) {
    const qs = queryPairs.map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`).join('&')
    url += (url.includes('?') ? '&' : '?') + qs
  }

  return url
}

export function buildSopInputBlocks(
  nodes: FlowNode[],
  edges: FlowEdge[],
  tools: ToolDefinition[],
  runtimeInputs: Record<string, Record<string, string>> = {},
  sharedAlertContext: Record<string, string> = {}
): SopInputSummary {
  if (nodes.length === 0) {
    return { blocks: [], sharedAlertKeys: [] }
  }

  const nodeMap = new Map(nodes.map((n) => [n.id, n]))
  const outgoing = new Map<string, string[]>()
  const inDegree = new Map<string, number>()
  for (const n of nodes) inDegree.set(n.id, 0)
  for (const e of edges) {
    const list = outgoing.get(e.source) ?? []
    list.push(e.target)
    outgoing.set(e.source, list)
    inDegree.set(e.target, (inDegree.get(e.target) ?? 0) + 1)
  }

  const orderedNodes: FlowNode[] = []
  const visited = new Set<string>()
  const queue: string[] = []

  for (const n of nodes) {
    if ((inDegree.get(n.id) ?? 0) === 0) {
      queue.push(n.id)
    }
  }

  const bfsFrom = (startIds: string[]) => {
    const q = [...startIds]
    while (q.length > 0) {
      const id = q.shift()!
      if (visited.has(id)) continue
      visited.add(id)
      const node = nodeMap.get(id)
      if (node) orderedNodes.push(node)
      for (const nextId of outgoing.get(id) ?? []) {
        if (!visited.has(nextId)) q.push(nextId)
      }
    }
  }

  bfsFrom(queue)
  for (const n of nodes) {
    if (!visited.has(n.id)) {
      bfsFrom([n.id])
    }
  }

  const blocks: SopInputBlock[] = []
  const alertBlockCounts = new Map<string, number>()
  let stepIndex = 0

  for (const n of orderedNodes) {
    if (n.type !== 'lookup' && n.type !== 'action') continue
    const { runtimeInputItems } = getNodeParamItems(n, tools, runtimeInputs[n.id], sharedAlertContext)
    if (runtimeInputItems.length === 0) continue

    stepIndex += 1
    const canonicalToolId = n.tool === 'jira_ticket' ? 'ticket' : n.tool
    const matchedTool = tools.find((t) => t.id === canonicalToolId)
    const toolName = matchedTool?.name || (canonicalToolId === 'ticket' ? 'Ticket' : canonicalToolId)

    blocks.push({
      nodeId: n.id,
      stepIndex,
      nodeType: n.type,
      label: n.label,
      toolId: canonicalToolId,
      toolName,
      fields: runtimeInputItems,
    })

    const keysInBlock = new Set<string>()
    for (const f of runtimeInputItems) {
      for (const ak of f.alertKeys) {
        keysInBlock.add(ak)
      }
    }
    for (const ak of keysInBlock) {
      alertBlockCounts.set(ak, (alertBlockCounts.get(ak) ?? 0) + 1)
    }
  }

  const sharedAlertKeys: SharedAlertKeyInfo[] = []
  for (const [key, count] of alertBlockCounts.entries()) {
    if (count >= 2) {
      sharedAlertKeys.push({ key, blockCount: count })
    }
  }

  return { blocks, sharedAlertKeys }
}
