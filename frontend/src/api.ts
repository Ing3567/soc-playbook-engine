import type { FlowNode, FlowEdge, PlaybookItem } from './types'

export async function generateFlowchart(form: FormData) {
  const res = await fetch('/api/generate', { method: 'POST', body: form })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'สร้างกราฟไม่สำเร็จ')
  return data as {
    nodes: FlowNode[]
    edges: FlowEdge[]
    engine: string
    lines: { level: number; text: string }[]
  }
}

export async function getPlaybooks(): Promise<PlaybookItem[]> {
  const res = await fetch('/api/sops')
  if (!res.ok) return []
  const data = await res.json()
  return Array.isArray(data) ? data : []
}

export async function savePlaybook(payload: Omit<PlaybookItem, 'updated_at'>): Promise<PlaybookItem> {
  const res = await fetch('/api/sops', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'บันทึกไม่สำเร็จ')
  return data
}

export async function deletePlaybook(id: string) {
  await fetch(`/api/sops/${id}`, { method: 'DELETE' })
}

export async function executeStep(node: FlowNode, inputs?: Record<string, string>) {
  const res = await fetch('/api/execute', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      node_id: node.id,
      type: node.type,
      label: node.label,
      tool: node.tool,
      links: node.links,
      inputs,
    }),
  })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || 'รันไม่สำเร็จ')
  return data
}

export async function downloadExportFile(
  format: 'bpmn' | 'cacao' | 'markdown',
  title: string,
  nodes: FlowNode[],
  edges: FlowEdge[]
) {
  const res = await fetch(`/api/export/${format}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title, nodes, edges }),
  })
  if (!res.ok) throw new Error('ส่งออกไฟล์ไม่สำเร็จ')
  const content = await res.text()
  const extMap = { bpmn: '.bpmn', cacao: '.cacao.json', markdown: '.md' }
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `${title || 'playbook'}${extMap[format]}`
  link.click()
  URL.revokeObjectURL(url)
}