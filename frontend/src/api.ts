import type { ExecutionMode, FlowNode, FlowEdge, PlaybookItem, ToolDefinition, ToolPayload } from './types'

async function parseJsonSafe<T = Record<string, unknown>>(res: Response): Promise<T | null> {
  try {
    return (await res.json()) as T
  } catch {
    return null
  }
}

export async function generateFlowchart(form: FormData) {
  const res = await fetch('/api/generate', { method: 'POST', body: form })
  const data = await parseJsonSafe<{
    nodes?: FlowNode[]
    edges?: FlowEdge[]
    engine?: string
    lines?: { level: number; text: string }[]
    error?: string
  }>(res)
  if (!res.ok) throw new Error(data?.error || `สร้างกราฟไม่สำเร็จ (HTTP ${res.status})`)
  return {
    nodes: data?.nodes || [],
    edges: data?.edges || [],
    engine: data?.engine || '-',
    lines: data?.lines || [],
  }
}

export async function getPlaybooks(): Promise<PlaybookItem[]> {
  try {
    const res = await fetch('/api/sops')
    if (!res.ok) return []
    const data = await parseJsonSafe<unknown>(res)
    return Array.isArray(data) ? (data as PlaybookItem[]) : []
  } catch {
    return []
  }
}

export async function savePlaybook(payload: Omit<PlaybookItem, 'updated_at'>): Promise<PlaybookItem> {
  const res = await fetch('/api/sops', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const data = await parseJsonSafe<PlaybookItem & { error?: string }>(res)
  if (!res.ok || !data) throw new Error(data?.error || `บันทึกไม่สำเร็จ (HTTP ${res.status})`)
  return data
}

export async function deletePlaybook(id: string) {
  const res = await fetch(`/api/sops/${encodeURIComponent(id)}`, { method: 'DELETE' })
  if (!res.ok) {
    const data = await parseJsonSafe<{ error?: string }>(res)
    throw new Error(data?.error || `ลบไม่สำเร็จ (HTTP ${res.status})`)
  }
}

export async function getTools(): Promise<ToolDefinition[]> {
  const res = await fetch('/api/tools')
  const data = await parseJsonSafe<unknown>(res)
  if (!res.ok) {
    const errMsg =
      data && typeof data === 'object' && 'error' in data && typeof (data as { error?: unknown }).error === 'string'
        ? (data as { error: string }).error
        : `โหลดรายการ Tools ไม่สำเร็จ (HTTP ${res.status})`
    throw new Error(errMsg)
  }
  return Array.isArray(data) ? (data as ToolDefinition[]) : []
}

export async function createTool(payload: ToolPayload): Promise<ToolDefinition> {
  const res = await fetch('/api/tools', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const data = await parseJsonSafe<ToolDefinition & { error?: string }>(res)
  if (!res.ok || !data) throw new Error(data?.error || `สร้าง Tool ไม่สำเร็จ (HTTP ${res.status})`)
  return data
}

export async function updateTool(id: string, payload: ToolPayload): Promise<ToolDefinition> {
  const res = await fetch(`/api/tools/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const data = await parseJsonSafe<ToolDefinition & { error?: string }>(res)
  if (!res.ok || !data) throw new Error(data?.error || `อัปเดต Tool ไม่สำเร็จ (HTTP ${res.status})`)
  return data
}

export async function deleteTool(id: string) {
  const res = await fetch(`/api/tools/${encodeURIComponent(id)}`, { method: 'DELETE' })
  if (!res.ok) {
    const data = await parseJsonSafe<{ error?: string }>(res)
    throw new Error(data?.error || `ลบ Tool ไม่สำเร็จ (HTTP ${res.status})`)
  }
}

export interface RuntimeConfig {
  allow_live_execution: boolean
}

export async function getRuntimeConfig(): Promise<RuntimeConfig> {
  try {
    const res = await fetch('/api/config')
    if (!res.ok) return { allow_live_execution: false }
    const data = await parseJsonSafe<Partial<RuntimeConfig>>(res)
    return { allow_live_execution: Boolean(data?.allow_live_execution) }
  } catch {
    return { allow_live_execution: false }
  }
}

export interface ToolTestResponse {
  request: {
    method: string
    url: string
    payload_mode: string
    params?: Record<string, string>
    headers?: Record<string, string>
    body?: Record<string, unknown>
    auth?: {
      type: string
      in?: string
      key_name?: string
      env_var?: string
      source: string
      configured: boolean
      masked_preview?: string
    }
    missing_required?: string[]
  }
  result: {
    node_id: string
    status: string
    duration_ms: number
    timestamp: string
    output: Record<string, unknown>
  }
}

export async function testToolDraft(
  tool: ToolPayload,
  runtimeContext?: Record<string, unknown>
): Promise<ToolTestResponse> {
  const res = await fetch('/api/tools/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tool, runtime_context: runtimeContext }),
  })
  const data = await parseJsonSafe<ToolTestResponse & { error?: string }>(res)
  if (!res.ok || !data) throw new Error(data?.error || `ทดสอบ Tool ไม่สำเร็จ (HTTP ${res.status})`)
  return data
}

export async function executeStep(
  node: FlowNode,
  runtimeContext?: Record<string, unknown>,
  runtimeInputs?: Record<string, string>,
  options?: { runtimeAuth?: string; executionMode?: ExecutionMode }
) {
  const res = await fetch('/api/execute', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      node_id: node.id,
      type: node.type,
      label: node.label,
      tool: node.tool,
      payload_mode: node.payload_mode,
      param_modes: node.param_modes,
      input_payload: node.input_payload,
      runtime_inputs: runtimeInputs,
      runtime_context: runtimeContext,
      runtime_auth: options?.runtimeAuth || undefined,
      execution_mode: options?.executionMode || 'sandbox',
    }),
  })
  const data = await parseJsonSafe<{
    node_id: string
    status: string
    duration_ms: number
    timestamp: string
    output: Record<string, unknown>
    error?: string
  }>(res)
  if (!res.ok || !data) throw new Error(data?.error || `รันไม่สำเร็จ (HTTP ${res.status})`)
  return data
}

export async function downloadExportFile(
  format: 'bpmn' | 'cacao' | 'markdown',
  title: string,
  nodes: FlowNode[],
  edges: FlowEdge[]
) {
  const res = await fetch(`/api/export/${format}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title, nodes, edges }),
  })
  if (!res.ok) {
    const data = await parseJsonSafe<{ error?: string }>(res)
    throw new Error(data?.error || `ส่งออกไฟล์ไม่สำเร็จ (HTTP ${res.status})`)
  }
  const content = await res.text()
  const extMap = { bpmn: '.bpmn', cacao: '.cacao.json', markdown: '.md' }
  const safeName = (title || 'playbook').replace(/[<>:"/\\|?*]+/g, '_').trim() || 'playbook'
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `${safeName}${extMap[format]}`
  link.click()
  URL.revokeObjectURL(url)
}