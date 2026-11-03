import { useMemo, useRef, useState } from 'react'
import ReactFlow, { Background, Controls, MarkerType } from 'reactflow'
import type { Edge, Node } from 'reactflow'
import 'reactflow/dist/style.css'
import dagre from '@dagrejs/dagre'
import { toPng } from 'html-to-image'
import { jsPDF } from 'jspdf'

type NodeType = 'lookup' | 'decision' | 'action' | 'end' | 'terminal'
interface FlowNode { id: string; type: NodeType; label: string }
interface FlowEdge { id: string; source: string; target: string; label?: string }
interface Line { level: number; text: string }
interface FlowData { nodes: FlowNode[]; edges: FlowEdge[]; engine: string; lines: Line[] }

const W = 240

const nodeH = (label: string, type: NodeType) =>
  Math.max(56, 28 + Math.ceil(label.length / 26) * 18) + (type === 'lookup' ? 30 : 0)

const STYLE: Record<NodeType, { icon: string; bg: string; border: string; radius: number }> = {
  lookup: { icon: '🔎', bg: '#dbeafe', border: '#3b82f6', radius: 8 },
  decision: { icon: '🔀', bg: '#fef3c7', border: '#f59e0b', radius: 8 },
  action: { icon: '📝', bg: '#f1f5f9', border: '#94a3b8', radius: 8 },
  end: { icon: '🛑', bg: '#fee2e2', border: '#ef4444', radius: 8 },
  terminal: { icon: '', bg: '#dcfce7', border: '#22c55e', radius: 32 },
}

const SAMPLE = `ตรวจสอบ reputation ของ IP บน VirusTotal
ถ้า malicious มากกว่า 10
  escalate ไป Tier 2
  ถ้าเป็น host สำคัญ
    isolate เครื่องทันที
  ไม่เช่นนั้น
    แจ้งเจ้าของเครื่อง
ไม่เช่นนั้นถ้า malicious 1-10
  ค้นหาเคสเก่าใน Jira ที่คล้ายกัน
  ถ้าเคยเป็น false positive
    ปิดเคส
ไม่เช่นนั้น
  เฝ้าระวังต่อ 24 ชั่วโมง
บันทึกผลลงใน ticket`

const edgeColor = (l?: string) =>
  !l ? '#64748b' : l === 'ใช่' ? '#16a34a' : l === 'ไม่ใช่' ? '#dc2626' : '#2563eb'


function placeholder(title: string, color: string) {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360">
    <rect width="100%" height="100%" fill="${color}"/>
    <text x="50%" y="46%" font-size="28" text-anchor="middle" fill="#1e293b" font-family="sans-serif">${title}</text>
    <text x="50%" y="58%" font-size="16" text-anchor="middle" fill="#475569" font-family="sans-serif">(mock image)</text>
  </svg>`
  return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg)
}

const MOCK_IMAGES = [
  { title: 'รูปที่ 1: หน้าจอ VirusTotal', src: placeholder('VirusTotal Screenshot', '#dbeafe') },
  { title: 'รูปที่ 2: ตัวอย่าง Jira Ticket', src: placeholder('Jira Ticket Example', '#fef3c7') },
  { title: 'รูปที่ 3: แผนผังเครือข่าย', src: placeholder('Network Diagram', '#dcfce7') },
]

export default function App() {
  const [text, setText] = useState(SAMPLE)
  const [data, setData] = useState<FlowData | null>(null)
  const [name, setName] = useState('sop-flowchart')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [version, setVersion] = useState(0)
  const [showImages, setShowImages] = useState(false)
  const flowRef = useRef<HTMLDivElement>(null)

  async function send(form: FormData, baseName: string): Promise<FlowData | null> {
    setError('')
    setLoading(true)
    try {
      const res = await fetch('/api/generate', { method: 'POST', body: form })
      const json = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(json.error ?? res.statusText)
      setData(json)
      setName(baseName)
      setVersion((v) => v + 1)
      return json
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      return null
    } finally {
      setLoading(false)
    }
  }

  function generate() {
    const form = new FormData()
    form.append('text', text)
    send(form, 'sop-flowchart')
  }

  async function onFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    const lower = file.name.toLowerCase()
    if (lower.endsWith('.doc')) return setError('ไฟล์ .doc รุ่นเก่าไม่รองรับ กรุณา Save As เป็น .docx ก่อน')
    if (!lower.endsWith('.docx')) return setError('รองรับเฉพาะไฟล์ Word (.docx) เท่านั้น')
    const form = new FormData()
    form.append('file', file)
    const json = await send(form, file.name.replace(/\.docx$/i, ''))
    if (json) setText(json.lines.map((l) => '  '.repeat(l.level) + l.text).join('\n'))
  }

  const { rfNodes, rfEdges, bounds } = useMemo(() => {
    const empty = { rfNodes: [] as Node[], rfEdges: [] as Edge[], bounds: { x: 0, y: 0, w: 0, h: 0 } }
    if (!data) return empty

    const g = new dagre.graphlib.Graph({ multigraph: true })
    g.setGraph({ rankdir: 'TB', nodesep: 50, ranksep: 80 })
    g.setDefaultEdgeLabel(() => ({}))
    data.nodes.forEach((n) => g.setNode(n.id, { width: W, height: nodeH(n.label, n.type) }))
    data.edges.forEach((e) => g.setEdge(e.source, e.target, {}, e.id))
    dagre.layout(g)

    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
    const rfNodes: Node[] = data.nodes.map((n) => {
      const p = g.node(n.id)
      const h = nodeH(n.label, n.type)
      const x = p.x - W / 2
      const y = p.y - h / 2
      minX = Math.min(minX, x); minY = Math.min(minY, y)
      maxX = Math.max(maxX, x + W); maxY = Math.max(maxY, y + h)
      const s = STYLE[n.type] ?? STYLE.action
      return {
        id: n.id,
        position: { x, y },
        data: {
          label: (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 6 }}>
              <span>{`${s.icon} ${n.label}`.trim()}</span>
              {n.type === 'lookup' && (
                <button
                  type="button"
                  className="no-export rounded bg-blue-600 px-3 py-0.5 text-xs font-semibold text-white hover:bg-blue-700"
                  onClick={(e) => e.stopPropagation()}
                >
                  ▶ Run
                </button>
              )}
            </div>
          ),
        },
        style: {
          width: W, height: h, background: s.bg, border: `2px solid ${s.border}`,
          borderRadius: s.radius, fontSize: 13, display: 'flex', alignItems: 'center',
          justifyContent: 'center', textAlign: 'center', padding: 6,
        },
      }
    })

    const rfEdges: Edge[] = data.edges.map((e) => ({
      id: e.id,
      source: e.source,
      target: e.target,
      label: e.label || undefined,
      type: 'smoothstep',
      markerEnd: { type: MarkerType.ArrowClosed, color: edgeColor(e.label) },
      style: { stroke: edgeColor(e.label), strokeWidth: 2 },
      labelStyle: { fontSize: 12, fontWeight: 700, fill: edgeColor(e.label) },
      labelBgStyle: { fill: '#ffffff' },
    }))

    return { rfNodes, rfEdges, bounds: { x: minX, y: minY, w: maxX - minX, h: maxY - minY } }
  }, [data])

  async function savePdf() {
    const viewport = flowRef.current?.querySelector<HTMLElement>('.react-flow__viewport')
    if (!viewport || !data) return
    try {
      const pad = 60
      const w = Math.ceil(bounds.w + pad * 2)
      const h = Math.ceil(bounds.h + pad * 2)
      const png = await toPng(viewport, {
        backgroundColor: '#ffffff',
        width: w,
        height: h,
        pixelRatio: 2,
        filter: (el) => !(el as HTMLElement).classList?.contains('no-export'),
        style: {
          width: `${w}px`,
          height: `${h}px`,
          transform: `translate(${pad - bounds.x}px, ${pad - bounds.y}px) scale(1)`,
        },
      })
      const pdf = new jsPDF({ orientation: w >= h ? 'landscape' : 'portrait', unit: 'px', format: [w, h] })
      pdf.addImage(png, 'PNG', 0, 0, w, h)
      pdf.save(`${name}.pdf`)
    } catch (err) {
      setError('Save PDF ไม่สำเร็จ: ' + (err instanceof Error ? err.message : String(err)))
    }
  }

  return (
    <div className="grid h-full grid-cols-[360px_1fr] text-slate-800">
      <aside className="space-y-3 overflow-y-auto border-r bg-white p-4">
        <h1 className="text-lg font-bold">🛡️ SOP → Flowchart</h1>

        <label className="block cursor-pointer rounded border-2 border-dashed border-blue-400 p-3 text-center text-sm text-blue-700 hover:bg-blue-50">
          📄 อัปโหลดไฟล์ Word (.docx)
          <input
            type="file"
            hidden
            disabled={loading}
            accept=".docx,application/vnd.openxmlformats-officedocument.wordprocessingml.document"
            onChange={onFile}
          />
        </label>

        <p className="text-center text-xs text-slate-400">— หรือพิมพ์ / วาง SOP ด้านล่าง —</p>

        <textarea
          className="h-72 w-full rounded border p-2 font-mono text-xs"
          value={text}
          onChange={(e) => setText(e.target.value)}
          spellCheck={false}
        />
        <p className="text-xs text-slate-500">
          ย่อหน้า (Tab / เว้น 2 ช่อง) = อยู่ใน branch ของ "ถ้า" ด้านบน · "ไม่เช่นนั้นถ้า" = เพิ่มกิ่ง · "ปิดเคส" = หยุดเส้นทางนั้น
        </p>

        <button
          onClick={generate}
          disabled={loading || !text.trim()}
          className="w-full rounded bg-blue-600 py-2 font-semibold text-white hover:bg-blue-700 disabled:opacity-50"
        >
          {loading ? 'กำลังแปลง…' : '⚡ Generate Flowchart'}
        </button>

        {error && <p className="rounded bg-red-50 p-2 text-sm text-red-600">⚠️ {error}</p>}

        {data && (
          <>
            <p className="text-xs text-slate-500">
              Engine: <b>{data.engine}</b> · {data.nodes.length} nodes · {data.edges.length} edges
            </p>
            <button
              onClick={savePdf}
              className="w-full rounded bg-emerald-600 py-2 font-semibold text-white hover:bg-emerald-700"
            >
              💾 Save PDF
            </button>
          </>
        )}

        <div className="space-y-1 text-xs text-slate-600">
          <p>🔎 ฟ้า = ตรวจสอบ/ค้นหา (มีปุ่ม Run)</p>
          <p>🔀 เหลือง = เงื่อนไข (แตกกี่กิ่งก็ได้)</p>
          <p>📝 เทา = ขั้นตอนทั่วไป</p>
          <p>🛑 แดง = สั่งหยุดเส้นทางนี้</p>
          <p>ขั้นตอนที่ไม่มีลูกศรออก = จุดสิ้นสุดของกิ่งนั้น</p>
        </div>
      </aside>

      <main ref={flowRef} className="relative bg-slate-50">
        {data ? (
          <>
            <ReactFlow key={version} nodes={rfNodes} edges={rfEdges} fitView nodesDraggable={false} nodesConnectable={false}>
              <Background />
              <Controls showInteractive={false} />
            </ReactFlow>

            <button
              onClick={() => setShowImages(true)}
              className="absolute right-4 top-4 z-10 rounded-full border border-amber-400 bg-amber-50 px-4 py-2 text-sm font-semibold text-amber-800 shadow hover:bg-amber-100"
            >
              มีรูปภาพ {MOCK_IMAGES.length} รูปในโฟลว์ชาร์ตนี้
            </button>
          </>
        ) : (
          <div className="flex h-full items-center justify-center text-slate-400">
            พิมพ์ SOP หรืออัปโหลด .docx แล้วกด Generate
          </div>
        )}
      </main>


      {showImages && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-6"
          onClick={() => setShowImages(false)}
        >
          <div
            className="max-h-full w-full max-w-2xl overflow-y-auto rounded-lg bg-white p-4 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="mb-3 flex items-center justify-between">
              <h2 className="font-bold">🖼️ รูปภาพในเอกสาร ({MOCK_IMAGES.length})</h2>
              <button
                onClick={() => setShowImages(false)}
                className="rounded px-2 py-1 text-slate-500 hover:bg-slate-100"
              >
                ✕ ปิด
              </button>
            </div>
            <div className="space-y-4">
              {MOCK_IMAGES.map((img) => (
                <figure key={img.title}>
                  <img src={img.src} alt={img.title} className="w-full rounded border" />
                  <figcaption className="mt-1 text-center text-xs text-slate-500">{img.title}</figcaption>
                </figure>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}