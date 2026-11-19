import { useState, useMemo, useEffect, type FormEvent } from 'react'
import type {
  HTTPMethod,
  ParamLocation,
  PayloadMode,
  ToolCategory,
  ToolCommandType,
  ToolDefinition,
  ToolParameter,
  ToolPayload,
} from './types'
import { createTool, updateTool, deleteTool } from './api'

interface ToolManagerModalProps {
  open: boolean
  tools: ToolDefinition[]
  loadError?: string
  onClose: () => void
  onChanged: () => Promise<void>
}

interface FormState {
  id: string
  name: string
  category: ToolCategory
  description: string
  keywordsText: string
  command_type: ToolCommandType
  http_method: HTTPMethod
  endpoint: string
  payload_mode: PayloadMode
  params: ToolParameter[]
  mockOutputText: string
  enabled: boolean
}

const EMPTY_FORM: FormState = {
  id: '',
  name: '',
  category: 'lookup',
  description: '',
  keywordsText: '',
  command_type: 'http-api',
  http_method: 'GET',
  endpoint: '',
  payload_mode: 'dynamic',
  params: [],
  mockOutputText: '',
  enabled: true,
}

function slugifyName(name: string): string {
  return name
    .toLowerCase()
    .trim()
    .replace(/[\s-]+/g, '_')
    .replace(/[^a-z0-9_]+/g, '_')
    .replace(/_+/g, '_')
    .replace(/^_+|_+$/g, '')
}

function extractUrlPlaceholders(endpoint: string): string[] {
  const matches = endpoint.matchAll(/\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}/g)
  const seen = new Set<string>()
  for (const m of matches) {
    if (m[1]) seen.add(m[1])
  }
  return Array.from(seen)
}

export function ToolManagerModal({ open, tools, loadError, onClose, onChanged }: ToolManagerModalProps) {
  const [filterCat, setFilterCat] = useState<'all' | ToolCategory>('all')
  const [search, setSearch] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [idTouched, setIdTouched] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')

  useEffect(() => {
    if (!open) return
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [open, onClose])

  const filteredTools = useMemo(() => {
    const q = search.trim().toLowerCase()
    return tools.filter((t) => {
      if (filterCat !== 'all' && t.category !== filterCat) return false
      if (!q) return true
      const keywords = Array.isArray(t.keywords) ? t.keywords : []
      return (
        (t.name || '').toLowerCase().includes(q) ||
        (t.id || '').toLowerCase().includes(q) ||
        (t.description || '').toLowerCase().includes(q) ||
        keywords.some((k) => k.toLowerCase().includes(q))
      )
    })
  }, [tools, filterCat, search])

  const urlPreview = useMemo(() => {
    let url = form.endpoint.trim()
    if (!url) return ''
    const queryParts: string[] = []
    for (const p of form.params) {
      const k = p.key.trim()
      if (!k) continue
      const v = (p.value || '').trim() || `{{${k}}}`
      if (p.in === 'path') {
        url = url.replaceAll(`{{${k}}}`, v)
      } else if (p.in === 'query') {
        queryParts.push(`${k}=${v}`)
      }
    }
    if (queryParts.length > 0) {
      url += (url.includes('?') ? '&' : '?') + queryParts.join('&')
    }
    return `${form.http_method} ${url}`
  }, [form.endpoint, form.http_method, form.params])

  if (!open) return null

  function startCreate() {
    setEditingId(null)
    setIdTouched(false)
    setForm(EMPTY_FORM)
    setFormError('')
  }

  function startEdit(tool: ToolDefinition) {
    setEditingId(tool.id)
    setIdTouched(true)
    setForm({
      id: tool.id,
      name: tool.name,
      category: tool.category,
      description: tool.description,
      keywordsText: (Array.isArray(tool.keywords) ? tool.keywords : []).join(', '),
      command_type: tool.command_type || 'http-api',
      http_method: tool.http_method || (tool.category === 'lookup' ? 'GET' : 'POST'),
      endpoint: tool.endpoint || '',
      payload_mode: tool.payload_mode || 'dynamic',
      params: Array.isArray(tool.params) ? tool.params.map((p) => ({ ...p })) : [],
      mockOutputText:
        tool.mock_output && Object.keys(tool.mock_output).length > 0
          ? JSON.stringify(tool.mock_output, null, 2)
          : '',
      enabled: tool.enabled,
    })
    setFormError('')
  }

  function handleSyncParamsFromUrl() {
    const keys = extractUrlPlaceholders(form.endpoint)
    if (keys.length === 0) return
    setForm((prev) => {
      const existing = new Set(prev.params.map((p) => p.key))
      const added: ToolParameter[] = keys
        .filter((k) => !existing.has(k))
        .map((k) => ({
          key: k,
          in: 'path',
          mode: prev.payload_mode,
          value: `{{alert.${k === 'ip' ? 'src_ip' : k}}}`,
          required: true,
        }))
      return { ...prev, params: [...prev.params, ...added] }
    })
  }

  async function handleToggleEnabled(tool: ToolDefinition) {
    setFormError('')
    try {
      await updateTool(tool.id, {
        id: tool.id,
        name: tool.name,
        category: tool.category,
        description: tool.description,
        keywords: Array.isArray(tool.keywords) ? tool.keywords : [],
        command_type: tool.command_type || 'http-api',
        http_method: tool.http_method,
        endpoint: tool.endpoint || '',
        payload_mode: tool.payload_mode,
        params: tool.params,
        payload_template: tool.payload_template,
        mock_output: tool.mock_output,
        enabled: !tool.enabled,
      })
      await onChanged()
      if (editingId === tool.id) {
        setForm((prev) => ({ ...prev, enabled: !tool.enabled }))
      }
    } catch (e) {
      setFormError(e instanceof Error ? e.message : 'อัปเดตสถานะไม่สำเร็จ')
    }
  }

  async function handleDelete(tool: ToolDefinition) {
    if (!window.confirm(`ลบ Tool "${tool.name}" (${tool.id}) ใช่หรือไม่?`)) return
    setFormError('')
    try {
      await deleteTool(tool.id)
      await onChanged()
      if (editingId === tool.id) startCreate()
    } catch (e) {
      setFormError(e instanceof Error ? e.message : 'ลบ Tool ไม่สำเร็จ')
    }
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setFormError('')

    let parsedMock: Record<string, unknown> | undefined
    const rawMock = form.mockOutputText.trim()
    if (rawMock !== '') {
      try {
        const parsed = JSON.parse(rawMock)
        if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
          setFormError('Mock Output ต้องเป็น JSON Object เช่น {"verdict": "Clean"}')
          return
        }
        parsedMock = parsed as Record<string, unknown>
      } catch {
        setFormError('รูปแบบ JSON ของ Mock Output ไม่ถูกต้อง')
        return
      }
    }

    const keywords = form.keywordsText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)

    const cleanedParams = form.params
      .map((p) => ({
        ...p,
        key: p.key.trim(),
        value: (p.value || '').trim(),
      }))
      .filter((p) => p.key !== '')

    const payload: ToolPayload = {
      id: (form.id || slugifyName(form.name)).trim(),
      name: form.name.trim(),
      category: form.category,
      description: form.description.trim(),
      keywords,
      command_type: form.command_type,
      http_method: form.http_method,
      endpoint: form.endpoint.trim() || undefined,
      payload_mode: form.payload_mode,
      params: cleanedParams,
      mock_output: parsedMock,
      enabled: form.enabled,
    }

    setSaving(true)
    try {
      if (editingId) {
        await updateTool(editingId, payload)
      } else {
        await createTool(payload)
      }
      await onChanged()
      startCreate()
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'บันทึก Tool ไม่สำเร็จ')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="flex h-[88vh] w-full max-w-5xl flex-col overflow-hidden rounded-xl bg-white shadow-2xl border border-slate-200">
        <div className="flex shrink-0 items-center justify-between border-b border-slate-200 px-5 py-3 bg-slate-50">
          <div>
            <h2 className="text-base font-bold text-slate-900">จัดการ SOC Tools & AI Mapping Catalog</h2>
            <p className="text-xs text-slate-500">
              AI จะใช้ชื่อ คำอธิบาย และ Keywords ของ Tool ที่เปิดใช้งาน เพื่อจับคู่กับขั้นตอนใน SOP อัตโนมัติ
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg px-2.5 py-1 text-sm font-bold text-slate-500 hover:bg-slate-200 hover:text-slate-800"
          >
            ✕ ปิด
          </button>
        </div>

        {loadError && (
          <div className="shrink-0 border-b border-amber-200 bg-amber-50 px-5 py-2 text-xs text-amber-800 flex items-center justify-between gap-2">
            <span>{loadError}</span>
            <button
              type="button"
              onClick={() => void onChanged()}
              className="rounded border border-amber-300 bg-white px-2 py-0.5 text-[11px] font-semibold hover:bg-amber-100"
            >
              ลองโหลดใหม่
            </button>
          </div>
        )}

        <div className="grid min-h-0 flex-1 grid-cols-1 md:grid-cols-12 overflow-y-auto md:overflow-hidden">
          <div className="md:col-span-7 flex min-h-0 flex-col border-b md:border-b-0 md:border-r border-slate-200 md:overflow-hidden">
            <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-slate-200 p-3 bg-white">
              <div className="flex rounded-lg border border-slate-200 bg-slate-100 p-0.5 text-xs font-semibold">
                {(['all', 'lookup', 'action'] as const).map((cat) => (
                  <button
                    key={cat}
                    type="button"
                    onClick={() => setFilterCat(cat)}
                    className={`rounded-md px-2.5 py-1 capitalize ${
                      filterCat === cat ? 'bg-white text-blue-700 shadow-xs' : 'text-slate-600 hover:text-slate-900'
                    }`}
                  >
                    {cat}
                  </button>
                ))}
              </div>
              <input
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="ค้นหา Tool..."
                className="flex-1 min-w-[140px] rounded-lg border border-slate-300 px-2.5 py-1 text-xs"
              />
              <button
                type="button"
                onClick={startCreate}
                className="rounded-lg bg-blue-600 px-3 py-1.5 text-xs font-bold text-white hover:bg-blue-700"
              >
                + สร้าง Tool ใหม่
              </button>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto p-3 space-y-2 bg-slate-50/60">
              {filteredTools.map((t) => {
                const keywords = Array.isArray(t.keywords) ? t.keywords : []
                return (
                  <div
                    key={t.id}
                    className={`rounded-lg border p-3 bg-white transition ${
                      editingId === t.id ? 'border-blue-500 ring-2 ring-blue-100' : 'border-slate-200'
                    } ${!t.enabled ? 'opacity-60' : ''}`}
                  >
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5 flex-wrap">
                          <span className="text-sm font-bold text-slate-900">{t.name}</span>
                          <code className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-600">{t.id}</code>
                          <span
                            className={`rounded px-1.5 py-0.5 text-[10px] font-bold uppercase ${
                              t.category === 'lookup' ? 'bg-sky-100 text-sky-800' : 'bg-indigo-100 text-indigo-800'
                            }`}
                          >
                            {t.category}
                          </span>
                          <span
                            className={`rounded px-1.5 py-0.5 text-[10px] font-semibold ${
                              t.is_builtin ? 'bg-amber-100 text-amber-800' : 'bg-emerald-100 text-emerald-800'
                            }`}
                          >
                            {t.is_builtin ? 'Built-in' : 'Custom'}
                          </span>
                        </div>
                        <p className="mt-1 text-xs text-slate-600">{t.description}</p>
                        {t.endpoint && (
                          <div className="mt-1 font-mono text-[10px] text-slate-500 truncate">
                            <span className="font-bold text-slate-700">{t.http_method || 'GET'}</span> {t.endpoint}
                          </div>
                        )}
                        {keywords.length > 0 && (
                          <div className="mt-1.5 flex flex-wrap gap-1">
                            {keywords.map((kw) => (
                              <span key={kw} className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-700">
                                {kw}
                              </span>
                            ))}
                            {(t.params?.length || 0) > 0 && (
                              <span className="rounded bg-sky-50 px-1.5 py-0.5 text-[10px] font-semibold text-sky-700 border border-sky-200">
                                {t.params!.length} params ({t.payload_mode || 'dynamic'})
                              </span>
                            )}
                          </div>
                        )}
                      </div>

                      <div className="flex flex-col items-end gap-1.5 shrink-0">
                        <label className="flex items-center gap-1 text-[11px] font-semibold text-slate-700 cursor-pointer">
                          <input
                            type="checkbox"
                            checked={t.enabled}
                            onChange={() => void handleToggleEnabled(t)}
                            className="accent-blue-600"
                          />
                          {t.enabled ? 'Enabled' : 'Disabled'}
                        </label>
                        <div className="flex items-center gap-1.5">
                          <button
                            type="button"
                            onClick={() => startEdit(t)}
                            className="rounded border border-slate-300 px-2 py-0.5 text-[11px] font-semibold text-slate-700 hover:bg-slate-100"
                          >
                            แก้ไข
                          </button>
                          {!t.is_builtin && (
                            <button
                              type="button"
                              onClick={() => void handleDelete(t)}
                              className="rounded border border-rose-200 px-2 py-0.5 text-[11px] font-semibold text-rose-600 hover:bg-rose-50"
                            >
                              ลบ
                            </button>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                )
              })}
              {filteredTools.length === 0 && (
                <div className="py-8 text-center text-xs text-slate-400">ไม่พบ Tool ที่ตรงกับการค้นหา</div>
              )}
            </div>
          </div>

          <form onSubmit={handleSubmit} className="md:col-span-5 flex min-h-0 flex-col overflow-y-auto p-4 gap-3 bg-white">
            <div className="flex items-center justify-between border-b pb-2">
              <h3 className="text-sm font-bold text-slate-900">
                {editingId ? `แก้ไข Tool: ${editingId}` : 'สร้าง Tool ใหม่'}
              </h3>
              {editingId && (
                <button type="button" onClick={startCreate} className="text-xs text-blue-600 hover:underline">
                  + สลับไปสร้างใหม่
                </button>
              )}
            </div>

            {formError && (
              <div className="rounded border border-rose-200 bg-rose-50 p-2 text-xs text-rose-700">{formError}</div>
            )}

            <div className="grid grid-cols-2 gap-2">
              <label className="col-span-2 text-xs font-semibold text-slate-700">
                ชื่อ Tool *
                <input
                  type="text"
                  required
                  value={form.name}
                  onChange={(e) => {
                    const name = e.target.value
                    setForm((prev) => ({
                      ...prev,
                      name,
                      id: !editingId && !idTouched ? slugifyName(name) : prev.id,
                    }))
                  }}
                  placeholder="เช่น CrowdStrike EDR หรือ Splunk SIEM"
                  className="mt-1 w-full rounded border border-slate-300 px-2.5 py-1.5 text-xs font-normal"
                />
              </label>

              <label className="text-xs font-semibold text-slate-700">
                Tool ID (Slug)
                <input
                  type="text"
                  disabled={Boolean(editingId)}
                  value={form.id}
                  onChange={(e) => {
                    setIdTouched(true)
                    setForm((prev) => ({ ...prev, id: e.target.value }))
                  }}
                  placeholder="อัตโนมัติ หรือ crowdstrike_edr"
                  className="mt-1 w-full rounded border border-slate-300 px-2.5 py-1.5 font-mono text-xs font-normal disabled:bg-slate-100"
                />
              </label>

              <label className="text-xs font-semibold text-slate-700">
                Category *
                <select
                  value={form.category}
                  onChange={(e) => setForm((prev) => ({ ...prev, category: e.target.value as ToolCategory }))}
                  className="mt-1 w-full rounded border border-slate-300 px-2.5 py-1.5 text-xs font-normal"
                >
                  <option value="lookup">lookup (ตรวจสอบ / ค้นหา)</option>
                  <option value="action">action (ดำเนินการ / ระงับ)</option>
                </select>
              </label>
            </div>

            <label className="text-xs font-semibold text-slate-700">
              คำอธิบายสำหรับ AI (AI Description) *
              <textarea
                required
                rows={2}
                value={form.description}
                onChange={(e) => setForm((prev) => ({ ...prev, description: e.target.value }))}
                placeholder="อธิบายว่า Tool นี้ทำอะไร เพื่อใช้ให้ AI จับคู่กับประโยคใน SOP ได้แม่นยำ"
                className="mt-1 w-full rounded border border-slate-300 p-2 text-xs font-normal"
              />
            </label>

            <label className="text-xs font-semibold text-slate-700">
              Keywords (คั่นด้วยจุลภาค , สำหรับ Fallback & AI Hints)
              <input
                type="text"
                value={form.keywordsText}
                onChange={(e) => setForm((prev) => ({ ...prev, keywordsText: e.target.value }))}
                placeholder="splunk, siem, log, ค้นหา log"
                className="mt-1 w-full rounded border border-slate-300 px-2.5 py-1.5 text-xs font-normal"
              />
            </label>

            <div className="grid grid-cols-3 gap-2">
              <label className="text-xs font-semibold text-slate-700">
                CACAO Type
                <select
                  value={form.command_type}
                  onChange={(e) => setForm((prev) => ({ ...prev, command_type: e.target.value as ToolCommandType }))}
                  className="mt-1 w-full rounded border border-slate-300 px-2 py-1.5 text-xs font-normal"
                >
                  <option value="http-api">http-api</option>
                  <option value="manual">manual</option>
                  <option value="bash">bash</option>
                  <option value="ssh">ssh</option>
                </select>
              </label>

              <label className="text-xs font-semibold text-slate-700">
                HTTP Method
                <select
                  value={form.http_method}
                  onChange={(e) => setForm((prev) => ({ ...prev, http_method: e.target.value as HTTPMethod }))}
                  className="mt-1 w-full rounded border border-slate-300 px-2 py-1.5 text-xs font-normal"
                >
                  <option value="GET">GET</option>
                  <option value="POST">POST</option>
                  <option value="PUT">PUT</option>
                  <option value="DELETE">DELETE</option>
                </select>
              </label>

              <label className="text-xs font-semibold text-slate-700">
                Default Payload
                <select
                  value={form.payload_mode}
                  onChange={(e) => setForm((prev) => ({ ...prev, payload_mode: e.target.value as PayloadMode }))}
                  className="mt-1 w-full rounded border border-slate-300 px-2 py-1.5 text-xs font-normal"
                >
                  <option value="dynamic">Dynamic</option>
                  <option value="fixed">Fixed</option>
                </select>
              </label>
            </div>

            <div className="text-xs font-semibold text-slate-700">
              <div className="flex items-center justify-between">
                <label htmlFor="tool-endpoint-input">Endpoint URL (รองรับ {'{{param}}'} เช่น /ip/{'{{ip}}'})</label>
                <button
                  type="button"
                  onClick={handleSyncParamsFromUrl}
                  className="text-[11px] font-medium text-blue-600 hover:underline"
                >
                  + ดึงจาก URL
                </button>
              </div>
              <input
                id="tool-endpoint-input"
                type="text"
                value={form.endpoint}
                onChange={(e) => setForm((prev) => ({ ...prev, endpoint: e.target.value }))}
                placeholder="https://api.example.com/v1/ip/{{ip}}"
                className="mt-1 w-full rounded border border-slate-300 px-2.5 py-1.5 font-mono text-xs font-normal"
              />
            </div>

            <div className="rounded-lg border border-slate-200 bg-slate-50 p-2.5 space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-bold text-slate-800">
                  URL & Input Parameters ({form.params.length})
                </span>
                <button
                  type="button"
                  onClick={() =>
                    setForm((prev) => ({
                      ...prev,
                      params: [
                        ...prev.params,
                        { key: '', in: 'query', mode: prev.payload_mode, value: '', required: false },
                      ],
                    }))
                  }
                  className="rounded bg-slate-800 px-2 py-0.5 text-[11px] font-semibold text-white hover:bg-slate-900"
                >
                  + เพิ่ม Parameter
                </button>
              </div>

              {form.params.map((p, idx) => (
                <div key={idx} className="grid grid-cols-12 gap-1 items-center bg-white p-1.5 rounded border border-slate-200">
                  <input
                    type="text"
                    value={p.key}
                    onChange={(e) =>
                      setForm((prev) => ({
                        ...prev,
                        params: prev.params.map((item, i) => (i === idx ? { ...item, key: e.target.value } : item)),
                      }))
                    }
                    placeholder="key"
                    className="col-span-3 rounded border border-slate-300 px-1.5 py-1 font-mono text-[11px]"
                  />
                  <select
                    value={p.in}
                    onChange={(e) =>
                      setForm((prev) => ({
                        ...prev,
                        params: prev.params.map((item, i) =>
                          i === idx ? { ...item, in: e.target.value as ParamLocation } : item
                        ),
                      }))
                    }
                    className="col-span-2 rounded border border-slate-300 px-1 py-1 text-[11px]"
                  >
                    <option value="path">path</option>
                    <option value="query">query</option>
                    <option value="body">body</option>
                    <option value="header">header</option>
                  </select>
                  <select
                    value={p.mode}
                    onChange={(e) =>
                      setForm((prev) => ({
                        ...prev,
                        params: prev.params.map((item, i) =>
                          i === idx ? { ...item, mode: e.target.value as PayloadMode } : item
                        ),
                      }))
                    }
                    className="col-span-2 rounded border border-slate-300 px-1 py-1 text-[11px]"
                  >
                    <option value="dynamic">dyn</option>
                    <option value="fixed">fixed</option>
                  </select>
                  <input
                    type="text"
                    value={p.value || ''}
                    onChange={(e) =>
                      setForm((prev) => ({
                        ...prev,
                        params: prev.params.map((item, i) => (i === idx ? { ...item, value: e.target.value } : item)),
                      }))
                    }
                    placeholder={p.mode === 'fixed' ? 'value' : '{{alert.src_ip}}'}
                    className="col-span-3 rounded border border-slate-300 px-1.5 py-1 font-mono text-[11px]"
                  />
                  <label className="col-span-1 flex items-center justify-center text-[10px]" title="Required">
                    <input
                      type="checkbox"
                      checked={Boolean(p.required)}
                      onChange={(e) =>
                        setForm((prev) => ({
                          ...prev,
                          params: prev.params.map((item, i) =>
                            i === idx ? { ...item, required: e.target.checked } : item
                          ),
                        }))
                      }
                    />
                  </label>
                  <button
                    type="button"
                    onClick={() =>
                      setForm((prev) => ({
                        ...prev,
                        params: prev.params.filter((_, i) => i !== idx),
                      }))
                    }
                    className="col-span-1 text-center text-xs font-bold text-rose-500 hover:text-rose-700"
                    title="ลบ Parameter"
                  >
                    ✕
                  </button>
                </div>
              ))}

              {urlPreview && (
                <div className="rounded bg-slate-900 px-2 py-1 font-mono text-[10px] text-sky-300 break-all">
                  {urlPreview}
                </div>
              )}
            </div>

            <div className="text-xs font-semibold text-slate-700">
              <div className="flex items-center justify-between">
                <label htmlFor="tool-mock-output">Mock Sandbox Output (JSON, ไม่บังคับ)</label>
                <button
                  type="button"
                  onClick={() =>
                    setForm((prev) => ({
                      ...prev,
                      mockOutputText: JSON.stringify(
                        prev.category === 'lookup'
                          ? { is_malicious: true, matched_events: 5, verdict: 'Suspicious Activity' }
                          : { status: 'executed', result: 'Action Completed' },
                        null,
                        2
                      ),
                    }))
                  }
                  className="text-[11px] font-medium text-blue-600 hover:underline"
                >
                  ใส่ตัวอย่าง JSON
                </button>
              </div>
              <textarea
                id="tool-mock-output"
                rows={4}
                value={form.mockOutputText}
                onChange={(e) => setForm((prev) => ({ ...prev, mockOutputText: e.target.value }))}
                placeholder='{"is_malicious": true, "verdict": "Suspicious"}'
                className="mt-1 w-full rounded border border-slate-300 p-2 font-mono text-[11px] font-normal"
              />
            </div>

            <label className="flex items-center gap-2 text-xs font-semibold text-slate-700">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => setForm((prev) => ({ ...prev, enabled: e.target.checked }))}
                className="accent-blue-600"
              />
              เปิดใช้งาน Tool นี้ในการจับคู่ของ AI
            </label>

            <div className="mt-auto flex justify-end gap-2 border-t pt-3">
              <button
                type="button"
                onClick={onClose}
                className="rounded-lg border border-slate-300 px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-50"
              >
                ปิด
              </button>
              <button
                type="submit"
                disabled={saving}
                className="rounded-lg bg-blue-600 px-4 py-1.5 text-xs font-bold text-white hover:bg-blue-700 disabled:opacity-50"
              >
                {saving ? 'กำลังบันทึก...' : editingId ? 'บันทึกการแก้ไข' : 'สร้าง Tool'}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  )
}
