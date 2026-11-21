import { useState } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { NodeImage, NodeLink, RunStatus } from './types'

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
  const links = Array.isArray(data.links) ? (data.links as NodeLink[]) : []
  const images = Array.isArray(data.images) ? (data.images as NodeImage[]) : []
  const [copiedIdx, setCopiedIdx] = useState<number | null>(null)

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
  const hasAssets = links.length > 0 || images.length > 0
  const widthClass = hasAssets ? 'w-[260px]' : 'w-[230px]'

  return (
    <div className={`${widthClass} border-2 px-3 py-2 shadow-sm ${boxStyle} ${ringStyle}`}>
      <Handle type="target" position={Position.Top} className="!bg-slate-500" />
      <div className="flex items-center justify-between text-[10px] font-bold opacity-75">
        <span>{badge}</span>
        {tool !== 'none' && <span className="uppercase">{tool}</span>}
      </div>
      <div className="mt-1 text-xs font-semibold leading-snug break-words">{label}</div>

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