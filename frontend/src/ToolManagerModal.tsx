import { useState, useMemo, useEffect, useRef, type FormEvent, type KeyboardEvent as ReactKeyboardEvent } from 'react'
import type {
  HTTPMethod,
  ParamLocation,
  PayloadMode,
  ToolAuth,
  ToolCategory,
  ToolDefinition,
  ToolPayload,
} from './types'
import { createTool, updateTool, deleteTool, testToolDraft, type ToolTestResponse } from './api'
import {
  DYNAMIC_VAR_CHIPS,
  EMPTY_FORM,
  QUICK_TEMPLATES,
  lookupCtxPath,
  mockRowsToObject,
  objectToMockRows,
  parseSmartUrl,
  slugifyName,
  toolToFormState,
  type QuickTemplate,
  type ToolFormState,
} from './toolTemplates'
import { ToolAuthSection } from './ToolAuthSection'

interface ToolManagerModalProps {
  open: boolean
  tools: ToolDefinition[]
  loadError?: string
  initialCategory?: ToolCategory
  initialEditToolId?: string | null
  runtimeContext?: Record<string, unknown>
  onClose: () => void
  onChanged: () => Promise<void>
}

export function ToolManagerModal({
  open,
  tools,
  loadError,
  initialCategory,
  initialEditToolId,
  runtimeContext = {},
  onClose,
  onChanged,
}: ToolManagerModalProps) {
  const [filterCat, setFilterCat] = useState<'all' | ToolCategory>('all')
  const [search, setSearch] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [idTouched, setIdTouched] = useState(false)
  const [methodTouched, setMethodTouched] = useState(false)
  const [form, setForm] = useState<ToolFormState>(EMPTY_FORM)
  const [keywordInput, setKeywordInput] = useState('')
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')
  const [toastMsg, setToastMsg] = useState('')
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<ToolTestResponse | null>(null)
  const [sopTestText, setSopTestText] = useState('')

  const toolsRef = useRef(tools)
  toolsRef.current = tools

  useEffect(() => {
    if (!open) return
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [open, onClose])

  useEffect(() => {
    if (!open) return
    setTestResult(null)
    setFormError('')
    setToastMsg('')
    if (initialEditToolId) {
      const target = toolsRef.current.find((t) => t.id === initialEditToolId)
      if (target) {
        setEditingId(target.id)
        setIdTouched(true)
        setMethodTouched(true)
        setKeywordInput('')
        setForm(toolToFormState(target, false))
        return
      }
    }
    if (initialCategory) {
      setEditingId(null)
      setIdTouched(false)
      setMethodTouched(false)
      setKeywordInput('')
      setForm({
        ...EMPTY_FORM,
        category: initialCategory,
        http_method: initialCategory === 'lookup' ? 'GET' : 'POST',
      })
    }
  }, [open, initialCategory, initialEditToolId])

  function loadToolIntoForm(tool: ToolDefinition, asClone: boolean) {
    setEditingId(asClone ? null : tool.id)
    setIdTouched(!asClone)
    setMethodTouched(true)
    setKeywordInput('')
    setTestResult(null)
    setFormError('')
    setForm(toolToFormState(tool, asClone))
  }

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
      const raw = (p.value || '').trim() || `{{${k}}}`
      const resolved =
        p.mode === 'dynamic'
          ? raw.replace(/\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}/g, (m, path: string) => lookupCtxPath(runtimeContext, path) || m)
          : raw
      if (p.in === 'path') {
        url = url.replaceAll(`{{${k}}}`, resolved)
      } else if (p.in === 'query') {
        queryParts.push(`${k}=${resolved}`)
      }
    }
    if (queryParts.length > 0) {
      url += (url.includes('?') ? '&' : '?') + queryParts.join('&')
    }
    return `${form.http_method} ${url}`
  }, [form.endpoint, form.http_method, form.params, runtimeContext])

  const sopMatchResult = useMemo(() => {
    const text = sopTestText.trim().toLowerCase()
    if (!text) return null
    const matchedKeywords = form.keywords.filter((kw) => kw.trim() && text.includes(kw.trim().toLowerCase()))
    const nameMatch = form.name.trim() !== '' && text.includes(form.name.trim().toLowerCase())
    return {
      matched: matchedKeywords.length > 0 || nameMatch,
      matchedKeywords,
      nameMatch,
    }
  }, [sopTestText, form.keywords, form.name])

  if (!open) return null

  function startCreate(category: ToolCategory = 'lookup') {
    setEditingId(null)
    setIdTouched(false)
    setMethodTouched(false)
    setKeywordInput('')
    setTestResult(null)
    setForm({
      ...EMPTY_FORM,
      category,
      http_method: category === 'lookup' ? 'GET' : 'POST',
    })
    setFormError('')
  }

  function applyTemplate(tpl: QuickTemplate) {
    const patch = tpl.apply()
    setEditingId(null)
    setIdTouched(false)
    setMethodTouched(true)
    setTestResult(null)
    setForm({
      ...EMPTY_FORM,
      ...patch,
    })
    setFormError('')
  }

  function handleSyncParamsFromUrl() {
    if (!form.endpoint.trim()) return
    const { cleanEndpoint, params: parsed } = parseSmartUrl(form.endpoint, form.payload_mode)
    setForm((prev) => {
      const existingKeys = new Set(prev.params.map((p) => `${p.in}:${p.key}`))
      const merged = [...prev.params]
      for (const item of parsed) {
        if (!existingKeys.has(`${item.in}:${item.key}`)) {
          merged.push(item)
        }
      }
      return { ...prev, endpoint: cleanEndpoint, params: merged }
    })
  }

  function addKeywordChip(raw: string) {
    const parts = raw
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    if (parts.length === 0) return
    setForm((prev) => {
      const existing = new Set(prev.keywords.map((k) => k.toLowerCase()))
      const next = [...prev.keywords]
      for (const p of parts) {
        if (!existing.has(p.toLowerCase())) {
          existing.add(p.toLowerCase())
          next.push(p)
        }
      }
      return { ...prev, keywords: next }
    })
    setKeywordInput('')
  }

  function handleKeywordKeyDown(e: ReactKeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      addKeywordChip(keywordInput)
    }
  }

  function buildPayloadFromForm(): ToolPayload | null {
    let parsedMock: Record<string, unknown> | undefined
    if (form.mockJsonMode) {
      const rawMock = form.mockOutputText.trim()
      if (rawMock !== '') {
        try {
          const parsed = JSON.parse(rawMock)
          if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
            setFormError('Mock Output ต้องเป็น JSON Object เช่น {"verdict": "Clean"}')
            return null
          }
          parsedMock = parsed as Record<string, unknown>
        } catch {
          setFormError('รูปแบบ JSON ของ Mock Output ไม่ถูกต้อง')
          return null
        }
      }
    } else {
      parsedMock = mockRowsToObject(form.mockRows)
    }

    const pendingKeywords = [...form.keywords]
    if (keywordInput.trim()) {
      for (const kw of keywordInput.split(',').map((s) => s.trim()).filter(Boolean)) {
        if (!pendingKeywords.some((k) => k.toLowerCase() === kw.toLowerCase())) {
          pendingKeywords.push(kw)
        }
      }
    }

    const cleanedParams = form.params
      .map((p) => ({
        ...p,
        key: p.key.trim(),
        value: (p.value || '').trim(),
      }))
      .filter((p) => p.key !== '')

    const authPayload: ToolAuth | undefined =
      form.command_type === 'http-api' && form.auth.type !== 'none'
        ? {
            type: form.auth.type,
            in: form.auth.type === 'api_key' ? form.auth.in || 'header' : 'header',
            key_name: (form.auth.key_name || '').trim() || undefined,
            prefix: (form.auth.prefix || '').trim() || undefined,
            env_var: (form.auth.env_var || '').trim().toUpperCase() || undefined,
            allow_runtime: Boolean(form.auth.allow_runtime),
          }
        : { type: 'none' }

    return {
      id: (form.id || slugifyName(form.name)).trim(),
      name: form.name.trim(),
      category: form.category,
      description: form.description.trim(),
      keywords: pendingKeywords,
      command_type: form.command_type,
      http_method: form.http_method,
      endpoint: form.endpoint.trim() || undefined,
      payload_mode: form.payload_mode,
      auth: authPayload,
      params: cleanedParams,
      mock_output: parsedMock,
      enabled: form.enabled,
    }
  }

  async function handleDryRunTest() {
    setFormError('')
    const payload = buildPayloadFromForm()
    if (!payload) return
    if (!payload.name) {
      setFormError('กรุณาใส่ชื่อ Tool ก่อนทดสอบ')
      return
    }
    setTesting(true)
    try {
      const res = await testToolDraft(payload, runtimeContext)
      setTestResult(res)
    } catch (e) {
      setFormError(e instanceof Error ? e.message : 'ทดสอบ Tool ไม่สำเร็จ')
    } finally {
      setTesting(false)
    }
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
        auth: tool.auth,
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
      setToastMsg(`ลบ Tool "${tool.name}" แล้ว`)
    } catch (e) {
      setFormError(e instanceof Error ? e.message : 'ลบ Tool ไม่สำเร็จ')
    }
  }

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setFormError('')
    setToastMsg('')
    const payload = buildPayloadFromForm()
    if (!payload) return

    setSaving(true)
    try {
      if (editingId) {
        await updateTool(editingId, payload)
        setToastMsg(`อัปเดต Tool "${payload.name}" เรียบร้อยแล้ว`)
      } else {
        await createTool(payload)
        setToastMsg(`สร้าง Tool "${payload.name}" เรียบร้อยแล้ว`)
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
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 p-3 md:p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="flex h-[90vh] w-full max-w-6xl flex-col overflow-hidden rounded-xl bg-white shadow-2xl border border-slate-200">
        <div className="flex shrink-0 items-center justify-between border-b border-slate-200 px-5 py-3 bg-slate-50">
          <div>
            <h2 className="text-base font-bold text-slate-900">จัดการ SOC Tools & AI Mapping Catalog</h2>
            <p className="text-xs text-slate-500">
              สร้าง/แก้ไข Tool ด้วย Template หรือวาง URL จริง แล้วทดสอบ Dry Run ได้ทันทีก่อนนำไปใช้บน Flowchart
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
          {/* Left: Tool Catalog List */}
          <div className="md:col-span-5 flex min-h-0 flex-col border-b md:border-b-0 md:border-r border-slate-200 md:overflow-hidden">
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
                className="flex-1 min-w-[120px] rounded-lg border border-slate-300 px-2.5 py-1 text-xs"
              />
              <button
                type="button"
                onClick={() => startCreate()}
                className="rounded-lg bg-blue-600 px-3 py-1.5 text-xs font-bold text-white hover:bg-blue-700"
              >
                + สร้างใหม่
              </button>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto p-3 space-y-2 bg-slate-50/60">
              {filteredTools.map((t) => {
                const keywords = Array.isArray(t.keywords) ? t.keywords : []
                const paramCount = t.params?.length || 0
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
                          {t.command_type === 'manual' ? (
                            <span className="rounded bg-purple-100 px-1.5 py-0.5 text-[10px] font-semibold text-purple-800">
                              🌐 Browser
                            </span>
                          ) : t.auth && t.auth.type !== 'none' ? (
                            <span
                              className={`rounded px-1.5 py-0.5 text-[10px] font-semibold ${
                                t.auth_configured
                                  ? 'bg-emerald-100 text-emerald-800'
                                  : 'bg-amber-100 text-amber-800'
                              }`}
                              title={
                                t.auth_configured
                                  ? `Configured via .env (${t.auth.env_var})`
                                  : t.auth.env_var
                                    ? `Env var ${t.auth.env_var} not set`
                                    : 'Session key auth'
                              }
                            >
                              {t.auth_configured ? '🔑 .env' : '🔑 Auth'}
                            </span>
                          ) : null}
                        </div>
                        <p className="mt-1 text-xs text-slate-600 line-clamp-2">{t.description}</p>
                        {t.endpoint && (
                          <div className="mt-1 font-mono text-[10px] text-slate-500 truncate">
                            <span className="font-bold text-slate-700">{t.http_method || 'GET'}</span> {t.endpoint}
                          </div>
                        )}
                        {(keywords.length > 0 || paramCount > 0) && (
                          <div className="mt-1.5 flex flex-wrap gap-1">
                            {keywords.map((kw) => (
                              <span key={kw} className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-700">
                                {kw}
                              </span>
                            ))}
                            {paramCount > 0 && (
                              <span className="rounded bg-sky-50 px-1.5 py-0.5 text-[10px] font-semibold text-sky-700 border border-sky-200">
                                {paramCount} params ({t.payload_mode || 'dynamic'})
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
                          {t.enabled ? 'On' : 'Off'}
                        </label>
                        <div className="flex items-center gap-1">
                          <button
                            type="button"
                            onClick={() => loadToolIntoForm(t, true)}
                            title="คัดลอกเป็น Tool ใหม่"
                            className="rounded border border-slate-300 px-1.5 py-0.5 text-[10px] font-semibold text-slate-600 hover:bg-slate-100"
                          >
                            คัดลอก
                          </button>
                          <button
                            type="button"
                            onClick={() => loadToolIntoForm(t, false)}
                            className="rounded border border-slate-300 px-2 py-0.5 text-[10px] font-semibold text-slate-700 hover:bg-slate-100"
                          >
                            แก้ไข
                          </button>
                          {!t.is_builtin && (
                            <button
                              type="button"
                              onClick={() => void handleDelete(t)}
                              className="rounded border border-rose-200 px-1.5 py-0.5 text-[10px] font-semibold text-rose-600 hover:bg-rose-50"
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

          {/* Right: Guided Tool Builder Form */}
          <form onSubmit={handleSubmit} className="md:col-span-7 flex min-h-0 flex-col overflow-y-auto p-4 gap-3 bg-white">
            <div className="flex items-center justify-between border-b pb-2">
              <h3 className="text-sm font-bold text-slate-900">
                {editingId ? `แก้ไข Tool: ${editingId}` : 'สร้าง Tool ใหม่'}
              </h3>
              {editingId && (
                <button type="button" onClick={() => startCreate()} className="text-xs text-blue-600 hover:underline">
                  + สลับไปสร้างใหม่
                </button>
              )}
            </div>

            {toastMsg && (
              <div className="rounded-lg border border-emerald-300 bg-emerald-50 px-3 py-2 text-xs font-semibold text-emerald-800">
                ✓ {toastMsg}
              </div>
            )}

            {formError && (
              <div className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
                {formError}
              </div>
            )}

            {!editingId && (
              <div className="rounded-lg border border-slate-200 bg-slate-50 p-2.5">
                <div className="text-[11px] font-bold text-slate-700 mb-1.5">
                  เริ่มต้นเร็วด้วย Template (1 คลิก):
                </div>
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-1.5">
                  {QUICK_TEMPLATES.map((tpl) => (
                    <button
                      key={tpl.id}
                      type="button"
                      onClick={() => applyTemplate(tpl)}
                      className="rounded-lg border border-slate-200 bg-white p-2 text-left hover:border-blue-400 hover:bg-blue-50/40 transition"
                    >
                      <div className="text-[11px] font-bold text-slate-800 truncate">{tpl.label}</div>
                      <div className="text-[10px] font-mono text-blue-600">{tpl.badge}</div>
                    </button>
                  ))}
                </div>
              </div>
            )}

            {/* Step 1: Name & Category */}
            <div className="grid grid-cols-1 sm:grid-cols-12 gap-2.5">
              <label className="sm:col-span-7 text-xs font-semibold text-slate-700">
                1. ชื่อ Tool *
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
                  placeholder="เช่น Shodan Host, Splunk SIEM, CrowdStrike"
                  className="mt-1 w-full rounded-lg border border-slate-300 px-2.5 py-1.5 text-xs font-normal"
                />
              </label>

              <div className="sm:col-span-5 text-xs font-semibold text-slate-700">
                ประเภท (Category) *
                <div className="mt-1 grid grid-cols-2 gap-1.5">
                  <button
                    type="button"
                    onClick={() =>
                      setForm((prev) => ({
                        ...prev,
                        category: 'lookup',
                        http_method: methodTouched ? prev.http_method : 'GET',
                      }))
                    }
                    className={`rounded-lg border px-2 py-1.5 text-xs font-bold transition ${
                      form.category === 'lookup'
                        ? 'border-sky-500 bg-sky-50 text-sky-900'
                        : 'border-slate-300 bg-white text-slate-600 hover:bg-slate-50'
                    }`}
                  >
                    🔍 Lookup
                  </button>
                  <button
                    type="button"
                    onClick={() =>
                      setForm((prev) => ({
                        ...prev,
                        category: 'action',
                        http_method: methodTouched ? prev.http_method : 'POST',
                      }))
                    }
                    className={`rounded-lg border px-2 py-1.5 text-xs font-bold transition ${
                      form.category === 'action'
                        ? 'border-indigo-500 bg-indigo-50 text-indigo-900'
                        : 'border-slate-300 bg-white text-slate-600 hover:bg-slate-50'
                    }`}
                  >
                    ⚡ Action
                  </button>
                </div>
              </div>
            </div>

            {/* Step 2: Endpoint, Execution Mode & Authentication */}
            <div className="rounded-lg border border-slate-200 bg-slate-50 p-3 space-y-2.5">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="text-xs font-bold text-slate-800">2. Endpoint URL, โหมดการทำงาน & Authentication</span>
                <div className="flex items-center gap-1.5">
                  <div className="flex rounded-lg border border-slate-300 bg-white p-0.5 text-[10px] font-bold">
                    <button
                      type="button"
                      onClick={() => setForm((prev) => ({ ...prev, command_type: 'http-api' }))}
                      className={`rounded px-2 py-0.5 ${
                        form.command_type === 'http-api' ? 'bg-blue-600 text-white' : 'text-slate-600 hover:text-slate-900'
                      }`}
                    >
                      API (http-api)
                    </button>
                    <button
                      type="button"
                      onClick={() => setForm((prev) => ({ ...prev, command_type: 'manual' }))}
                      className={`rounded px-2 py-0.5 ${
                        form.command_type === 'manual' ? 'bg-purple-600 text-white' : 'text-slate-600 hover:text-slate-900'
                      }`}
                    >
                      🌐 เปิดเว็บตรวจเอง
                    </button>
                  </div>
                  <button
                    type="button"
                    onClick={handleSyncParamsFromUrl}
                    className="rounded border border-blue-300 bg-blue-50 px-2 py-0.5 text-[11px] font-semibold text-blue-700 hover:bg-blue-100"
                  >
                    ⚡ แยก Params จาก URL
                  </button>
                </div>
              </div>

              {form.command_type === 'manual' && (
                <div className="rounded border border-purple-200 bg-purple-50 px-2.5 py-1.5 text-[11px] text-purple-900">
                  <strong>โหมดเปิดเว็บตรวจเอง (แก้ปัญหาเว็บติด Anti-bot / SSO):</strong> ใส่ URL Template ด้านล่าง เช่น{' '}
                  <code className="font-mono">https://www.virustotal.com/gui/ip-address/{'{{ip}}'}</code>{' '}
                  บนการ์ดโหนดจะมีปุ่มเปิดแท็บใหม่ให้ Analyst กดตรวจเองแล้วกดยืนยันผล
                </div>
              )}

              <div className="flex gap-2">
                {form.command_type !== 'manual' && (
                  <select
                    value={form.http_method}
                    onChange={(e) => {
                      setMethodTouched(true)
                      setForm((prev) => ({ ...prev, http_method: e.target.value as HTTPMethod }))
                    }}
                    aria-label="HTTP Method"
                    className="rounded-lg border border-slate-300 bg-white px-2 py-1.5 font-mono text-xs font-bold text-slate-800"
                  >
                    <option value="GET">GET</option>
                    <option value="POST">POST</option>
                    <option value="PUT">PUT</option>
                    <option value="DELETE">DELETE</option>
                  </select>
                )}
                <input
                  id="tool-endpoint-input"
                  type="text"
                  value={form.endpoint}
                  onChange={(e) => setForm((prev) => ({ ...prev, endpoint: e.target.value }))}
                  placeholder={
                    form.command_type === 'manual'
                      ? 'วาง URL หน้าเว็บ เช่น https://www.virustotal.com/gui/ip-address/{{ip}}'
                      : 'วาง URL เช่น https://api.shodan.io/shodan/host/{ip} หรือ ?ip=8.8.8.8'
                  }
                  className="flex-1 rounded-lg border border-slate-300 bg-white px-2.5 py-1.5 font-mono text-xs"
                />
              </div>

              {form.command_type === 'http-api' && (
                <ToolAuthSection
                  auth={form.auth}
                  editingId={editingId}
                  isConfiguredInEnv={Boolean(tools.find((t) => t.id === editingId)?.auth_configured)}
                  onChange={(nextAuth) => setForm((prev) => ({ ...prev, auth: nextAuth }))}
                />
              )}

              <div className="flex items-center justify-between pt-1">
                <span className="text-[11px] font-semibold text-slate-600">
                  รายการ Parameters ({form.params.length})
                </span>
                <button
                  type="button"
                  onClick={() =>
                    setForm((prev) => ({
                      ...prev,
                      params: [
                        ...prev.params,
                        {
                          key: '',
                          in: prev.http_method === 'GET' ? 'query' : 'body',
                          mode: prev.payload_mode,
                          value: '',
                          required: false,
                        },
                      ],
                    }))
                  }
                  className="rounded bg-slate-800 px-2 py-0.5 text-[11px] font-semibold text-white hover:bg-slate-900"
                >
                  + เพิ่ม Parameter
                </button>
              </div>

              {form.params.length > 0 && (
                <div className="space-y-1.5">
                  <div className="hidden sm:grid grid-cols-12 gap-1 px-1 text-[10px] font-bold uppercase text-slate-500">
                    <span className="col-span-3">Key</span>
                    <span className="col-span-2">ตำแหน่ง</span>
                    <span className="col-span-2">โหมด</span>
                    <span className="col-span-3">ค่า / ตัวแปร</span>
                    <span className="col-span-1 text-center">บังคับ</span>
                    <span className="col-span-1" />
                  </div>

                  {form.params.map((p, idx) => (
                    <div key={idx} className="rounded-lg border border-slate-200 bg-white p-2 space-y-1.5">
                      <div className="grid grid-cols-12 gap-1.5 items-center">
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
                          className="col-span-3 rounded border border-slate-300 px-2 py-1 font-mono text-[11px]"
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
                          <option value="dynamic">Dynamic</option>
                          <option value="fixed">Fixed</option>
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
                          placeholder={p.mode === 'fixed' ? 'ค่าคงที่' : '{{alert.src_ip}}'}
                          className="col-span-3 rounded border border-slate-300 px-2 py-1 font-mono text-[11px]"
                        />
                        <label className="col-span-1 flex items-center justify-center" title="Required">
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

                      <div className="flex flex-wrap items-center justify-between gap-2">
                        {p.mode === 'dynamic' ? (
                          <div className="flex flex-wrap items-center gap-1">
                            <span className="text-[10px] text-slate-400">แทรกตัวแปร:</span>
                            {DYNAMIC_VAR_CHIPS.map((chip) => (
                              <button
                                key={chip}
                                type="button"
                                onClick={() =>
                                  setForm((prev) => ({
                                    ...prev,
                                    params: prev.params.map((item, i) => (i === idx ? { ...item, value: chip } : item)),
                                  }))
                                }
                                className="rounded border border-violet-200 bg-violet-50 px-1.5 py-0.5 font-mono text-[10px] text-violet-700 hover:bg-violet-100"
                              >
                                {chip}
                              </button>
                            ))}
                          </div>
                        ) : (
                          <span />
                        )}
                        <label
                          className="inline-flex items-center gap-1 text-[10px] font-semibold text-amber-800 cursor-pointer"
                          title="ซ่อนค่าเป็น *** ใน log และไม่บันทึกลง SOP"
                        >
                          <input
                            type="checkbox"
                            checked={Boolean(p.is_secret)}
                            onChange={(e) =>
                              setForm((prev) => ({
                                ...prev,
                                params: prev.params.map((item, i) =>
                                  i === idx ? { ...item, is_secret: e.target.checked } : item
                                ),
                              }))
                            }
                            className="accent-amber-600"
                          />
                          🔑 ค่าความลับ (Secret)
                        </label>
                      </div>
                    </div>
                  ))}
                </div>
              )}

              {urlPreview && (
                <div className="rounded bg-slate-900 px-2.5 py-1.5 font-mono text-[11px] text-sky-300 break-all">
                  <span className="text-emerald-400 font-bold mr-1.5">PREVIEW:</span>
                  {urlPreview}
                </div>
              )}
            </div>

            {/* Step 3: AI Hints (Keywords Tag Chips + Optional Description) */}
            <div className="rounded-lg border border-slate-200 p-3 space-y-2">
              <div className="text-xs font-bold text-slate-800">3. คำค้นหาสำหรับจับคู่ SOP (Keywords & AI Hints)</div>

              <div className="flex flex-wrap items-center gap-1.5 rounded-lg border border-slate-300 p-1.5 bg-white">
                {form.keywords.map((kw) => (
                  <span
                    key={kw}
                    className="inline-flex items-center gap-1 rounded-full bg-blue-50 border border-blue-200 px-2 py-0.5 text-[11px] font-semibold text-blue-800"
                  >
                    {kw}
                    <button
                      type="button"
                      onClick={() =>
                        setForm((prev) => ({ ...prev, keywords: prev.keywords.filter((k) => k !== kw) }))
                      }
                      className="text-blue-500 hover:text-rose-600 font-bold"
                    >
                      ×
                    </button>
                  </span>
                ))}
                <input
                  type="text"
                  value={keywordInput}
                  onChange={(e) => setKeywordInput(e.target.value)}
                  onKeyDown={handleKeywordKeyDown}
                  onBlur={() => addKeywordChip(keywordInput)}
                  placeholder="พิมพ์ Keyword แล้วกด Enter (เช่น shodan, ค้นหา ip)"
                  className="flex-1 min-w-[180px] px-1.5 py-0.5 text-xs outline-none"
                />
              </div>

              <label className="block text-xs font-semibold text-slate-700">
                คำอธิบายสำหรับ AI (ไม่บังคับ — หากเว้นว่างระบบจะสร้างให้อัตโนมัติ)
                <textarea
                  rows={2}
                  value={form.description}
                  onChange={(e) => setForm((prev) => ({ ...prev, description: e.target.value }))}
                  placeholder="เช่น ตรวจสอบประวัติ IP และ Port ที่เปิดอยู่ผ่าน Shodan"
                  className="mt-1 w-full rounded-lg border border-slate-300 p-2 text-xs font-normal"
                />
              </label>
            </div>

            {/* Step 4: Instant Test (Dry Run + SOP Keyword Check) */}
            <div className="rounded-lg border border-indigo-200 bg-indigo-50/40 p-3 space-y-2">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="text-xs font-bold text-indigo-950">4. ทดสอบก่อนบันทึก (Dry Run & SOP Match)</span>
                <button
                  type="button"
                  disabled={testing}
                  onClick={() => void handleDryRunTest()}
                  className="rounded-lg bg-indigo-600 px-3 py-1 text-xs font-bold text-white hover:bg-indigo-700 disabled:opacity-50"
                >
                  {testing ? 'กำลังทดสอบ...' : '▶ ทดสอบ Tool (Dry Run)'}
                </button>
              </div>

              <div className="flex items-center gap-2">
                <input
                  type="text"
                  value={sopTestText}
                  onChange={(e) => setSopTestText(e.target.value)}
                  placeholder="ลองพิมพ์ประโยค SOP เพื่อเช็กการจับคู่ เช่น ตรวจสอบ IP ด้วย Shodan"
                  className="flex-1 rounded border border-indigo-200 bg-white px-2.5 py-1 text-xs"
                />
                {sopMatchResult && (
                  <span
                    className={`shrink-0 rounded px-2 py-1 text-[11px] font-bold ${
                      sopMatchResult.matched
                        ? 'bg-emerald-100 text-emerald-800'
                        : 'bg-amber-100 text-amber-800'
                    }`}
                  >
                    {sopMatchResult.matched
                      ? `✓ จับคู่ติด (${[...sopMatchResult.matchedKeywords, ...(sopMatchResult.nameMatch ? [form.name] : [])].join(', ')})`
                      : 'ยังไม่ตรง Keyword'}
                  </span>
                )}
              </div>

              {testResult && (
                <div className="rounded-lg bg-slate-900 p-2.5 font-mono text-[11px] text-slate-100 space-y-1">
                  <div className="flex items-center justify-between">
                    <span className="text-sky-300 font-bold">
                      {testResult.request.method} {testResult.request.url || '(no URL)'}
                    </span>
                    <span
                      className={`rounded px-1.5 py-0.5 text-[10px] font-bold uppercase ${
                        testResult.result.status === 'success'
                          ? 'bg-emerald-600 text-white'
                          : 'bg-rose-600 text-white'
                      }`}
                    >
                      {testResult.result.status}
                    </span>
                  </div>
                  <pre className="text-[10px] text-slate-300 overflow-x-auto">
                    {JSON.stringify(testResult.result.output, null, 2)}
                  </pre>
                </div>
              )}
            </div>

            {/* Advanced Settings (Collapsed by default) */}
            <details className="rounded-lg border border-slate-200 bg-slate-50/70 p-2.5">
              <summary className="cursor-pointer text-xs font-bold text-slate-700">
                ตั้งค่าขั้นสูง (Tool ID, CACAO Type, Default Mode, Mock Output)
              </summary>
              <div className="mt-2.5 space-y-2.5">
                <div className="grid grid-cols-3 gap-2">
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
                      placeholder="auto_slug"
                      className="mt-1 w-full rounded border border-slate-300 px-2 py-1 font-mono text-xs font-normal disabled:bg-slate-100"
                    />
                  </label>
                  <label className="text-xs font-semibold text-slate-700">
                    CACAO Type
                    <select
                      value={form.command_type}
                      onChange={(e) => setForm((prev) => ({ ...prev, command_type: e.target.value as ToolCommandType }))}
                      className="mt-1 w-full rounded border border-slate-300 px-2 py-1 text-xs font-normal"
                    >
                      <option value="http-api">http-api</option>
                      <option value="manual">manual</option>
                      <option value="bash">bash</option>
                      <option value="ssh">ssh</option>
                    </select>
                  </label>
                  <label className="text-xs font-semibold text-slate-700">
                    Default Payload
                    <select
                      value={form.payload_mode}
                      onChange={(e) => setForm((prev) => ({ ...prev, payload_mode: e.target.value as PayloadMode }))}
                      className="mt-1 w-full rounded border border-slate-300 px-2 py-1 text-xs font-normal"
                    >
                      <option value="dynamic">Dynamic</option>
                      <option value="fixed">Fixed</option>
                    </select>
                  </label>
                </div>

                <div className="space-y-1.5">
                  <div className="flex items-center justify-between">
                    <span className="text-xs font-semibold text-slate-700">Mock Sandbox Output (ผลจำลองตอนรัน)</span>
                    <div className="flex items-center gap-2">
                      {!form.mockJsonMode && (
                        <button
                          type="button"
                          onClick={() =>
                            setForm((prev) => ({
                              ...prev,
                              mockRows: [...prev.mockRows, { key: '', value: '' }],
                            }))
                          }
                          className="text-[11px] font-semibold text-blue-600 hover:underline"
                        >
                          + เพิ่ม Field
                        </button>
                      )}
                      <button
                        type="button"
                        onClick={() =>
                          setForm((prev) => {
                            if (!prev.mockJsonMode) {
                              const obj = mockRowsToObject(prev.mockRows) || {}
                              return {
                                ...prev,
                                mockJsonMode: true,
                                mockOutputText: Object.keys(obj).length > 0 ? JSON.stringify(obj, null, 2) : '',
                              }
                            }
                            try {
                              const parsed = prev.mockOutputText.trim()
                                ? (JSON.parse(prev.mockOutputText) as Record<string, unknown>)
                                : {}
                              return {
                                ...prev,
                                mockJsonMode: false,
                                mockRows: objectToMockRows(parsed),
                              }
                            } catch {
                              return { ...prev, mockJsonMode: false }
                            }
                          })
                        }
                        className="text-[11px] font-semibold text-slate-600 hover:underline"
                      >
                        {form.mockJsonMode ? 'สลับเป็น Key-Value' : 'สลับเป็น JSON'}
                      </button>
                    </div>
                  </div>

                  {form.mockJsonMode ? (
                    <textarea
                      rows={3}
                      value={form.mockOutputText}
                      onChange={(e) => setForm((prev) => ({ ...prev, mockOutputText: e.target.value }))}
                      placeholder='{"verdict": "Suspicious", "score": 85}'
                      className="w-full rounded border border-slate-300 bg-white p-2 font-mono text-[11px]"
                    />
                  ) : (
                    <div className="space-y-1">
                      {form.mockRows.map((row, idx) => (
                        <div key={idx} className="grid grid-cols-12 gap-1.5 items-center">
                          <input
                            type="text"
                            value={row.key}
                            onChange={(e) =>
                              setForm((prev) => ({
                                ...prev,
                                mockRows: prev.mockRows.map((r, i) => (i === idx ? { ...r, key: e.target.value } : r)),
                              }))
                            }
                            placeholder="key (เช่น verdict)"
                            className="col-span-5 rounded border border-slate-300 bg-white px-2 py-1 font-mono text-[11px]"
                          />
                          <input
                            type="text"
                            value={row.value}
                            onChange={(e) =>
                              setForm((prev) => ({
                                ...prev,
                                mockRows: prev.mockRows.map((r, i) => (i === idx ? { ...r, value: e.target.value } : r)),
                              }))
                            }
                            placeholder="value (เช่น Malicious)"
                            className="col-span-6 rounded border border-slate-300 bg-white px-2 py-1 font-mono text-[11px]"
                          />
                          <button
                            type="button"
                            onClick={() =>
                              setForm((prev) => ({
                                ...prev,
                                mockRows: prev.mockRows.filter((_, i) => i !== idx),
                              }))
                            }
                            className="col-span-1 text-center text-xs font-bold text-rose-500 hover:text-rose-700"
                          >
                            ✕
                          </button>
                        </div>
                      ))}
                      {form.mockRows.length === 0 && (
                        <div className="text-[11px] text-slate-400">
                          ยังไม่มี Mock Output — กด "+ เพิ่ม Field" เพื่อกำหนดผลลัพธ์จำลอง
                        </div>
                      )}
                    </div>
                  )}
                </div>
              </div>
            </details>

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
