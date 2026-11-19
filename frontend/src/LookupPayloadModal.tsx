import { useState, useEffect, useMemo, type FormEvent } from 'react'
import type { FlowNode, PayloadMode, ToolDefinition, ToolParameter } from './types'

interface LookupPayloadModalProps {
  node: FlowNode | null
  tool?: ToolDefinition
  runtimeContext: Record<string, unknown>
  onClose: () => void
  onSave: (nodeId: string, payloadMode: PayloadMode, inputPayload: Record<string, string>) => void
}

const DYNAMIC_VAR_CHIPS = [
  '{{alert.src_ip}}',
  '{{alert.domain}}',
  '{{alert.file_hash}}',
  '{{alert.username}}',
  '{{prev.verdict}}',
]

function lookupPath(ctx: Record<string, unknown>, path: string): string {
  const parts = path.split('.')
  let cur: unknown = ctx
  for (const part of parts) {
    if (cur == null || typeof cur !== 'object') return ''
    cur = (cur as Record<string, unknown>)[part]
  }
  return cur == null ? '' : String(cur)
}

function resolvePreviewValue(expr: string, mode: PayloadMode, ctx: Record<string, unknown>): string {
  if (mode === 'fixed') return expr
  return expr.replace(/\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}/g, (_, key: string) => lookupPath(ctx, key))
}

export function LookupPayloadModal({
  node,
  tool,
  runtimeContext,
  onClose,
  onSave,
}: LookupPayloadModalProps) {
  const [mode, setMode] = useState<PayloadMode>('dynamic')
  const [values, setValues] = useState<Record<string, string>>({})
  const [customKey, setCustomKey] = useState('')

  useEffect(() => {
    if (!node) return
    const nextMode: PayloadMode = node.payload_mode || tool?.payload_mode || 'dynamic'
    const initial: Record<string, string> = {}
    for (const p of tool?.params || []) {
      initial[p.key] = node.input_payload?.[p.key] ?? p.value ?? p.default_value ?? ''
    }
    for (const [k, v] of Object.entries(node.input_payload || {})) {
      if (!(k in initial)) initial[k] = v
    }
    setMode(nextMode)
    setValues(initial)
    setCustomKey('')
  }, [node, tool])

  useEffect(() => {
    if (!node) return
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [node, onClose])

  const paramList: ToolParameter[] = useMemo(() => {
    const base = [...(tool?.params || [])]
    const seen = new Set(base.map((p) => p.key))
    for (const k of Object.keys(values)) {
      if (!seen.has(k)) {
        base.push({ key: k, in: 'query', mode })
      }
    }
    return base
  }, [tool, values, mode])

  const preview = useMemo(() => {
    const method = tool?.http_method || 'GET'
    let url = tool?.endpoint || ''
    const queryPairs: Array<[string, string]> = []
    const bodyObj: Record<string, string> = {}

    for (const p of paramList) {
      const raw = values[p.key] ?? ''
      const resolved = resolvePreviewValue(raw, mode, runtimeContext)
      if (p.in === 'path') {
        url = url.replaceAll(`{{${p.key}}}`, encodeURIComponent(resolved))
      } else if (p.in === 'query' && resolved !== '') {
        queryPairs.push([p.key, resolved])
      } else if (p.in === 'body') {
        bodyObj[p.key] = resolved
      }
    }

    if (queryPairs.length > 0 && url) {
      const qs = queryPairs.map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`).join('&')
      url += (url.includes('?') ? '&' : '?') + qs
    }

    return { method, url, bodyObj }
  }, [tool, paramList, values, mode, runtimeContext])

  if (!node) return null

  function handleAddCustomParam() {
    const k = customKey.trim()
    if (!k) return
    setValues((prev) => ({ ...prev, [k]: prev[k] ?? '' }))
    setCustomKey('')
  }

  function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!node) return
    onSave(node.id, mode, values)
    onClose()
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="flex max-h-[85vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-2xl">
        <div className="flex shrink-0 items-center justify-between border-b border-slate-200 bg-slate-50 px-4 py-3">
          <div>
            <h3 className="text-sm font-bold text-slate-900">
              Lookup Input Payload: {tool?.name || node.tool}
            </h3>
            <p className="text-[11px] text-slate-500">{node.label}</p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded px-2 py-1 text-xs font-bold text-slate-500 hover:bg-slate-200"
          >
            ✕ ปิด
          </button>
        </div>

        <form onSubmit={handleSubmit} className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
          <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-slate-200 bg-slate-50 p-2.5">
            <span className="text-xs font-bold text-slate-700">Payload Mode:</span>
            <div className="flex rounded-lg border border-slate-300 bg-white p-0.5 text-xs font-semibold">
              <button
                type="button"
                onClick={() => setMode('fixed')}
                className={`rounded-md px-3 py-1 ${
                  mode === 'fixed' ? 'bg-amber-500 text-white' : 'text-slate-600 hover:text-slate-900'
                }`}
              >
                Fixed Payload (ค่าคงที่)
              </button>
              <button
                type="button"
                onClick={() => setMode('dynamic')}
                className={`rounded-md px-3 py-1 ${
                  mode === 'dynamic' ? 'bg-violet-600 text-white' : 'text-slate-600 hover:text-slate-900'
                }`}
              >
                Dynamic Payload (ตัวแปร {'{{...}}'})
              </button>
            </div>
          </div>

          <div className="space-y-2">
            {paramList.map((p) => (
              <div key={`${p.in}-${p.key}`} className="rounded-lg border border-slate-200 p-2.5 bg-white">
                <div className="flex items-center justify-between gap-2 mb-1.5">
                  <div className="flex items-center gap-1.5">
                    <span className="rounded bg-sky-100 px-1.5 py-0.5 text-[10px] font-bold uppercase text-sky-800">
                      {p.in}
                    </span>
                    <code className="text-xs font-bold text-slate-800">{p.key}</code>
                    {p.label && <span className="text-[11px] text-slate-500">({p.label})</span>}
                    {p.required && <span className="text-[10px] font-bold text-rose-600">*required</span>}
                  </div>
                  <span
                    className={`rounded px-1.5 py-0.5 text-[10px] font-bold uppercase ${
                      mode === 'fixed' ? 'bg-amber-100 text-amber-800' : 'bg-violet-100 text-violet-800'
                    }`}
                  >
                    {mode}
                  </span>
                </div>

                <input
                  type="text"
                  value={values[p.key] ?? ''}
                  onChange={(e) => setValues((prev) => ({ ...prev, [p.key]: e.target.value }))}
                  placeholder={mode === 'fixed' ? 'ใส่ค่าคงที่ เช่น 8.8.8.8' : 'ใส่ตัวแปร เช่น {{alert.src_ip}}'}
                  className="w-full rounded border border-slate-300 px-2.5 py-1.5 font-mono text-xs"
                />

                {mode === 'dynamic' && (
                  <div className="mt-1.5 flex flex-wrap gap-1">
                    {DYNAMIC_VAR_CHIPS.map((chip) => (
                      <button
                        key={chip}
                        type="button"
                        onClick={() => setValues((prev) => ({ ...prev, [p.key]: chip }))}
                        className="rounded border border-violet-200 bg-violet-50 px-1.5 py-0.5 font-mono text-[10px] text-violet-700 hover:bg-violet-100"
                      >
                        {chip}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            ))}

            {paramList.length === 0 && (
              <div className="rounded border border-dashed border-slate-300 p-3 text-center text-xs text-slate-500">
                Tool นี้ยังไม่ได้กำหนด URL Parameters — สามารถเพิ่ม Key เองด้านล่าง หรือตั้งค่าที่หน้าจัดการ Tools
              </div>
            )}

            <div className="flex gap-2 pt-1">
              <input
                type="text"
                value={customKey}
                onChange={(e) => setCustomKey(e.target.value)}
                placeholder="เพิ่ม Parameter Key อื่นๆ (เช่น domain, hash)"
                className="flex-1 rounded border border-slate-300 px-2.5 py-1 text-xs"
              />
              <button
                type="button"
                onClick={handleAddCustomParam}
                className="rounded border border-slate-300 bg-slate-100 px-3 py-1 text-xs font-semibold text-slate-700 hover:bg-slate-200"
              >
                + เพิ่ม Tag
              </button>
            </div>
          </div>

          <div className="rounded-lg border border-slate-200 bg-slate-900 p-2.5 text-slate-100 font-mono text-[11px]">
            <div className="text-[10px] font-bold uppercase text-emerald-400 mb-1">
              Live Resolved URL Preview ({preview.method})
            </div>
            <div className="break-all text-sky-300">{preview.url || '(no endpoint URL configured)'}</div>
            {Object.keys(preview.bodyObj).length > 0 && (
              <pre className="mt-1 text-[10px] text-slate-300">{JSON.stringify(preview.bodyObj)}</pre>
            )}
          </div>

          <div className="mt-auto flex justify-end gap-2 border-t pt-3">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-slate-300 px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50"
            >
              ยกเลิก
            </button>
            <button
              type="submit"
              className="rounded-lg bg-blue-600 px-4 py-1.5 text-xs font-bold text-white hover:bg-blue-700"
            >
              บันทึก Payload
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
