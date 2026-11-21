import type {
  AuthUser,
  ExecutionMode,
  FlowNode,
  FlowEdge,
  PlaybookItem,
  PlaybookVersionSnapshot,
  RunEvent,
  RunScore,
  ScenarioItem,
  ScenarioPayload,
  ScenarioVerdict,
  SimulationRun,
  ToolDefinition,
  ToolPayload,
} from './types'

const AUTH_TOKEN_KEY = 'soc_playbook_auth_token'

export function getStoredAuthToken(): string {
  try {
    return typeof localStorage !== 'undefined' ? localStorage.getItem(AUTH_TOKEN_KEY) || '' : ''
  } catch {
    return ''
  }
}

export function setStoredAuthToken(token: string) {
  try {
    if (typeof localStorage === 'undefined') return
    if (token) localStorage.setItem(AUTH_TOKEN_KEY, token)
    else localStorage.removeItem(AUTH_TOKEN_KEY)
  } catch {
    // ignore storage errors
  }
}

function authFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const token = getStoredAuthToken()
  const headers = new Headers(init?.headers)
  if (token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  return fetch(input, { ...init, headers })
}

async function parseJsonSafe<T = Record<string, unknown>>(res: Response): Promise<T | null> {
  try {
    return (await res.json()) as T
  } catch {
    return null
  }
}

export interface RuntimeConfig {
  allow_live_execution: boolean
  auth_enabled: boolean
}

export async function getRuntimeConfig(): Promise<RuntimeConfig> {
  try {
    const res = await authFetch('/api/config')
    if (!res.ok) return { allow_live_execution: false, auth_enabled: false }
    const data = await parseJsonSafe<Partial<RuntimeConfig>>(res)
    return {
      allow_live_execution: Boolean(data?.allow_live_execution),
      auth_enabled: Boolean(data?.auth_enabled),
    }
  } catch {
    return { allow_live_execution: false, auth_enabled: false }
  }
}

export async function login(username: string, password: string): Promise<{ token: string; user: AuthUser }> {
  const res = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  const data = await parseJsonSafe<{ token?: string; user?: AuthUser; error?: string }>(res)
  if (!res.ok || !data?.token || !data?.user) {
    throw new Error(data?.error || `เข้าสู่ระบบไม่สำเร็จ (HTTP ${res.status})`)
  }
  setStoredAuthToken(data.token)
  return { token: data.token, user: data.user }
}

export async function registerUser(payload: {
  username: string
  password: string
  display_name?: string
}): Promise<{ token: string; user: AuthUser }> {
  const res = await authFetch('/api/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const data = await parseJsonSafe<{ token?: string; user?: AuthUser; error?: string }>(res)
  if (!res.ok || !data?.token || !data?.user) {
    throw new Error(data?.error || `สมัครสมาชิกไม่สำเร็จ (HTTP ${res.status})`)
  }
  setStoredAuthToken(data.token)
  return { token: data.token, user: data.user }
}

export async function getMe(): Promise<AuthUser | null> {
  try {
    const res = await authFetch('/api/auth/me')
    if (!res.ok) {
      if (res.status === 401) setStoredAuthToken('')
      return null
    }
    const data = await parseJsonSafe<AuthUser>(res)
    return data && data.id ? data : null
  } catch {
    return null
  }
}

export async function generateFlowchart(form: FormData) {
  const res = await authFetch('/api/generate', { method: 'POST', body: form })
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
    const res = await authFetch('/api/sops')
    if (!res.ok) return []
    const data = await parseJsonSafe<unknown>(res)
    return Array.isArray(data) ? (data as PlaybookItem[]) : []
  } catch {
    return []
  }
}

export async function getPlaybookVersions(playbookId: string): Promise<PlaybookVersionSnapshot[]> {
  try {
    const res = await authFetch(`/api/sops/${encodeURIComponent(playbookId)}/versions`)
    if (!res.ok) return []
    const data = await parseJsonSafe<unknown>(res)
    return Array.isArray(data) ? (data as PlaybookVersionSnapshot[]) : []
  } catch {
    return []
  }
}

export async function savePlaybook(payload: Omit<PlaybookItem, 'updated_at'>): Promise<PlaybookItem> {
  const res = await authFetch('/api/sops', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const data = await parseJsonSafe<PlaybookItem & { error?: string }>(res)
  if (!res.ok || !data) throw new Error(data?.error || `บันทึกไม่สำเร็จ (HTTP ${res.status})`)
  return data
}

export async function deletePlaybook(id: string) {
  const res = await authFetch(`/api/sops/${encodeURIComponent(id)}`, { method: 'DELETE' })
  if (!res.ok) {
    const data = await parseJsonSafe<{ error?: string }>(res)
    throw new Error(data?.error || `ลบไม่สำเร็จ (HTTP ${res.status})`)
  }
}

export async function getTools(): Promise<ToolDefinition[]> {
  const res = await authFetch('/api/tools')
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
  const res = await authFetch('/api/tools', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const data = await parseJsonSafe<ToolDefinition & { error?: string }>(res)
  if (!res.ok || !data) throw new Error(data?.error || `สร้าง Tool ไม่สำเร็จ (HTTP ${res.status})`)
  return data
}

export async function updateTool(id: string, payload: ToolPayload): Promise<ToolDefinition> {
  const res = await authFetch(`/api/tools/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const data = await parseJsonSafe<ToolDefinition & { error?: string }>(res)
  if (!res.ok || !data) throw new Error(data?.error || `อัปเดต Tool ไม่สำเร็จ (HTTP ${res.status})`)
  return data
}

export async function deleteTool(id: string) {
  const res = await authFetch(`/api/tools/${encodeURIComponent(id)}`, { method: 'DELETE' })
  if (!res.ok) {
    const data = await parseJsonSafe<{ error?: string }>(res)
    throw new Error(data?.error || `ลบ Tool ไม่สำเร็จ (HTTP ${res.status})`)
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
  const res = await authFetch('/api/tools/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tool, runtime_context: runtimeContext }),
  })
  const data = await parseJsonSafe<ToolTestResponse & { error?: string }>(res)
  if (!res.ok || !data) throw new Error(data?.error || `ทดสอบ Tool ไม่สำเร็จ (HTTP ${res.status})`)
  return data
}

export async function getScenarios(hideAnswers = true): Promise<ScenarioItem[]> {
  try {
    const query = hideAnswers ? '?hide_answers=true' : ''
    const res = await authFetch(`/api/scenarios${query}`)
    if (!res.ok) return []
    const data = await parseJsonSafe<ScenarioItem[]>(res)
    return Array.isArray(data) ? data : []
  } catch {
    return []
  }
}

export async function createScenario(payload: ScenarioPayload): Promise<ScenarioItem> {
  const res = await authFetch('/api/scenarios', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const data = await parseJsonSafe<ScenarioItem & { error?: string }>(res)
  if (!res.ok || !data) throw new Error(data?.error || `สร้าง Scenario ไม่สำเร็จ (HTTP ${res.status})`)
  return data
}

export async function updateScenario(id: string, payload: ScenarioPayload): Promise<ScenarioItem> {
  const res = await authFetch(`/api/scenarios/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const data = await parseJsonSafe<ScenarioItem & { error?: string }>(res)
  if (!res.ok || !data) throw new Error(data?.error || `อัปเดต Scenario ไม่สำเร็จ (HTTP ${res.status})`)
  return data
}

export async function deleteScenario(id: string): Promise<void> {
  const res = await authFetch(`/api/scenarios/${encodeURIComponent(id)}`, { method: 'DELETE' })
  if (!res.ok) {
    const data = await parseJsonSafe<{ error?: string }>(res)
    throw new Error(data?.error || `ลบ Scenario ไม่สำเร็จ (HTTP ${res.status})`)
  }
}

export async function startSimulationRun(payload: {
  playbook_id?: string
  playbook_version?: number
  playbook_title?: string
  scenario_id?: string
  nodes?: FlowNode[]
  edges?: FlowEdge[]
  alert?: Record<string, unknown>
  mode?: ExecutionMode
}): Promise<{ run: SimulationRun; events: RunEvent[] }> {
  const res = await authFetch('/api/runs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const data = await parseJsonSafe<{ run?: SimulationRun; events?: RunEvent[]; error?: string }>(res)
  if (!res.ok || !data?.run) throw new Error(data?.error || `เริ่มรอบฝึกไม่สำเร็จ (HTTP ${res.status})`)
  return { run: data.run, events: data.events || [] }
}

export async function listSimulationRuns(): Promise<SimulationRun[]> {
  const res = await authFetch('/api/runs')
  if (!res.ok) {
    const data = await parseJsonSafe<{ error?: string }>(res)
    throw new Error(data?.error || `โหลดประวัติรอบฝึกไม่สำเร็จ (HTTP ${res.status})`)
  }
  const data = await parseJsonSafe<SimulationRun[]>(res)
  return Array.isArray(data) ? data : []
}

export async function getSimulationRun(runId: string): Promise<{ run: SimulationRun; events: RunEvent[] }> {
  const res = await authFetch(`/api/runs/${encodeURIComponent(runId)}`)
  const data = await parseJsonSafe<{ run?: SimulationRun; events?: RunEvent[]; error?: string }>(res)
  if (!res.ok || !data?.run) throw new Error(data?.error || `โหลดรอบฝึกไม่สำเร็จ (HTTP ${res.status})`)
  return { run: data.run, events: data.events || [] }
}

export async function executeRunStep(
  runId: string,
  nodeId: string,
  payload: {
    runtime_inputs?: Record<string, string>
    runtime_auth?: string
    manual_verdict?: 'clean' | 'suspicious' | 'malicious' | 'done'
    notes?: string
  }
): Promise<{ run: SimulationRun; event: RunEvent }> {
  const res = await authFetch(`/api/runs/${encodeURIComponent(runId)}/steps/${encodeURIComponent(nodeId)}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const data = await parseJsonSafe<{ run?: SimulationRun; event?: RunEvent; error?: string }>(res)
  if (!res.ok || !data?.run || !data?.event) {
    throw new Error(data?.error || `รันขั้นตอนไม่สำเร็จ (HTTP ${res.status})`)
  }
  return { run: data.run, event: data.event }
}

export async function completeSimulationRun(
  runId: string,
  finalVerdict: ScenarioVerdict,
  summary?: string
): Promise<{ run: SimulationRun; score: RunScore; events: RunEvent[] }> {
  const res = await authFetch(`/api/runs/${encodeURIComponent(runId)}/complete`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ final_verdict: finalVerdict, summary }),
  })
  const data = await parseJsonSafe<{
    run?: SimulationRun
    score?: RunScore
    events?: RunEvent[]
    error?: string
  }>(res)
  if (!res.ok || !data?.run || !data?.score) {
    throw new Error(data?.error || `ส่งคำตอบไม่สำเร็จ (HTTP ${res.status})`)
  }
  return { run: data.run, score: data.score, events: data.events || [] }
}

export async function executeStep(
  node: FlowNode,
  runtimeContext?: Record<string, unknown>,
  runtimeInputs?: Record<string, string>,
  options?: { runtimeAuth?: string; executionMode?: ExecutionMode; scenarioId?: string }
) {
  const res = await authFetch('/api/execute', {
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
      scenario_id: options?.scenarioId || undefined,
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
  const res = await authFetch(`/api/export/${format}`, {
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