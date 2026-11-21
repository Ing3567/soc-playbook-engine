import type {
  HTTPMethod,
  PayloadMode,
  ToolAuth,
  ToolCategory,
  ToolCommandType,
  ToolDefinition,
  ToolParameter,
} from './types'

export interface MockKVRow {
  key: string
  value: string
}

export interface ToolFormState {
  id: string
  name: string
  category: ToolCategory
  description: string
  keywords: string[]
  command_type: ToolCommandType
  http_method: HTTPMethod
  endpoint: string
  payload_mode: PayloadMode
  auth: ToolAuth
  params: ToolParameter[]
  mockRows: MockKVRow[]
  mockOutputText: string
  mockJsonMode: boolean
  enabled: boolean
}

export const DEFAULT_AUTH: ToolAuth = {
  type: 'none',
  in: 'header',
  key_name: '',
  prefix: '',
  env_var: '',
  allow_runtime: true,
}

export const EMPTY_FORM: ToolFormState = {
  id: '',
  name: '',
  category: 'lookup',
  description: '',
  keywords: [],
  command_type: 'http-api',
  http_method: 'GET',
  endpoint: '',
  payload_mode: 'dynamic',
  auth: { ...DEFAULT_AUTH },
  params: [],
  mockRows: [],
  mockOutputText: '',
  mockJsonMode: false,
  enabled: true,
}

export const DYNAMIC_VAR_CHIPS = [
  '{{alert.src_ip}}',
  '{{alert.domain}}',
  '{{alert.file_hash}}',
  '{{alert.username}}',
  '{{prev.verdict}}',
]

export interface QuickTemplate {
  id: string
  label: string
  badge: string
  apply: () => Partial<ToolFormState>
}

export const QUICK_TEMPLATES: QuickTemplate[] = [
  {
    id: 'lookup_path',
    label: 'IP / Domain Lookup',
    badge: 'GET + Path',
    apply: () => ({
      name: 'Shodan Host',
      id: 'shodan_host',
      category: 'lookup',
      http_method: 'GET',
      endpoint: 'https://api.shodan.io/shodan/host/{{ip}}',
      payload_mode: 'dynamic',
      keywords: ['shodan', 'ip', 'port'],
      description: 'Lookup open ports, services, and host reputation on Shodan',
      params: [{ key: 'ip', label: 'Target IP', in: 'path', mode: 'dynamic', value: '{{alert.src_ip}}', required: true }],
      mockRows: [
        { key: 'verdict', value: 'Exposed Services Found' },
        { key: 'ports', value: '22, 80, 443' },
      ],
    }),
  },
  {
    id: 'lookup_query',
    label: 'Query API Lookup',
    badge: 'GET + Query',
    apply: () => ({
      name: 'SIEM Event Search',
      id: 'siem_event_search',
      category: 'lookup',
      http_method: 'GET',
      endpoint: 'https://siem.internal/api/v1/events',
      payload_mode: 'dynamic',
      keywords: ['siem', 'splunk', 'log', 'ค้นหา log'],
      description: 'Search historical security events and authentication logs in SIEM',
      params: [
        { key: 'src_ip', label: 'Source IP', in: 'query', mode: 'dynamic', value: '{{alert.src_ip}}', required: true },
        { key: 'window', label: 'Time Window', in: 'query', mode: 'fixed', value: '24h', required: false },
      ],
      mockRows: [
        { key: 'matched_events', value: '7' },
        { key: 'verdict', value: 'Suspicious Activity' },
      ],
    }),
  },
  {
    id: 'action_containment',
    label: 'Containment Action',
    badge: 'POST + Body',
    apply: () => ({
      name: 'EDR Isolate Host',
      id: 'edr_isolate_host',
      category: 'action',
      http_method: 'POST',
      endpoint: 'https://edr.internal/api/v1/hosts/isolate',
      payload_mode: 'dynamic',
      keywords: ['isolate', 'edr', 'กักกัน', 'ระงับเครื่อง'],
      description: 'Isolate compromised endpoint host via EDR agent',
      params: [
        { key: 'target', label: 'Target Host/IP', in: 'body', mode: 'dynamic', value: '{{alert.src_ip}}', required: true },
        { key: 'reason', label: 'Reason', in: 'body', mode: 'fixed', value: 'SOC Playbook Containment', required: false },
      ],
      mockRows: [
        { key: 'status', value: 'isolated' },
        { key: 'result', value: 'Host Isolated Successfully' },
      ],
    }),
  },
  {
    id: 'action_webhook',
    label: 'Alert Webhook',
    badge: 'POST Webhook',
    apply: () => ({
      name: 'SOC Slack Notify',
      id: 'soc_slack_notify',
      category: 'action',
      http_method: 'POST',
      endpoint: 'https://hooks.slack.com/services/T00/B00/XXX',
      payload_mode: 'dynamic',
      keywords: ['slack', 'notify', 'แจ้งเตือน', 'teams'],
      description: 'Send incident alert notification to SOC channel',
      params: [
        { key: 'text', label: 'Message', in: 'body', mode: 'dynamic', value: 'SOC Alert: {{alert.src_ip}} ({{prev.verdict}})', required: true },
      ],
      mockRows: [
        { key: 'delivered', value: 'true' },
        { key: 'channel', value: '#soc-alerts' },
      ],
    }),
  },
  {
    id: 'manual_web_check',
    label: 'Web Check (Anti-bot)',
    badge: 'Manual Browser',
    apply: () => ({
      name: 'VirusTotal Web GUI',
      id: 'virustotal_web',
      category: 'lookup',
      command_type: 'manual',
      http_method: 'GET',
      endpoint: 'https://www.virustotal.com/gui/ip-address/{{ip}}',
      payload_mode: 'dynamic',
      auth: { ...DEFAULT_AUTH },
      keywords: ['vt web', 'เปิดเว็บ', 'manual check'],
      description: 'Open VirusTotal web portal in browser for manual analyst inspection (avoids anti-bot)',
      params: [{ key: 'ip', label: 'Target IP', in: 'path', mode: 'dynamic', value: '{{alert.src_ip}}', required: true }],
      mockRows: [],
    }),
  },
]

export function slugifyName(name: string): string {
  return name
    .toLowerCase()
    .trim()
    .replace(/[\s-]+/g, '_')
    .replace(/[^a-z0-9_]+/g, '_')
    .replace(/_+/g, '_')
    .replace(/^_+|_+$/g, '')
}

export function parseSmartUrl(
  rawEndpoint: string,
  defaultMode: PayloadMode
): { cleanEndpoint: string; params: ToolParameter[] } {
  let clean = rawEndpoint.trim()
  const extracted: ToolParameter[] = []
  const seen = new Set<string>()

  const qIdx = clean.indexOf('?')
  let pathPart = qIdx >= 0 ? clean.slice(0, qIdx) : clean
  const queryPart = qIdx >= 0 ? clean.slice(qIdx + 1) : ''

  pathPart = pathPart.replace(/\{([a-zA-Z0-9_.-]+)\}/g, (full, key: string, offset: number, str: string) => {
    const prevChar = offset > 0 ? str[offset - 1] : ''
    const nextChar = str[offset + full.length] || ''
    if (prevChar === '{' || nextChar === '}') return full
    return `{{${key}}}`
  })
  pathPart = pathPart.replace(/\/:([a-zA-Z0-9_.-]+)/g, '/{{$1}}')

  for (const m of pathPart.matchAll(/\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}/g)) {
    const key = m[1]
    if (key && !seen.has(`path:${key}`)) {
      seen.add(`path:${key}`)
      const lk = key.toLowerCase()
      const guessVar =
        lk === 'ip' || lk.includes('ip')
          ? '{{alert.src_ip}}'
          : lk.includes('domain') || lk.includes('host')
            ? '{{alert.domain}}'
            : lk.includes('hash')
              ? '{{alert.file_hash}}'
              : `{{alert.${key}}}`
      extracted.push({ key, in: 'path', mode: defaultMode, value: guessVar, required: true })
    }
  }

  if (queryPart) {
    const sp = new URLSearchParams(queryPart)
    sp.forEach((rawVal, key) => {
      if (!key || seen.has(`query:${key}`)) return
      seen.add(`query:${key}`)
      const looksLikeIP = /^\d{1,3}(\.\d{1,3}){3}$/.test(rawVal)
      const looksLikeTemplate = /\{\{.*\}\}/.test(rawVal)
      let mode: PayloadMode = defaultMode
      let value = rawVal
      if (looksLikeIP) {
        mode = 'dynamic'
        value = '{{alert.src_ip}}'
      } else if (!looksLikeTemplate && rawVal !== '') {
        mode = 'fixed'
      } else if (rawVal === '') {
        mode = 'dynamic'
        value = `{{alert.${key}}}`
      }
      extracted.push({ key, in: 'query', mode, value, required: looksLikeIP || rawVal === '' })
    })
  }

  clean = pathPart
  return { cleanEndpoint: clean, params: extracted }
}

export function objectToMockRows(obj?: Record<string, unknown>): MockKVRow[] {
  if (!obj) return []
  return Object.entries(obj).map(([key, val]) => ({
    key,
    value: typeof val === 'string' ? val : JSON.stringify(val),
  }))
}

export function mockRowsToObject(rows: MockKVRow[]): Record<string, unknown> | undefined {
  const out: Record<string, unknown> = {}
  let count = 0
  for (const r of rows) {
    const k = r.key.trim()
    if (!k) continue
    count++
    const raw = r.value.trim()
    if (raw === 'true') out[k] = true
    else if (raw === 'false') out[k] = false
    else if (raw !== '' && !Number.isNaN(Number(raw))) out[k] = Number(raw)
    else if ((raw.startsWith('[') && raw.endsWith(']')) || (raw.startsWith('{') && raw.endsWith('}'))) {
      try {
        out[k] = JSON.parse(raw)
      } catch {
        out[k] = raw
      }
    } else {
      out[k] = raw
    }
  }
  return count > 0 ? out : undefined
}

export function lookupCtxPath(ctx: Record<string, unknown>, path: string): string {
  const parts = path.split('.')
  let cur: unknown = ctx
  for (const part of parts) {
    if (cur == null || typeof cur !== 'object') return ''
    cur = (cur as Record<string, unknown>)[part]
  }
  return cur == null ? '' : String(cur)
}

export function toolToFormState(tool: ToolDefinition, asClone: boolean): ToolFormState {
  return {
    id: asClone ? `${tool.id}_copy` : tool.id,
    name: asClone ? `${tool.name} (Copy)` : tool.name,
    category: tool.category,
    description: tool.description,
    keywords: Array.isArray(tool.keywords) ? [...tool.keywords] : [],
    command_type: tool.command_type || 'http-api',
    http_method: tool.http_method || (tool.category === 'lookup' ? 'GET' : 'POST'),
    endpoint: tool.endpoint || '',
    payload_mode: tool.payload_mode || 'dynamic',
    auth: tool.auth
      ? {
          type: tool.auth.type || 'none',
          in: tool.auth.in || 'header',
          key_name: tool.auth.key_name || '',
          prefix: tool.auth.prefix || '',
          env_var: tool.auth.env_var || '',
          allow_runtime: Boolean(tool.auth.allow_runtime),
        }
      : { ...DEFAULT_AUTH },
    params: Array.isArray(tool.params) ? tool.params.map((p) => ({ ...p })) : [],
    mockRows: objectToMockRows(tool.mock_output),
    mockOutputText:
      tool.mock_output && Object.keys(tool.mock_output).length > 0
        ? JSON.stringify(tool.mock_output, null, 2)
        : '',
    mockJsonMode: false,
    enabled: tool.enabled,
  }
}
