import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { RunStatus, ToolDefinition, ToolSource } from './types'

export function FlowNodeCard({ data }: NodeProps) {
  const nodeType = String(data.type || 'action')
  const label = String(data.label || '')
  const tool = String(data.tool || 'none')
  const toolSource = data.tool_source as ToolSource | undefined
  const status = (data.status as RunStatus) || 'idle'
  const availableTools = (data.availableTools as ToolDefinition[]) || []
  const onRun = data.onRun as () => void
  const onChangeTool = data.onChangeTool as ((nextTool: string) => void) | undefined

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

  return (
    <div className={`w-[240px] border-2 px-3 py-2 shadow-sm ${boxStyle} ${ringStyle}`}>
      <Handle type="target" position={Position.Top} className="!bg-slate-500" />
      <div className="flex items-center justify-between gap-1 text-[10px] font-bold opacity-85">
        <span>{badge}</span>
        <div className="flex items-center gap-1 min-w-0">
          {tool !== 'none' && <span className="truncate uppercase">{toolDisplay}</span>}
          {sourceLabel && (
            <span className={`rounded px-1 py-0.2 text-[9px] font-bold ${sourceBadgeStyle}`}>
              {sourceLabel}
            </span>
          )}
        </div>
      </div>
      <div className="mt-1 text-xs font-semibold leading-snug">{label}</div>
      {canRun && (
        <div className="mt-2 flex items-center justify-between gap-1.5 no-print">
          {onChangeTool ? (
            <select
              value={tool}
              onClick={(e) => e.stopPropagation()}
              onChange={(e) => {
                e.stopPropagation()
                onChangeTool(e.target.value)
              }}
              aria-label="Select tool"
              className="max-w-[145px] truncate rounded border border-slate-300 bg-white/90 px-1.5 py-0.5 text-[10px] font-medium text-slate-800"
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
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              onRun()
            }}
            className="shrink-0 rounded bg-blue-600 px-2 py-0.5 text-[10px] font-semibold text-white hover:bg-blue-700"
          >
            {status === 'running' ? 'Running...' : '▶ Run'}
          </button>
        </div>
      )}
      <Handle type="source" position={Position.Bottom} className="!bg-slate-500" />
    </div>
  )
}