import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { PayloadMode, RunStatus, ToolDefinition, ToolSource } from './types'
import type { PipelineNodeAvailability } from './pipeline'

export function FlowNodeCard({ data }: NodeProps) {
  const nodeType = String(data.type || 'action')
  const label = String(data.label || '')
  const tool = String(data.tool || 'none')
  const toolSource = data.tool_source as ToolSource | undefined
  const payloadMode = data.payload_mode as PayloadMode | undefined
  const paramModes = (data.param_modes as Record<string, PayloadMode> | undefined) || {}
  const inputPayload = (data.input_payload as Record<string, string> | undefined) || {}
  const nodeRuntimeInputs = (data.runtimeInputs as Record<string, string> | undefined) || {}
  const sharedAlertContext = (data.alertContext as Record<string, string> | undefined) || {}
  const status = (data.status as RunStatus) || 'idle'
  const pipelineAvailability = (data.pipelineAvailability as PipelineNodeAvailability | undefined) || 'ready'
  const lockedReason = (data.lockedReason as string | undefined) || ''
  const availableTools = Array.isArray(data.availableTools)
    ? (data.availableTools as ToolDefinition[])
    : []
  const onRun = data.onRun as (() => void) | undefined
  const onChangeTool = data.onChangeTool as ((nextTool: string) => void) | undefined
  const onEditPayload = data.onEditPayload as (() => void) | undefined
  const onCreateTool = data.onCreateTool as ((category: 'lookup' | 'action') => void) | undefined
  const onChangeRuntimeInput = data.onChangeRuntimeInput as ((key: string, value: string) => void) | undefined

  let boxStyle = 'bg-slate-100 border-slate-400 text-slate-800 rounded-xl'
  let badge = 'ACTION'

  if (nodeType === 'terminal') {
    boxStyle = 'bg-emerald-100 border-emerald-500 text-emerald-900 rounded-full'
    badge = 'TERMINAL'
  } else if (nodeType === 'lookup') {
    boxStyle = 'bg-sky-100 border-sky-500 text-sky-900 rounded-xl'
    badge = 'LOOKUP'
  } else if (nodeType === 'decision') {
    boxStyle = 'bg-amber-100 border-amber-500 text-amber-900 rounded-xl'
    badge = 'DECISION'
  } else if (nodeType === 'end') {
    boxStyle = 'bg-rose-100 border-rose-500 text-rose-900 rounded-xl'
    badge = 'END'
  }

  let ringStyle = ''
  if (status === 'running') ringStyle = 'ring-4 ring-blue-400 animate-pulse'
  if (status === 'success') ringStyle = 'ring-4 ring-emerald-500'
  if (status === 'failed') ringStyle = 'ring-4 ring-rose-500'

  const canRun = nodeType === 'lookup' || nodeType === 'action'
  const matchedTool = availableTools.find((t) => t.id === tool)
  const toolDisplay = matchedTool ? matchedTool.name : tool
  const manualOptionId = nodeType === 'lookup' ? 'manual_lookup' : 'manual_action'
  const selectableTools = availableTools.filter((t) => t.enabled && t.category === nodeType)

  let sourceLabel = ''
  let sourceBadgeStyle = ''
  if (toolSource === 'ai') {
    sourceLabel = 'AI'
    sourceBadgeStyle = 'bg-violet-600 text-white'
  } else if (toolSource === 'keyword') {
    sourceLabel = 'KW'
    sourceBadgeStyle = 'bg-sky-700 text-white'
  } else if (toolSource === 'manual') {
    sourceLabel = 'MANUAL'
    sourceBadgeStyle = 'bg-slate-600 text-white'
  }

  const effectiveMode: PayloadMode = payloadMode || matchedTool?.payload_mode || 'dynamic'
  const paramItems: Array<{
    key: string
    location: string
    mode: PayloadMode
    blueprintVal: string
    runtimeVal: string
    sharedHint: string
    required: boolean
    isPipeTemplate: boolean
  }> = []

  const resolveAlertHint = (expr: string): string => {
    if (!expr.includes('{{alert.')) return ''
    let anyFilled = false
    const resolved = expr.replace(/\{\{\s*alert\.([a-zA-Z0-9_]+)\s*\}\}/g, (_, k: string) => {
      const v = sharedAlertContext[k]?.trim() || ''
      if (v) anyFilled = true
      return v
    })
    return anyFilled ? resolved : ''
  }

  if (canRun) {
    const seen = new Set<string>()
    for (const p of matchedTool?.params || []) {
      seen.add(p.key)
      const pMode: PayloadMode = paramModes[p.key] || p.mode || effectiveMode
      const rawDef = p.value ?? p.default_value ?? ''
      const blueprintVal =
        inputPayload[p.key] ??
        (pMode === 'fixed' || (rawDef.includes('{{') && rawDef.includes('}}')) ? rawDef : '')
      const runtimeVal = nodeRuntimeInputs[p.key] ?? ''
      const isPipeTemplate = pMode === 'dynamic' && blueprintVal.includes('{{prev.')
      const sharedHint = pMode === 'dynamic' && !isPipeTemplate ? resolveAlertHint(blueprintVal) : ''
      paramItems.push({
        key: p.key,
        location: p.in.toUpperCase(),
        mode: pMode,
        blueprintVal,
        runtimeVal,
        sharedHint,
        required: Boolean(p.required),
        isPipeTemplate,
      })
    }
    for (const [k, v] of Object.entries(inputPayload)) {
      if (!seen.has(k)) {
        seen.add(k)
        const pMode: PayloadMode = paramModes[k] || effectiveMode
        const isPipeTemplate = pMode === 'dynamic' && v.includes('{{prev.')
        const sharedHint = pMode === 'dynamic' && !isPipeTemplate ? resolveAlertHint(v) : ''
        paramItems.push({
          key: k,
          location: 'PARAM',
          mode: pMode,
          blueprintVal: v,
          runtimeVal: nodeRuntimeInputs[k] ?? '',
          sharedHint,
          required: false,
          isPipeTemplate,
        })
      }
    }
  }

  const fixedOrPipeItems = paramItems.filter((item) => item.mode === 'fixed' || item.isPipeTemplate)
  const runtimeInputItems = paramItems.filter((item) => item.mode === 'dynamic' && !item.isPipeTemplate)

  const isSkipped = pipelineAvailability === 'skipped'
  const isLocked = pipelineAvailability === 'locked'

  return (
    <div
      className={`w-[256px] border-2 px-3 py-2 shadow-sm transition-opacity ${boxStyle} ${ringStyle} ${
        isSkipped ? 'opacity-50' : ''
      }`}
    >
      <Handle type="target" position={Position.Top} className="!bg-slate-500" />
      <div className="flex items-center justify-between gap-1 text-[10px] font-bold opacity-85">
        <div className="flex items-center gap-1">
          <span>{badge}</span>
          {isSkipped && (
            <span className="rounded bg-slate-500 px-1 py-0.5 text-[8px] leading-none font-bold text-white">
              SKIPPED
            </span>
          )}
        </div>
        <div className="flex items-center gap-1 min-w-0">
          {tool !== 'none' && <span className="truncate uppercase">{toolDisplay}</span>}
          {sourceLabel && (
            <span className={`shrink-0 rounded px-1 py-0.5 text-[9px] leading-none font-bold ${sourceBadgeStyle}`}>
              {sourceLabel}
            </span>
          )}
        </div>
      </div>

      <div className="mt-1 text-xs font-semibold leading-snug">{label}</div>

      {canRun && fixedOrPipeItems.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {fixedOrPipeItems.map((tag) => (
            <button
              key={`${tag.location}-${tag.key}`}
              type="button"
              onPointerDown={(e) => e.stopPropagation()}
              onMouseDown={(e) => e.stopPropagation()}
              onClick={(e) => {
                e.stopPropagation()
                onEditPayload?.()
              }}
              title={`คลิกเพื่อแก้ไข ${tag.location} (${tag.mode}): ${tag.key}=${tag.blueprintVal || '(empty)'}`}
              className={`nodrag nopan cursor-pointer inline-flex items-center gap-1 max-w-full rounded px-1.5 py-0.5 text-[9px] font-mono border hover:brightness-95 ${
                tag.isPipeTemplate
                  ? 'bg-violet-50 border-violet-300 text-violet-900'
                  : 'bg-amber-50 border-amber-300 text-amber-900'
              }`}
            >
              <span className="font-bold opacity-75">{tag.location}</span>
              <span className="truncate max-w-[140px]">
                {tag.key}={tag.blueprintVal || '?'}
              </span>
              <span
                className={`rounded px-1 text-[8px] font-bold uppercase ${
                  tag.isPipeTemplate ? 'bg-violet-200 text-violet-900' : 'bg-amber-200 text-amber-900'
                }`}
              >
                {tag.isPipeTemplate ? 'PIPE' : 'FIX'}
              </span>
            </button>
          ))}
        </div>
      )}

      {canRun && runtimeInputItems.length > 0 && (
        <div className="mt-1.5 space-y-1 no-print">
          {runtimeInputItems.map((item) => {
            const missingRequired = item.required && !item.runtimeVal.trim() && !item.sharedHint
            return (
              <div key={`rt-${item.key}`} className="flex items-center gap-1">
                <span
                  className="shrink-0 rounded bg-white/80 border border-slate-300 px-1 py-0.5 font-mono text-[9px] font-bold text-slate-700"
                  title={`${item.location} (กรอกตอนรัน ไม่เซฟลง SOP)`}
                >
                  {item.key}
                  {item.required && <span className="text-rose-600">*</span>}
                </span>
                <input
                  type="text"
                  value={item.runtimeVal}
                  disabled={isSkipped}
                  onPointerDown={(e) => e.stopPropagation()}
                  onMouseDown={(e) => e.stopPropagation()}
                  onClick={(e) => e.stopPropagation()}
                  onChange={(e) => {
                    e.stopPropagation()
                    onChangeRuntimeInput?.(item.key, e.target.value)
                  }}
                  placeholder={item.sharedHint ? `Session: ${item.sharedHint}` : `กรอก ${item.key}...`}
                  className={`nodrag nopan min-w-0 flex-1 rounded border px-1.5 py-0.5 font-mono text-[10px] outline-none ${
                    missingRequired
                      ? 'border-rose-400 bg-rose-50/80 text-rose-900 placeholder:text-rose-400'
                      : 'border-slate-300 bg-white/95 text-slate-900 placeholder:text-slate-400 focus:border-blue-500'
                  }`}
                />
              </div>
            )
          })}
        </div>
      )}

      {canRun && (
        <div className="mt-2 flex items-center justify-between gap-1 no-print">
          {onChangeTool ? (
            <select
              value={tool}
              onPointerDown={(e) => e.stopPropagation()}
              onMouseDown={(e) => e.stopPropagation()}
              onClick={(e) => e.stopPropagation()}
              onChange={(e) => {
                e.stopPropagation()
                const val = e.target.value
                if (val === '__create_new__') {
                  onCreateTool?.(nodeType === 'lookup' ? 'lookup' : 'action')
                  return
                }
                onChangeTool(val)
              }}
              aria-label="Select tool"
              className="nodrag nopan max-w-[112px] truncate rounded border border-slate-300 bg-white/90 px-1.5 py-0.5 text-[10px] font-medium text-slate-800 cursor-pointer"
            >
              {selectableTools.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name}
                </option>
              ))}
              {!selectableTools.some((t) => t.id === tool) && tool !== manualOptionId && tool !== 'none' && (
                <option value={tool}>{toolDisplay}</option>
              )}
              <option value={manualOptionId}>
                {nodeType === 'lookup' ? 'Manual Lookup' : 'Manual Action'}
              </option>
              {onCreateTool && <option value="__create_new__">+ สร้าง Tool ใหม่...</option>}
            </select>
          ) : (
            <span />
          )}

          <div className="flex items-center gap-1 shrink-0">
            {(nodeType === 'lookup' || paramItems.length > 0) && onEditPayload && (
              <button
                type="button"
                onPointerDown={(e) => e.stopPropagation()}
                onMouseDown={(e) => e.stopPropagation()}
                onClick={(e) => {
                  e.stopPropagation()
                  onEditPayload()
                }}
                title="แก้ไข Input Payload (Fixed / Dynamic)"
                className="nodrag nopan rounded border border-sky-400 bg-white px-1.5 py-0.5 text-[10px] font-semibold text-sky-800 hover:bg-sky-50"
              >
                ⚙ Payload
              </button>
            )}
            <button
              type="button"
              disabled={isLocked || isSkipped || status === 'running'}
              title={isLocked || isSkipped ? lockedReason : undefined}
              onPointerDown={(e) => e.stopPropagation()}
              onMouseDown={(e) => e.stopPropagation()}
              onClick={(e) => {
                e.stopPropagation()
                if (!isLocked && !isSkipped && status !== 'running') {
                  onRun?.()
                }
              }}
              className={`nodrag nopan shrink-0 rounded px-2 py-0.5 text-[10px] font-semibold ${
                isLocked || isSkipped
                  ? 'bg-slate-200 text-slate-500 cursor-not-allowed'
                  : status === 'failed'
                    ? 'bg-rose-600 text-white hover:bg-rose-700'
                    : status === 'success'
                      ? 'bg-emerald-600 text-white hover:bg-emerald-700'
                      : 'bg-blue-600 text-white hover:bg-blue-700'
              }`}
            >
              {isSkipped
                ? '🔒 ข้ามกิ่ง'
                : isLocked
                  ? '🔒 รอโหนดบน'
                  : status === 'running'
                    ? 'Running...'
                    : status === 'success'
                      ? '↻ Re-run'
                      : status === 'failed'
                        ? '↻ Retry'
                        : '▶ Run'}
            </button>
          </div>
        </div>
      )}
      <Handle type="source" position={Position.Bottom} className="!bg-slate-500" />
    </div>
  )
}