import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { PayloadMode, RunStatus, ToolDefinition, ToolSource } from './types'

export function FlowNodeCard({ data }: NodeProps) {
  const nodeType = String(data.type || 'action')
  const label = String(data.label || '')
  const tool = String(data.tool || 'none')
  const toolSource = data.tool_source as ToolSource | undefined
  const payloadMode = data.payload_mode as PayloadMode | undefined
  const inputPayload = (data.input_payload as Record<string, string> | undefined) || {}
  const status = (data.status as RunStatus) || 'idle'
  const availableTools = Array.isArray(data.availableTools)
    ? (data.availableTools as ToolDefinition[])
    : []
  const onRun = data.onRun as (() => void) | undefined
  const onChangeTool = data.onChangeTool as ((nextTool: string) => void) | undefined
  const onEditPayload = data.onEditPayload as (() => void) | undefined

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
  const payloadTags: Array<{
    key: string
    location: string
    mode: PayloadMode
    value: string
    missing: boolean
  }> = []

  if (nodeType === 'lookup') {
    const seen = new Set<string>()
    for (const p of matchedTool?.params || []) {
      seen.add(p.key)
      const val = inputPayload[p.key] ?? p.value ?? p.default_value ?? ''
      const pMode: PayloadMode = payloadMode || p.mode || effectiveMode
      payloadTags.push({
        key: p.key,
        location: p.in.toUpperCase(),
        mode: pMode,
        value: val,
        missing: Boolean(p.required && !val.trim()),
      })
    }
    for (const [k, v] of Object.entries(inputPayload)) {
      if (!seen.has(k)) {
        payloadTags.push({
          key: k,
          location: 'PARAM',
          mode: effectiveMode,
          value: v,
          missing: false,
        })
      }
    }
  }

  return (
    <div className={`w-[256px] border-2 px-3 py-2 shadow-sm ${boxStyle} ${ringStyle}`}>
      <Handle type="target" position={Position.Top} className="!bg-slate-500" />
      <div className="flex items-center justify-between gap-1 text-[10px] font-bold opacity-85">
        <span>{badge}</span>
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

      {nodeType === 'lookup' && payloadTags.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {payloadTags.map((tag) => (
            <span
              key={`${tag.location}-${tag.key}`}
              title={`${tag.location} (${tag.mode}): ${tag.key}=${tag.value || '(empty)'}`}
              className={`inline-flex items-center gap-1 max-w-full rounded px-1.5 py-0.5 text-[9px] font-mono border ${
                tag.missing
                  ? 'bg-rose-50 border-rose-400 text-rose-800'
                  : tag.mode === 'fixed'
                    ? 'bg-amber-50 border-amber-300 text-amber-900'
                    : 'bg-white/90 border-sky-300 text-sky-900'
              }`}
            >
              <span className="font-bold opacity-75">{tag.location}</span>
              <span className="truncate max-w-[140px]">
                {tag.key}={tag.value || '?'}
              </span>
              <span
                className={`rounded px-1 text-[8px] font-bold uppercase ${
                  tag.mode === 'fixed' ? 'bg-amber-200 text-amber-900' : 'bg-violet-100 text-violet-800'
                }`}
              >
                {tag.mode === 'fixed' ? 'FIX' : 'DYN'}
              </span>
            </span>
          ))}
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
                onChangeTool(e.target.value)
              }}
              aria-label="Select tool"
              className="nodrag nopan max-w-[118px] truncate rounded border border-slate-300 bg-white/90 px-1.5 py-0.5 text-[10px] font-medium text-slate-800 cursor-pointer"
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
            </select>
          ) : (
            <span />
          )}

          <div className="flex items-center gap-1 shrink-0">
            {nodeType === 'lookup' && onEditPayload && (
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
              onPointerDown={(e) => e.stopPropagation()}
              onMouseDown={(e) => e.stopPropagation()}
              onClick={(e) => {
                e.stopPropagation()
                onRun?.()
              }}
              className="nodrag nopan shrink-0 rounded bg-blue-600 px-2 py-0.5 text-[10px] font-semibold text-white hover:bg-blue-700"
            >
              {status === 'running' ? 'Running...' : '▶ Run'}
            </button>
          </div>
        </div>
      )}
      <Handle type="source" position={Position.Bottom} className="!bg-slate-500" />
    </div>
  )
}