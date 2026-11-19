export type ToolSource = 'ai' | 'keyword' | 'manual'
export type ParamLocation = 'path' | 'query' | 'body' | 'header'
export type PayloadMode = 'fixed' | 'dynamic'
export type HTTPMethod = 'GET' | 'POST' | 'PUT' | 'DELETE'

export interface FlowNode {
  id: string
  type: string
  label: string
  tool: string
  tool_source?: ToolSource
  payload_mode?: PayloadMode
  param_modes?: Record<string, PayloadMode>
  input_payload?: Record<string, string>
}

export interface FlowEdge {
  id: string
  source: string
  target: string
  label?: string
}

export interface PlaybookItem {
  id: string
  title: string
  raw_text: string
  engine: string
  nodes: FlowNode[]
  edges: FlowEdge[]
  updated_at: string
}

export type RunStatus = 'idle' | 'running' | 'success' | 'failed'

export interface ExecutionLog {
  node_id: string
  label: string
  status: RunStatus
  duration_ms: number
  timestamp: string
  output: Record<string, unknown>
}

export type ToolCategory = 'lookup' | 'action'
export type ToolCommandType = 'http-api' | 'manual' | 'bash' | 'ssh'

export interface ToolParameter {
  key: string
  label?: string
  in: ParamLocation
  mode: PayloadMode
  value?: string
  default_value?: string
  required?: boolean
}

export interface ToolDefinition {
  id: string
  name: string
  category: ToolCategory
  description: string
  keywords: string[]
  command_type: ToolCommandType
  http_method?: HTTPMethod
  endpoint?: string
  payload_mode?: PayloadMode
  params?: ToolParameter[]
  payload_template?: Record<string, unknown>
  mock_output?: Record<string, unknown>
  is_builtin: boolean
  enabled: boolean
  created_at: string
  updated_at: string
}

export type ToolPayload = Omit<ToolDefinition, 'is_builtin' | 'created_at' | 'updated_at'>