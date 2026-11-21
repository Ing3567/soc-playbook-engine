import test from 'node:test'
import assert from 'node:assert/strict'
import {
  computePipelineState,
  evaluateEdgeCondition,
  formatEdgeCondition,
  getDownstreamNodeIds,
  pickDecisionEdge,
} from './pipeline.ts'
import type { FlowEdge, FlowNode } from './types.ts'

test('evaluateEdgeCondition supports numeric, equality, contains, boolean, and alert paths', () => {
  assert.equal(
    evaluateEdgeCondition({ field: 'prev.malicious_count', op: '>', value: 5 }, { malicious_count: 14 }),
    true
  )
  assert.equal(
    evaluateEdgeCondition({ field: 'prev.malicious_count', op: '<=', value: 5 }, { malicious_count: 5 }),
    true
  )
  assert.equal(
    evaluateEdgeCondition({ field: 'prev.malicious_count', op: '>', value: 5 }, { malicious_count: 0 }),
    false
  )
  assert.equal(
    evaluateEdgeCondition({ field: 'prev.verdict', op: 'contains', value: 'phishing' }, { verdict: 'Phishing Portal' }),
    true
  )
  assert.equal(
    evaluateEdgeCondition({ field: 'prev.matched_cases', op: 'contains', value: 'SOC-1024' }, { matched_cases: ['SOC-1024'] }),
    true
  )
  assert.equal(
    evaluateEdgeCondition({ field: 'alert.severity', op: '==', value: 'high' }, null, { alert: { severity: 'HIGH' } }),
    true
  )
  assert.equal(
    evaluateEdgeCondition({ field: 'prev.is_malicious', op: 'is_true' }, { is_malicious: true }),
    true
  )
  assert.equal(
    evaluateEdgeCondition({ field: 'prev.is_malicious', op: 'is_false' }, { is_malicious: false }),
    true
  )
  assert.equal(formatEdgeCondition({ field: 'prev.malicious_count', op: '>', value: 5 }), 'malicious_count > 5')
  assert.equal(formatEdgeCondition({ field: 'prev.is_malicious', op: 'is_true' }), 'is_malicious is true')
})

test('pickDecisionEdge prefers structured conditions and falls back to else or legacy heuristic', () => {
  const structured: FlowEdge[] = [
    { id: 'e_high', source: 'd1', target: 'block', label: 'ใช่', condition: { field: 'prev.malicious_count', op: '>', value: 5 } },
    { id: 'e_else', source: 'd1', target: 'close', label: 'ไม่ใช่' },
  ]
  assert.equal(pickDecisionEdge(structured, { malicious_count: 14 })?.id, 'e_high')
  assert.equal(pickDecisionEdge(structured, { malicious_count: 0 })?.id, 'e_else')

  const legacy: FlowEdge[] = [
    { id: 'e_yes', source: 'd2', target: 'a1', label: 'ใช่' },
    { id: 'e_no', source: 'd2', target: 'a2', label: 'ไม่ใช่' },
  ]
  assert.equal(pickDecisionEdge(legacy, { confidence_score: 85 })?.id, 'e_yes')
  assert.equal(pickDecisionEdge(legacy, { confidence_score: 10 })?.id, 'e_no')
})

test('computePipelineState locks downstream nodes until upstream succeeds and skips non-selected branch', () => {
  const nodes: FlowNode[] = [
    { id: 'start', type: 'terminal', label: 'Start', tool: 'none' },
    { id: 'n_vt', type: 'lookup', label: 'Check VT', tool: 'virustotal' },
    { id: 'd1', type: 'decision', label: 'Malicious?', tool: 'none' },
    { id: 'n_block', type: 'action', label: 'Block IP', tool: 'firewall_block' },
    { id: 'n_close', type: 'end', label: 'Close FP', tool: 'none' },
  ]
  const edges: FlowEdge[] = [
    { id: 'e0', source: 'start', target: 'n_vt' },
    { id: 'e1', source: 'n_vt', target: 'd1' },
    { id: 'e2', source: 'd1', target: 'n_block', label: 'ใช่', condition: { field: 'prev.malicious_count', op: '>', value: 5 } },
    { id: 'e3', source: 'd1', target: 'n_close', label: 'ไม่ใช่' },
  ]

  assert.deepEqual(getDownstreamNodeIds('d1', edges).sort(), ['n_block', 'n_close'])

  const initial = computePipelineState(nodes, edges, {}, {})
  assert.equal(initial.nodeStates.n_vt.availability, 'ready')
  assert.equal(initial.nodeStates.n_block.availability, 'locked')

  const afterTP = computePipelineState(nodes, edges, { n_vt: 'success' }, { n_vt: { malicious_count: 14 } })
  assert.equal(afterTP.nodeStates.n_block.availability, 'ready')
  assert.equal(afterTP.nodeStates.n_close.availability, 'skipped')

  const afterFP = computePipelineState(nodes, edges, { n_vt: 'success' }, { n_vt: { malicious_count: 0 } })
  assert.equal(afterFP.nodeStates.n_block.availability, 'skipped')
  assert.equal(afterFP.nodeStates.n_close.availability, 'ready')
})
