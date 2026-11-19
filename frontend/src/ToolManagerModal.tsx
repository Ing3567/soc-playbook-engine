import React, { useState, useMemo } from 'react'
import type { ToolCategory, ToolCommandType, ToolDefinition, ToolPayload } from './types'
import { createTool, updateTool, deleteTool } from './api'

interface ToolManagerModalProps {
  open: boolean
  tools: ToolDefinition[]
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
  endpoint: string
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
  endpoint: '',
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

export function ToolManagerModal({ open, tools, onClose, onChanged }: ToolManagerModalProps) {
  const [filterCat, setFilterCat] = useState<'all' | ToolCategory>('all')
  const [search, setSearch] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [idTouched, setIdTouched] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY_FORM)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')

  const filteredTools = useMemo(() => {
    const q = search.trim().toLowerCase()
    return tools.filter((t) => {
      if (filterCat !== 'all' && t.category !== filterCat) return false
      if (!q) return true
      return (
        t.name.toLowerCase().includes(q) ||
        t.id.toLowerCase().includes(q) ||
        t.description.toLowerCase().includes(q) ||
        t.keywords.some((k) => k.toLowerCase().includes(q))
      )
    })
  }, [tools, filterCat, search])

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
      keywordsText: (tool.keywords || []).join(', '),
      command_type: tool.command_type || 'http-api',
      endpoint: tool.endpoint || '',
      mockOutputText:
        tool.mock_output && Object.keys(tool.mock_output).length > 0
          ? JSON.stringify(tool.mock_output, null, 2)
          : '',
      enabled: tool.enabled,
    })
    setFormError('')
  }

  async function handleToggleEnabled(tool: ToolDefinition) {
    setFormError('')
    try {
      await updateTool(tool.id, {
        id: tool.id,
        name: tool.name,
        category: tool.category,
        description: tool.description,
        keywords: tool.keywords || [],
        command_type: tool.command_type,
        endpoint: tool.endpoint || '',
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

  async function handleSubmit(e: React.FormEvent) {
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

    const payload: ToolPayload = {
      id: (form.id || slugifyName(form.name)).trim(),
      name: form.name.trim(),
      category: form.category,
      description: form.description.trim(),
      keywords,
      command_type: form.command_type,
      endpoint: form.endpoint.trim() || undefined,
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
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 p-4">
      <div className="flex max-h-[90vh] w-full max-w-5xl flex-col overflow-hidden rounded-xl bg-white shadow-2xl border border-slate-200">
        <div className="flex items-center justify-between border-b border-slate-200 px-5 py-3 bg-slate-50">
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

        <div className="grid flex-1 grid-cols-1 md:grid-cols-12 overflow-hidden">
          <div className="md:col-span-7 flex flex-col border-b md:border-b-0 md:border-r border-slate-200 overflow-hidden">
            <div className="flex flex-wrap items-center gap-2 border-b border-slate-200 p-3 bg-white">
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

            <div className="flex-1 overflow-y-auto p-3 space-y-2 bg-slate-50/60">
              {filteredTools.map((t) => (
                <div
                  key={t.id}
                  className={`rounded-lg border p-3 bg-white transition ${
                    editingId === t.id ? 'border-blue-500 ring-2 ring-blue-100' : 'border-slate-200'
                  } ${!t.enabled ? 'opacity-60' : ''}`}
                >
                  <div className="flex items-start justify-between gap-2">
                    <div>
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
                      {t.keywords?.length > 0 && (
                        <div className="mt-1.5 flex flex-wrap gap-1">
                          {t.keywords.map((kw) => (
                            <span key={kw} className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-700">
                              {kw}
                            </span>
                          ))}
                        </div>
                      )}
                    </div>

                    <div className="flex flex-col items-end gap-1.5 shrink-0">
                      <label className="flex items-center gap-1 text-[11px] font-semibold text-slate-700 cursor-pointer">
                        <input
                          type="checkbox"
                          checked={t.enabled}
                          onChange={() => handleToggleEnabled(t)}
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
                            onClick={() => handleDelete(t)}
                            className="rounded border border-rose-200 px-2 py-0.5 text-[11px] font-semibold text-rose-600 hover:bg-rose-50"
                          >
                            ลบ
                          </button>
                        )}
                      </div>
                    </div>
                  </div>
                </div>
              ))}
              {filteredTools.length === 0 && (
                <div className="py-8 text-center text-xs text-slate-400">ไม่พบ Tool ที่ตรงกับการค้นหา</div>
              )}
            </div>
          </div>

          <form onSubmit={handleSubmit} className="md:col-span-5 flex flex-col overflow-y-auto p-4 gap-3 bg-white">
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
                Tool ID (Slug) *
                <input
                  type="text"
                  required
                  disabled={Boolean(editingId)}
                  value={form.id}
                  onChange={(e) => {
                    setIdTouched(true)
                    setForm((prev) => ({ ...prev, id: e.target.value }))
                  }}
                  placeholder="crowdstrike_edr"
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

            <div className="grid grid-cols-2 gap-2">
              <label className="text-xs font-semibold text-slate-700">
                CACAO Command Type
                <select
                  value={form.command_type}
                  onChange={(e) => setForm((prev) => ({ ...prev, command_type: e.target.value as ToolCommandType }))}
                  className="mt-1 w-full rounded border border-slate-300 px-2.5 py-1.5 text-xs font-normal"
                >
                  <option value="http-api">http-api</option>
                  <option value="manual">manual</option>
                  <option value="bash">bash</option>
                  <option value="ssh">ssh</option>
                </select>
              </label>

              <label className="text-xs font-semibold text-slate-700">
                Endpoint / Command (ไม่บังคับ)
                <input
                  type="text"
                  value={form.endpoint}
                  onChange={(e) => setForm((prev) => ({ ...prev, endpoint: e.target.value }))}
                  placeholder="https://api.example.com/v1"
                  className="mt-1 w-full rounded border border-slate-300 px-2.5 py-1.5 text-xs font-normal"
                />
              </label>
            </div>

            <label className="text-xs font-semibold text-slate-700">
              <div className="flex items-center justify-between">
                <span>Mock Sandbox Output (JSON, ไม่บังคับ)</span>
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
                rows={4}
                value={form.mockOutputText}
                onChange={(e) => setForm((prev) => ({ ...prev, mockOutputText: e.target.value }))}
                placeholder='{"is_malicious": true, "verdict": "Suspicious"}'
                className="mt-1 w-full rounded border border-slate-300 p-2 font-mono text-[11px] font-normal"
              />
            </label>

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
