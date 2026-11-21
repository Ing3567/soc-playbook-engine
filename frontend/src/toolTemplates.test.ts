import test from 'node:test'
import assert from 'node:assert/strict'
import { mockRowsToObject, objectToMockRows, parseSmartUrl, slugifyName } from './toolTemplates.ts'

test('slugifyName normalizes tool names into valid IDs', () => {
  assert.equal(slugifyName('VirusTotal Web GUI!'), 'virustotal_web_gui')
  assert.equal(slugifyName('  EDR -- Isolate Host '), 'edr_isolate_host')
})

test('parseSmartUrl extracts path and query parameters with smart defaults', () => {
  const { cleanEndpoint, params } = parseSmartUrl(
    'https://api.example.com/v1/ip/{ip}?target=8.8.8.8&maxAge=90',
    'dynamic'
  )
  assert.equal(cleanEndpoint, 'https://api.example.com/v1/ip/{{ip}}')
  assert.equal(params.length, 3)
  assert.deepEqual(params[0], {
    key: 'ip',
    in: 'path',
    mode: 'dynamic',
    value: '{{alert.src_ip}}',
    required: true,
  })
  assert.equal(params[1].key, 'target')
  assert.equal(params[1].mode, 'dynamic')
  assert.equal(params[1].value, '{{alert.src_ip}}')
  assert.equal(params[2].key, 'maxAge')
  assert.equal(params[2].mode, 'fixed')
  assert.equal(params[2].value, '90')
})

test('mockRowsToObject and objectToMockRows round-trip typed values', () => {
  const obj = mockRowsToObject([
    { key: 'malicious_count', value: '12' },
    { key: 'is_malicious', value: 'true' },
    { key: 'tags', value: '["botnet","ssh"]' },
    { key: 'verdict', value: 'Malicious' },
  ])
  assert.deepEqual(obj, {
    malicious_count: 12,
    is_malicious: true,
    tags: ['botnet', 'ssh'],
    verdict: 'Malicious',
  })
  const rows = objectToMockRows(obj)
  assert.equal(rows.length, 4)
})
