export interface NodeLink {
  label: string
  url: string
  source?: string
  splunk_query?: string
}

export interface NodeImage {
  id: string
  filename: string
  content_type: string
  url: string
  caption?: string
}

export interface FlowNode {
  id: string
  type: string
  label: string
  tool: string
  links?: NodeLink[]
  images?: NodeImage[]
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

export interface ToolInputField {
  key: string
  label: string
  placeholder?: string
  type: 'text' | 'password'
  required?: boolean
}

export interface ToolStatusSummary {
  id: string
  name: string
  category: string
  enabled: boolean
  ready_real: boolean
  mode: string
  missing_env?: string[]
  input_fields?: ToolInputField[]
}

export interface ExecutionLog {
  node_id: string
  label: string
  status: RunStatus
  mode?: string
  duration_ms: number
  timestamp: string
  output: Record<string, unknown>
  error?: string
}