import test from 'node:test'
import assert from 'node:assert/strict'
import { buildNodeResolvedUrl, buildSopInputBlocks, getNodeParamItems } from './runtimeParams.ts'
import type { FlowEdge, FlowNode, ToolDefinition } from './types.ts'

const sampleTools: ToolDefinition[] = [
  {
    id: 'virustotal_web',
    name: 'VirusTotal Web',
    category: 'lookup',
    description: 'Manual VT lookup',
    keywords: ['vt'],
    command_type: 'manual',
    http_method: 'GET',
    endpoint: 'https://www.virustotal.com/gui/ip-address/{{ip}}',
    payload_mode: 'dynamic',
    params: [
      { key: 'ip', label: 'IP', in: 'path', mode: 'dynamic', value: '{{alert.src_ip}}', required: true },
      { key: 'tab', label: 'Tab', in: 'query', mode: 'fixed', value: 'relations' },
    ],
    is_builtin: false,
    enabled: true,
    created_at: '',
    updated_at: '',
  },
  {
    id: 'secret_api',
    name: 'Secret API',
    category: 'action',
    description: 'Action with secret param',
    keywords: ['secret'],
    command_type: 'http-api',
    http_method: 'POST',
    endpoint: 'https://api.example.test/block',
    payload_mode: 'dynamic',
    params: [
      { key: 'ip', in: 'body', mode: 'dynamic', value: '{{alert.src_ip}}', required: true },
      { key: 'api_token', in: 'header', mode: 'dynamic', required: true, is_secret: true },
    ],
    is_builtin: false,
    enabled: true,
    created_at: '',
    updated_at: '',
  },
]

test('getNodeParamItems separates fixed/pipe vs runtime items and preserves isSecret', () => {
  const node: FlowNode = { id: 'n1', type: 'action', label: 'Block', tool: 'secret_api' }
  const { runtimeInputItems } = getNodeParamItems(node, sampleTools, { api_token: 'tok-123' }, { src_ip: '8.8.8.8' })
  assert.equal(runtimeInputItems.length, 2)
  assert.equal(runtimeInputItems[0].sharedHint, '8.8.8.8')
  assert.equal(runtimeInputItems[1].isSecret, true)
  assert.equal(runtimeInputItems[1].runtimeVal, 'tok-123')
})

test('buildNodeResolvedUrl encodes path and query parameters cleanly', () => {
  const node: FlowNode = { id: 'n1', type: 'lookup', label: 'Check VT', tool: 'virustotal_web' }
  const { paramItems } = getNodeParamItems(node, sampleTools, {}, { src_ip: '185.220.101.42' })
  const url = buildNodeResolvedUrl(sampleTools[0], paramItems)
  assert.equal(url, 'https://www.virustotal.com/gui/ip-address/185.220.101.42?tab=relations')
})

test('buildSopInputBlocks detects shared alert keys across multiple steps', () => {
  const nodes: FlowNode[] = [
    { id: 'n1', type: 'lookup', label: 'VT', tool: 'virustotal_web' },
    { id: 'n2', type: 'action', label: 'Block', tool: 'secret_api' },
  ]
  const edges: FlowEdge[] = [{ id: 'e1', source: 'n1', target: 'n2' }]
  const summary = buildSopInputBlocks(nodes, edges, sampleTools, {}, { src_ip: '1.2.3.4' })
  assert.equal(summary.blocks.length, 2)
  assert.equal(summary.sharedAlertKeys.length, 1)
  assert.equal(summary.sharedAlertKeys[0].key, 'src_ip')
  assert.equal(summary.sharedAlertKeys[0].blockCount, 2)
})
