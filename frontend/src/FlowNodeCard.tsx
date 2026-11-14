import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { RunStatus } from './types'

export function FlowNodeCard({ data }: NodeProps) {
  const nodeType = String(data.type || 'action')
  const label = String(data.label || '')
  const tool = String(data.tool || 'none')
  const status = (data.status as RunStatus) || 'idle'
  const onRun = data.onRun as () => void

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

  const canRun = nodeType === 'lookup' || nodeType === 'action'

  return (
    <div className={`w-[230px] border-2 px-3 py-2 shadow-sm ${boxStyle} ${ringStyle}`}>
      <Handle type="target" position={Position.Top} className="!bg-slate-500" />
      <div className="flex items-center justify-between text-[10px] font-bold opacity-75">
        <span>{badge}</span>
        {tool !== 'none' && <span className="uppercase">{tool}</span>}
      </div>
      <div className="mt-1 text-xs font-semibold leading-snug">{label}</div>
      {canRun && (
        <div className="mt-1.5 flex justify-end no-print">
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation()
              onRun()
            }}
            className="rounded bg-blue-600 px-2 py-0.5 text-[10px] font-semibold text-white hover:bg-blue-700"
          >
            {status === 'running' ? 'Running...' : '▶ Run'}
          </button>
        </div>
      )}
      <Handle type="source" position={Position.Bottom} className="!bg-slate-500" />
    </div>
  )
}