export interface FlowNode {
  id: string
  type: string
  label: string
  tool: string
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