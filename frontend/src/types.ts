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

export type ConditionOperator =
  | '=='
  | '!='
  | '>'
  | '>='
  | '<'
  | '<='
  | 'contains'
  | 'is_true'
  | 'is_false'

export interface EdgeCondition {
  field: string
  op: ConditionOperator
  value?: string | number | boolean
}

export interface FlowEdge {
  id: string
  source: string
  target: string
  label?: string
  condition?: EdgeCondition
}

export interface PlaybookItem {
  id: string
  version?: number
  title: string
  raw_text: string
  engine: string
  nodes: FlowNode[]
  edges: FlowEdge[]
  updated_at: string
}

export interface PlaybookVersionSnapshot {
  id: string
  playbook_id: string
  version: number
  title: string
  raw_text: string
  engine: string
  nodes: FlowNode[]
  edges: FlowEdge[]
  created_at: string
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
export type ToolAuthType = 'none' | 'api_key' | 'bearer' | 'basic'
export type ToolAuthLocation = 'header' | 'query'
export type ExecutionMode = 'sandbox' | 'live'

export interface ToolAuth {
  type: ToolAuthType
  in?: ToolAuthLocation
  key_name?: string
  prefix?: string
  env_var?: string
  allow_runtime?: boolean
}

export interface ToolParameter {
  key: string
  label?: string
  in: ParamLocation
  mode: PayloadMode
  value?: string
  default_value?: string
  required?: boolean
  is_secret?: boolean
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
  auth?: ToolAuth
  auth_configured?: boolean
  params?: ToolParameter[]
  payload_template?: Record<string, unknown>
  mock_output?: Record<string, unknown>
  is_builtin: boolean
  enabled: boolean
  created_at: string
  updated_at: string
}

export type ToolPayload = Omit<ToolDefinition, 'auth_configured' | 'is_builtin' | 'created_at' | 'updated_at'>

export type ScenarioDifficulty = 'easy' | 'medium' | 'hard'
export type ScenarioVerdict = 'clean' | 'suspicious' | 'malicious'

export interface ScenarioGroundTruth {
  expected_verdict: ScenarioVerdict
  required_nodes?: string[]
  expected_edges?: string[]
  max_duration_sec?: number
}

export interface ScenarioItem {
  id: string
  title: string
  description: string
  difficulty: ScenarioDifficulty
  playbook_id?: string
  alert: Record<string, unknown>
  tool_mocks?: Record<string, Record<string, Record<string, unknown>>>
  ground_truth?: ScenarioGroundTruth
  is_builtin: boolean
  created_at: string
  updated_at: string
}

export type ScenarioPayload = Omit<ScenarioItem, 'is_builtin' | 'created_at' | 'updated_at'>

export type UserRole = 'trainee' | 'instructor'

export interface AuthUser {
  id: string
  username: string
  display_name: string
  role: UserRole
}

export interface RunScore {
  total: number
  verdict_points: number
  coverage_points: number
  path_points: number
  speed_points: number
  duration_sec: number
  max_duration_sec?: number
  expected_verdict?: string
  submitted_verdict: string
  missed_nodes?: string[]
  missed_edges?: string[]
  feedback: string[]
}

export interface SimulationRun {
  id: string
  scenario_id?: string
  scenario_title?: string
  playbook_id?: string
  playbook_version?: number
  playbook_title: string
  nodes: FlowNode[]
  edges: FlowEdge[]
  alert?: Record<string, unknown>
  trainee_id: string
  trainee_name: string
  mode: ExecutionMode
  status: 'in_progress' | 'completed' | 'abandoned'
  node_statuses: Record<string, RunStatus>
  node_outputs: Record<string, Record<string, unknown>>
  traversed_edges?: string[]
  final_verdict?: ScenarioVerdict
  summary?: string
  score?: RunScore
  started_at: string
  completed_at?: string
  updated_at: string
}

export interface RunEvent {
  id: string
  run_id: string
  seq: number
  node_id?: string
  node_label?: string
  action: 'run_started' | 'step_executed' | 'manual_verdict' | 'run_completed'
  status?: string
  input?: Record<string, unknown>
  output?: Record<string, unknown>
  verdict?: string
  notes?: string
  duration_ms?: number
  timestamp: string
}