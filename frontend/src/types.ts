export type ToolSource = 'ai' | 'keyword' | 'manual'

export interface FlowNode {
  id: string
  type: string
  label: string
  tool: string
  tool_source?: ToolSource
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

export interface ToolDefinition {
  id: string
  name: string
  category: ToolCategory
  description: string
  keywords: string[]
  command_type: ToolCommandType
  endpoint?: string
  mock_output?: Record<string, unknown>
  is_builtin: boolean
  enabled: boolean
  created_at: string
  updated_at: string
}

export type ToolPayload = Omit<ToolDefinition, 'is_builtin' | 'created_at' | 'updated_at'>