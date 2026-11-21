import { useEffect, useState, type FormEvent } from 'react'
import type { ConditionOperator, EdgeCondition, FlowEdge } from './types'

interface EdgeConditionModalProps {
  edge: FlowEdge | null
  sourceLabel?: string
  targetLabel?: string
  onClose: () => void
  onSave: (edgeId: string, label: string, condition?: EdgeCondition) => void
}

const OPERATORS: Array<{ value: ConditionOperator; label: string }> = [
  { value: '>', label: '> (มากกว่า)' },
  { value: '>=', label: '>= (มากกว่าหรือเท่ากับ)' },
  { value: '<', label: '< (น้อยกว่า)' },
  { value: '<=', label: '<= (น้อยกว่าหรือเท่ากับ)' },
  { value: '==', label: '== (เท่ากับ)' },
  { value: '!=', label: '!= (ไม่เท่ากับ)' },
  { value: 'contains', label: 'contains (มีคำว่า)' },
  { value: 'is_true', label: 'is_true (เป็นจริง)' },
  { value: 'is_false', label: 'is_false (เป็นเท็จ)' },
]

const QUICK_FIELDS = [
  'prev.malicious_count',
  'prev.confidence_score',
  'prev.verdict',
  'prev.is_malicious',
  'alert.severity',
]

export function EdgeConditionModal({
  edge,
  sourceLabel,
  targetLabel,
  onClose,
  onSave,
}: EdgeConditionModalProps) {
  const [label, setLabel] = useState('')
  const [useCondition, setUseCondition] = useState(false)
  const [field, setField] = useState('prev.malicious_count')
  const [op, setOp] = useState<ConditionOperator>('>')
  const [rawValue, setRawValue] = useState('0')

  useEffect(() => {
    if (!edge) return
    setLabel(edge.label || '')
    if (edge.condition && edge.condition.field) {
      setUseCondition(true)
      setField(edge.condition.field)
      setOp(edge.condition.op)
      setRawValue(edge.condition.value !== undefined ? String(edge.condition.value) : '')
    } else {
      setUseCondition(false)
      setField('prev.malicious_count')
      setOp('>')
      setRawValue('0')
    }
  }, [edge])

  if (!edge) return null

  const needsValue = op !== 'is_true' && op !== 'is_false'

  function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!edge) return
    let nextCondition: EdgeCondition | undefined
    if (useCondition && field.trim()) {
      let parsedValue: string | number | boolean | undefined
      if (needsValue) {
        const trimmed = rawValue.trim()
        if (trimmed === 'true') parsedValue = true
        else if (trimmed === 'false') parsedValue = false
        else if (trimmed !== '' && !Number.isNaN(Number(trimmed))) parsedValue = Number(trimmed)
        else parsedValue = trimmed
      }
      nextCondition = {
        field: field.trim(),
        op,
        ...(needsValue ? { value: parsedValue } : {}),
      }
    }
    onSave(edge.id, label.trim(), nextCondition)
    onClose()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 p-4">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-md rounded-xl border border-slate-200 bg-white shadow-xl overflow-hidden"
      >
        <div className="flex items-center justify-between border-b bg-amber-50 px-4 py-3">
          <div>
            <h2 className="text-sm font-bold text-amber-950">⚙ เงื่อนไขกิ่งตัดสินใจ (Decision Edge)</h2>
            <p className="text-[11px] text-amber-800 truncate max-w-[340px]">
              {sourceLabel || edge.source} → {targetLabel || edge.target}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded px-2 py-1 text-xs font-bold text-slate-500 hover:bg-slate-200"
          >
            ✕
          </button>
        </div>

        <div className="p-4 space-y-3 text-xs">
          <label className="block font-semibold text-slate-700">
            ชื่อกิ่ง (Label)
            <input
              type="text"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="เช่น ใช่, ไม่ใช่, เสี่ยงสูง"
              className="mt-1 w-full rounded-lg border border-slate-300 px-2.5 py-1.5 text-xs font-normal"
            />
          </label>

          <label className="flex items-center gap-2 font-semibold text-slate-700 cursor-pointer">
            <input
              type="checkbox"
              checked={useCondition}
              onChange={(e) => setUseCondition(e.target.checked)}
              className="accent-amber-600"
            />
            กำหนดเงื่อนไขแบบมีโครงสร้าง (ถ้าไม่ติ๊กจะเป็นกิ่ง Else / Fallback)
          </label>

          {useCondition && (
            <div className="rounded-lg border border-amber-200 bg-amber-50/50 p-3 space-y-2.5">
              <div>
                <label className="block font-semibold text-slate-700">
                  Field (จากผลลัพธ์ก่อนหน้าหรือ Alert)
                  <input
                    type="text"
                    value={field}
                    onChange={(e) => setField(e.target.value)}
                    placeholder="prev.malicious_count"
                    className="mt-1 w-full rounded border border-slate-300 bg-white px-2 py-1 font-mono text-xs font-normal"
                  />
                </label>
                <div className="mt-1.5 flex flex-wrap gap-1">
                  {QUICK_FIELDS.map((qf) => (
                    <button
                      key={qf}
                      type="button"
                      onClick={() => setField(qf)}
                      className="rounded border border-amber-300 bg-white px-1.5 py-0.5 font-mono text-[10px] text-amber-900 hover:bg-amber-100"
                    >
                      {qf}
                    </button>
                  ))}
                </div>
              </div>

              <div className="grid grid-cols-2 gap-2">
                <label className="block font-semibold text-slate-700">
                  Operator
                  <select
                    value={op}
                    onChange={(e) => setOp(e.target.value as ConditionOperator)}
                    className="mt-1 w-full rounded border border-slate-300 bg-white px-2 py-1 text-xs font-normal"
                  >
                    {OPERATORS.map((item) => (
                      <option key={item.value} value={item.value}>
                        {item.label}
                      </option>
                    ))}
                  </select>
                </label>

                {needsValue && (
                  <label className="block font-semibold text-slate-700">
                    ค่าเปรียบเทียบ (Value)
                    <input
                      type="text"
                      value={rawValue}
                      onChange={(e) => setRawValue(e.target.value)}
                      placeholder="เช่น 0, 50, high, phishing"
                      className="mt-1 w-full rounded border border-slate-300 bg-white px-2 py-1 font-mono text-xs font-normal"
                    />
                  </label>
                )}
              </div>
            </div>
          )}
        </div>

        <div className="flex items-center justify-end gap-2 border-t bg-slate-50 px-4 py-2.5">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-100"
          >
            ยกเลิก
          </button>
          <button
            type="submit"
            className="rounded-lg bg-amber-600 px-3 py-1.5 text-xs font-bold text-white hover:bg-amber-700"
          >
            บันทึกเงื่อนไข
          </button>
        </div>
      </form>
    </div>
  )
}
