import { useState, useEffect, useMemo, type FormEvent } from 'react'
import type { FlowNode, PayloadMode, ToolDefinition, ToolParameter } from './types'

interface LookupPayloadModalProps {
  node: FlowNode | null
  tool?: ToolDefinition
  runtimeContext: Record<string, unknown>
  runtimeInputs?: Record<string, string>
  onClose: () => void
  onSave: (
    nodeId: string,
    paramModes: Record<string, PayloadMode>,
    inputPayload: Record<string, string>
  ) => void
  onEditTool?: (toolId: string) => void
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
  runtimeInputs = {},
  onClose,
  onSave,
  onEditTool,
}: LookupPayloadModalProps) {
  const [paramModes, setParamModes] = useState<Record<string, PayloadMode>>({})
  const [values, setValues] = useState<Record<string, string>>({})
  const [customKey, setCustomKey] = useState('')

  useEffect(() => {
    if (!node) return
    const fallbackMode: PayloadMode = node.payload_mode || tool?.payload_mode || 'dynamic'
    const initialModes: Record<string, PayloadMode> = {}
    const initialValues: Record<string, string> = {}

    for (const p of tool?.params || []) {
      const m: PayloadMode = node.param_modes?.[p.key] || p.mode || fallbackMode
      initialModes[p.key] = m
      const savedVal = node.input_payload?.[p.key]
      if (savedVal !== undefined) {
        initialValues[p.key] = savedVal
      } else if (m === 'fixed') {
        initialValues[p.key] = p.value ?? p.default_value ?? ''
      } else {
        // Keep template expressions like {{alert.src_ip}} or query wrappers, never raw customer values
        const defVal = p.value ?? p.default_value ?? ''
        initialValues[p.key] = defVal.includes('{{') && defVal.includes('}}') ? defVal : ''
      }
    }

    for (const [k, v] of Object.entries(node.input_payload || {})) {
      if (!(k in initialValues)) {
        initialModes[k] = node.param_modes?.[k] || fallbackMode
        initialValues[k] = v
      }
    }

    setParamModes(initialModes)
    setValues(initialValues)
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
        base.push({ key: k, in: 'query', mode: paramModes[k] || 'dynamic' })
      }
    }
    return base
  }, [tool, values, paramModes])

  const preview = useMemo(() => {
    const method = tool?.http_method || 'GET'
    let url = tool?.endpoint || ''
    const queryPairs: Array<[string, string]> = []
    const bodyObj: Record<string, string> = {}

    for (const p of paramList) {
      const pMode: PayloadMode = paramModes[p.key] || p.mode || 'dynamic'
      const raw = values[p.key] ?? ''
      const rt = runtimeInputs[p.key]?.trim()
      const resolved =
        pMode === 'fixed'
          ? raw
          : rt
            ? rt
            : resolvePreviewValue(raw, 'dynamic', runtimeContext)

      if (p.in === 'path') {
        url = url.replaceAll(`{{${p.key}}}`, resolved ? encodeURIComponent(resolved) : `{{${p.key}}}`)
      } else if (p.in === 'query' && resolved !== '') {
        queryPairs.push([p.key, resolved])
      } else if (p.in === 'body' && resolved !== '') {
        bodyObj[p.key] = resolved
      }
    }

    if (queryPairs.length > 0 && url) {
      const qs = queryPairs.map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`).join('&')
      url += (url.includes('?') ? '&' : '?') + qs
    }

    return { method, url, bodyObj }
  }, [tool, paramList, paramModes, values, runtimeContext, runtimeInputs])

  if (!node) return null

  function handleAddCustomParam() {
    const k = customKey.trim()
    if (!k) return
    setParamModes((prev) => ({ ...prev, [k]: prev[k] || 'fixed' }))
    setValues((prev) => ({ ...prev, [k]: prev[k] ?? '' }))
    setCustomKey('')
  }

  function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!node) return
    const cleanedBlueprintValues: Record<string, string> = {}
    for (const [k, v] of Object.entries(values)) {
      const m = paramModes[k] || 'dynamic'
      const isTemplate = v.includes('{{') && v.includes('}}')
      if (m === 'fixed' || isTemplate) {
        if (v !== '') cleanedBlueprintValues[k] = v
      }
    }
    onSave(node.id, paramModes, cleanedBlueprintValues)
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
          <div className="min-w-0">
            <h3 className="text-sm font-bold text-slate-900 truncate">
              ตั้งค่า Parameters (SOP Blueprint): {tool?.name || node.tool}
            </h3>
            <p className="text-[11px] text-slate-500 truncate">{node.label}</p>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            {onEditTool && tool && (
              <button
                type="button"
                onClick={() => {
                  onClose()
                  onEditTool(tool.id)
                }}
                className="rounded border border-slate-300 bg-white px-2.5 py-1 text-[11px] font-semibold text-slate-700 hover:bg-slate-100"
              >
                ✎ แก้ไข Tool นี้
              </button>
            )}
            <button
              type="button"
              onClick={onClose}
              className="rounded px-2 py-1 text-xs font-bold text-slate-500 hover:bg-slate-200"
            >
              ✕ ปิด
            </button>
          </div>
        </div>

        <form onSubmit={handleSubmit} className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
          <div className="rounded-lg border border-sky-200 bg-sky-50 px-3 py-2 text-[11px] text-sky-900">
            <strong>แยกค่า SOP กับข้อมูลลูกค้า:</strong> เลือก <strong>📌 Fixed</strong> สำหรับค่าคงที่/Query ที่ต้องการเซฟไปพร้อม SOP และเลือก{' '}
            <strong>✏️ Runtime</strong> สำหรับช่องที่ให้กรอกตอนรัน (จะแสดงเป็นช่องว่างบนการ์ดโหนดและไม่เซฟข้อมูลลูกค้าลง SOP)
          </div>

          <div className="space-y-2.5">
            {paramList.map((p) => {
              const pMode: PayloadMode = paramModes[p.key] || p.mode || 'dynamic'
              const isFixed = pMode === 'fixed'
              return (
                <div key={`${p.in}-${p.key}`} className="rounded-lg border border-slate-200 p-2.5 bg-white space-y-2">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-1.5">
                      <span className="rounded bg-sky-100 px-1.5 py-0.5 text-[10px] font-bold uppercase text-sky-800">
                        {p.in}
                      </span>
                      <code className="text-xs font-bold text-slate-800">{p.key}</code>
                      {p.label && <span className="text-[11px] text-slate-500">({p.label})</span>}
                      {p.required && <span className="text-[10px] font-bold text-rose-600">*required</span>}
                    </div>

                    <div className="flex rounded-lg border border-slate-300 bg-slate-50 p-0.5 text-[11px] font-semibold">
                      <button
                        type="button"
                        onClick={() => setParamModes((prev) => ({ ...prev, [p.key]: 'fixed' }))}
                        className={`rounded-md px-2 py-0.5 ${
                          isFixed ? 'bg-amber-500 text-white' : 'text-slate-600 hover:text-slate-900'
                        }`}
                      >
                        📌 Fixed (เซฟลง SOP)
                      </button>
                      <button
                        type="button"
                        onClick={() => setParamModes((prev) => ({ ...prev, [p.key]: 'dynamic' }))}
                        className={`rounded-md px-2 py-0.5 ${
                          !isFixed ? 'bg-violet-600 text-white' : 'text-slate-600 hover:text-slate-900'
                        }`}
                      >
                        ✏️ Runtime (กรอกตอนรัน)
                      </button>
                    </div>
                  </div>

                  {isFixed ? (
                    <div>
                      <input
                        type="text"
                        value={values[p.key] ?? ''}
                        onChange={(e) => setValues((prev) => ({ ...prev, [p.key]: e.target.value }))}
                        placeholder="ใส่ค่าคงที่หรือ Query ที่ต้องการเซฟติดไปกับ SOP เช่น 90 หรือ deny"
                        className="w-full rounded border border-amber-300 bg-amber-50/30 px-2.5 py-1.5 font-mono text-xs"
                      />
                    </div>
                  ) : (
                    <div className="space-y-1.5">
                      <input
                        type="text"
                        value={values[p.key] ?? ''}
                        onChange={(e) => setValues((prev) => ({ ...prev, [p.key]: e.target.value }))}
                        placeholder="เว้นว่างเพื่อให้กรอกบนการ์ดตอนรัน หรือใส่ Template เช่น {{alert.src_ip}} / {{prev.verdict}}"
                        className="w-full rounded border border-slate-300 px-2.5 py-1.5 font-mono text-xs"
                      />
                      <div className="flex flex-wrap items-center gap-1">
                        <span className="text-[10px] text-slate-400">Template (ไม่เก็บค่าลูกค้า):</span>
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
                        <button
                          type="button"
                          onClick={() => setValues((prev) => ({ ...prev, [p.key]: '' }))}
                          className="rounded border border-slate-200 bg-slate-50 px-1.5 py-0.5 text-[10px] text-slate-600 hover:bg-slate-100"
                        >
                          ปล่อยเป็นช่องว่าง
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              )
            })}

            {paramList.length === 0 && (
              <div className="rounded border border-dashed border-slate-300 p-3 text-center text-xs text-slate-500">
                Tool นี้ยังไม่ได้กำหนด URL Parameters — สามารถเพิ่ม Parameter ด้านล่าง หรือตั้งค่าที่หน้าจัดการ Tools
              </div>
            )}

            <div className="flex gap-2 pt-1">
              <input
                type="text"
                value={customKey}
                onChange={(e) => setCustomKey(e.target.value)}
                placeholder="เพิ่ม Parameter Key อื่นๆ (เช่น filter, env)"
                className="flex-1 rounded border border-slate-300 px-2.5 py-1 text-xs"
              />
              <button
                type="button"
                onClick={handleAddCustomParam}
                className="rounded border border-slate-300 bg-slate-100 px-3 py-1 text-xs font-semibold text-slate-700 hover:bg-slate-200"
              >
                + เพิ่ม Parameter
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
              บันทึกโครง SOP
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
