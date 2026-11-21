import { useState } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { NodeImage, NodeLink, RunStatus, ToolInputField } from './types'

function isSafeHttpUrl(raw: string): boolean {
  return /^https?:\/\//i.test(raw.trim())
}

function isSafeImageUrl(raw: string): boolean {
  const trimmed = raw.trim()
  return trimmed.startsWith('/api/images/') || /^https?:\/\//i.test(trimmed)
}

export function FlowNodeCard({ data }: NodeProps) {
  const nodeType = String(data.type || 'action')
  const label = String(data.label || '')
  const tool = String(data.tool || 'none')
  const status = (data.status as RunStatus) || 'idle'
  const onRun = data.onRun as () => void
  const onPreviewImage = data.onPreviewImage as ((img: NodeImage) => void) | undefined
  const onInputChange = data.onInputChange as ((key: string, value: string) => void) | undefined
  const links = Array.isArray(data.links) ? (data.links as NodeLink[]) : []
  const images = Array.isArray(data.images) ? (data.images as NodeImage[]) : []
  const inputFields = Array.isArray(data.inputFields) ? (data.inputFields as ToolInputField[]) : []
  const inputValues = (data.inputValues as Record<string, string>) || {}
  const [copiedIdx, setCopiedIdx] = useState<number | null>(null)
  const [revealedKeys, setRevealedKeys] = useState<Record<string, boolean>>({})

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
  const hasRichContent = links.length > 0 || images.length > 0 || (canRun && inputFields.length > 0)
  const widthClass = hasRichContent ? 'w-[260px]' : 'w-[230px]'

  return (
    <div className={`${widthClass} border-2 px-3 py-2 shadow-sm ${boxStyle} ${ringStyle}`}>
      <Handle type="target" position={Position.Top} className="!bg-slate-500" />
      <div className="flex items-center justify-between text-[10px] font-bold opacity-75">
        <span>{badge}</span>
        {tool !== 'none' && <span className="uppercase">{tool}</span>}
      </div>
      <div className="mt-1 text-xs font-semibold leading-snug break-words">{label}</div>

      {canRun && inputFields.length > 0 && (
        <div
          className="nodrag nopan mt-2 space-y-1.5 border-t border-black/10 pt-1.5"
          onMouseDown={(e) => e.stopPropagation()}
          onClick={(e) => e.stopPropagation()}
        >
          {inputFields.map((field) => {
            const isPassword = field.type === 'password'
            const isRevealed = Boolean(revealedKeys[field.key])
            const currentVal = inputValues[field.key] ?? ''
            return (
              <div key={field.key} className="space-y-0.5">
                <label className="block text-[9px] font-bold uppercase tracking-wide opacity-75">
                  {isPassword ? '🔑 ' : '🔎 '}
                  {field.label}
                </label>
                <div className="flex items-center gap-1">
                  <input
                    type={isPassword && !isRevealed ? 'password' : 'text'}
                    value={currentVal}
                    onChange={(e) => onInputChange?.(field.key, e.target.value)}
                    onKeyDown={(e) => {
                      e.stopPropagation()
                      if (e.key === 'Enter') {
                        e.preventDefault()
                        onRun()
                      }
                    }}
                    onMouseDown={(e) => e.stopPropagation()}
                    onClick={(e) => e.stopPropagation()}
                    autoComplete="off"
                    spellCheck={false}
                    placeholder={field.placeholder || ''}
                    className="nodrag w-full rounded border border-black/20 bg-white/95 px-1.5 py-1 font-mono text-[10px] text-slate-900 placeholder:text-slate-400 focus:border-blue-500 focus:outline-none"
                  />
                  {isPassword && (
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation()
                        setRevealedKeys((prev) => ({ ...prev, [field.key]: !prev[field.key] }))
                      }}
                      className="no-print shrink-0 rounded border border-black/15 bg-white/90 px-1.5 py-1 text-[9px] font-semibold text-slate-600 hover:bg-slate-100"
                      title={isRevealed ? 'ซ่อนรหัส' : 'แสดงรหัส'}
                    >
                      {isRevealed ? 'ซ่อน' : 'ดู'}
                    </button>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      )}

      {links.length > 0 && (
        <div className="nodrag nopan mt-2 space-y-1 border-t border-black/10 pt-1.5">
          {links.map((link, idx) => {
            if (!isSafeHttpUrl(link.url)) return null
            const isSplunk = link.source === 'splunk' || Boolean(link.splunk_query)
            return (
              <div key={`${link.url}-${idx}`} className="rounded bg-white/80 p-1 text-[10px] shadow-xs border border-black/10">
                <a
                  href={link.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={(e) => e.stopPropagation()}
                  className="flex items-center justify-between gap-1 font-semibold text-blue-700 hover:text-blue-900 hover:underline"
                  title={link.url}
                >
                  <span className="truncate">
                    {isSplunk ? '🔎 ' : '🔗 '}
                    {link.label || link.url}
                  </span>
                  <span className="shrink-0 text-[9px] opacity-75">↗</span>
                </a>
                {link.splunk_query && (
                  <div className="mt-1 flex items-center justify-between gap-1 rounded bg-slate-900 px-1.5 py-0.5 font-mono text-[9px] text-emerald-300">
                    <span className="truncate" title={link.splunk_query}>
                      {link.splunk_query}
                    </span>
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation()
                        navigator.clipboard?.writeText(link.splunk_query || '')
                        setCopiedIdx(idx)
                        setTimeout(() => setCopiedIdx(null), 1500)
                      }}
                      className="shrink-0 rounded bg-slate-700 px-1 py-0.5 text-[8px] text-slate-200 hover:bg-slate-600 no-print"
                    >
                      {copiedIdx === idx ? 'Copied' : 'Copy'}
                    </button>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {images.length > 0 && (
        <div
          className={`nodrag nopan mt-2 grid gap-1.5 border-t border-black/10 pt-1.5 ${
            images.length > 1 ? 'grid-cols-2' : 'grid-cols-1'
          }`}
        >
          {images.map((img, idx) => {
            if (!isSafeImageUrl(img.url)) return null
            const caption = img.caption || img.filename || `Image ${idx + 1}`
            return (
              <button
                key={`${img.id || img.url}-${idx}`}
                type="button"
                onClick={(e) => {
                  e.stopPropagation()
                  onPreviewImage?.(img)
                }}
                className="group relative flex flex-col overflow-hidden rounded border border-black/15 bg-white/90 text-left shadow-xs hover:border-blue-500"
                title={`คลิกเพื่อขยายดู: ${caption}`}
              >
                <img
                  src={img.url}
                  alt={caption}
                  loading="lazy"
                  className="h-20 w-full object-cover transition-transform group-hover:scale-105"
                />
                <span className="w-full truncate px-1 py-0.5 text-[9px] font-medium text-slate-700 bg-white/95">
                  🖼️ {caption}
                </span>
              </button>
            )
          })}
        </div>
      )}

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