import { useEffect, useState } from 'react'
import type { AuthUser, RunEvent, SimulationRun } from './types'
import { getSimulationRun, listSimulationRuns } from './api'

interface RunDashboardModalProps {
  open: boolean
  user: AuthUser | null
  initialRunId?: string | null
  onClose: () => void
  onResumeRun: (run: SimulationRun, events: RunEvent[]) => void
}

export function RunDashboardModal({
  open,
  user,
  initialRunId,
  onClose,
  onResumeRun,
}: RunDashboardModalProps) {
  const [runs, setRuns] = useState<SimulationRun[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [selectedRun, setSelectedRun] = useState<SimulationRun | null>(null)
  const [selectedEvents, setSelectedEvents] = useState<RunEvent[]>([])
  const [detailLoading, setDetailLoading] = useState(false)

  useEffect(() => {
    if (!open) return
    setLoading(true)
    setError('')
    listSimulationRuns()
      .then((items) => {
        setRuns(items)
        const targetId = initialRunId || items[0]?.id
        if (targetId) {
          loadRunDetail(targetId)
        } else {
          setSelectedRun(null)
          setSelectedEvents([])
        }
      })
      .catch((e) => setError(e instanceof Error ? e.message : 'โหลดผลฝึกไม่สำเร็จ'))
      .finally(() => setLoading(false))
  }, [open, initialRunId])

  async function loadRunDetail(runId: string) {
    setDetailLoading(true)
    try {
      const data = await getSimulationRun(runId)
      setSelectedRun(data.run)
      setSelectedEvents(data.events)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'โหลดรายละเอียดรอบฝึกไม่สำเร็จ')
    } finally {
      setDetailLoading(false)
    }
  }

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/60 p-4">
      <div className="flex h-[82vh] w-full max-w-5xl flex-col rounded-xl border border-slate-200 bg-white shadow-2xl overflow-hidden">
        <div className="flex items-center justify-between border-b bg-slate-50 px-4 py-3">
          <div>
            <h2 className="text-sm font-bold text-slate-900">
              📊 {user?.role === 'instructor' ? 'Instructor Results Dashboard & Audit Log' : 'ประวัติการฝึกและคะแนนของคุณ'}
            </h2>
            <p className="text-[11px] text-slate-500">
              บันทึกผลจาก Server พร้อมคะแนน 100 คะแนนและ Event Timeline ทุกขั้นตอน
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

        {error && (
          <div className="mx-4 mt-2 rounded border border-rose-200 bg-rose-50 px-3 py-1.5 text-xs text-rose-700">
            {error}
          </div>
        )}

        <div className="flex flex-1 min-h-0 divide-x divide-slate-200">
          <div className="w-80 shrink-0 overflow-y-auto p-3 space-y-2 bg-slate-50/50">
            <div className="text-[11px] font-bold text-slate-600">รอบฝึกทั้งหมด ({runs.length})</div>
            {loading ? (
              <div className="text-xs text-slate-500">กำลังโหลด...</div>
            ) : runs.length === 0 ? (
              <div className="rounded border border-dashed border-slate-300 p-3 text-xs text-slate-500">
                ยังไม่มีประวัติรอบฝึก กดปุ่ม "เริ่มรอบฝึก (Server Run)" เพื่อบันทึกการฝึก
              </div>
            ) : (
              runs.map((r) => {
                const isSelected = selectedRun?.id === r.id
                return (
                  <button
                    key={r.id}
                    type="button"
                    onClick={() => loadRunDetail(r.id)}
                    className={`w-full rounded-lg border p-2.5 text-left transition ${
                      isSelected ? 'border-blue-500 bg-blue-50/70 shadow-sm' : 'border-slate-200 bg-white hover:bg-slate-50'
                    }`}
                  >
                    <div className="flex items-center justify-between gap-1">
                      <span className="truncate text-xs font-bold text-slate-800">
                        {r.scenario_title || r.playbook_title}
                      </span>
                      {r.score ? (
                        <span
                          className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-bold ${
                            r.score.total >= 80
                              ? 'bg-emerald-100 text-emerald-800'
                              : r.score.total >= 50
                                ? 'bg-amber-100 text-amber-800'
                                : 'bg-rose-100 text-rose-800'
                          }`}
                        >
                          {r.score.total}/100
                        </span>
                      ) : (
                        <span className="shrink-0 rounded bg-sky-100 px-1.5 py-0.5 text-[10px] font-bold text-sky-800">
                          In Progress
                        </span>
                      )}
                    </div>
                    <div className="mt-1 flex items-center justify-between text-[10px] text-slate-500">
                      <span>👤 {r.trainee_name}</span>
                      <span>{new Date(r.started_at).toLocaleString()}</span>
                    </div>
                  </button>
                )
              })
            )}
          </div>

          <div className="flex-1 overflow-y-auto p-4 space-y-4">
            {detailLoading ? (
              <div className="text-xs text-slate-500">กำลังโหลดรายละเอียด...</div>
            ) : !selectedRun ? (
              <div className="text-xs text-slate-500">เลือกรอบฝึกทางซ้ายเพื่อดูคะแนนและ Audit Timeline</div>
            ) : (
              <>
                <div className="flex flex-wrap items-start justify-between gap-2 border-b pb-3">
                  <div>
                    <div className="text-sm font-bold text-slate-900">
                      {selectedRun.scenario_title || selectedRun.playbook_title}
                    </div>
                    <div className="text-[11px] text-slate-500">
                      Playbook: {selectedRun.playbook_title} · Trainee: <strong>{selectedRun.trainee_name}</strong> · Run ID:{' '}
                      <code className="font-mono">{selectedRun.id}</code>
                    </div>
                  </div>
                  {selectedRun.status === 'in_progress' && (
                    <button
                      type="button"
                      onClick={() => {
                        onResumeRun(selectedRun, selectedEvents)
                        onClose()
                      }}
                      className="rounded-lg bg-indigo-600 px-3 py-1.5 text-xs font-bold text-white hover:bg-indigo-700"
                    >
                      ▶ ทำต่อจากจุดเดิม
                    </button>
                  )}
                </div>

                {selectedRun.score && (
                  <div className="rounded-xl border border-slate-200 bg-slate-50 p-3 space-y-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div>
                        <div className="text-xs font-bold text-slate-700">ผลคะแนนรวม (Simulation Score)</div>
                        <div className="text-[11px] text-slate-500">
                          MTTR: {selectedRun.score.duration_sec}s
                          {selectedRun.score.max_duration_sec
                            ? ` / เกณฑ์ ${selectedRun.score.max_duration_sec}s`
                            : ''}{' '}
                          · คำตอบที่ส่ง: <strong className="uppercase">{selectedRun.score.submitted_verdict}</strong>
                          {selectedRun.score.expected_verdict && (
                            <>
                              {' '}
                              · เฉลย: <strong className="uppercase">{selectedRun.score.expected_verdict}</strong>
                            </>
                          )}
                        </div>
                      </div>
                      <div
                        className={`rounded-xl px-3 py-1.5 text-lg font-extrabold ${
                          selectedRun.score.total >= 80
                            ? 'bg-emerald-600 text-white'
                            : selectedRun.score.total >= 50
                              ? 'bg-amber-500 text-white'
                              : 'bg-rose-600 text-white'
                        }`}
                      >
                        {selectedRun.score.total} / 100
                      </div>
                    </div>

                    <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-center text-[11px]">
                      <div className="rounded-lg border bg-white p-2">
                        <div className="text-slate-500">Verdict</div>
                        <div className="text-sm font-bold text-slate-900">{selectedRun.score.verdict_points} / 40</div>
                      </div>
                      <div className="rounded-lg border bg-white p-2">
                        <div className="text-slate-500">Coverage</div>
                        <div className="text-sm font-bold text-slate-900">{selectedRun.score.coverage_points} / 35</div>
                      </div>
                      <div className="rounded-lg border bg-white p-2">
                        <div className="text-slate-500">Decision Path</div>
                        <div className="text-sm font-bold text-slate-900">{selectedRun.score.path_points} / 15</div>
                      </div>
                      <div className="rounded-lg border bg-white p-2">
                        <div className="text-slate-500">Speed (MTTR)</div>
                        <div className="text-sm font-bold text-slate-900">{selectedRun.score.speed_points} / 10</div>
                      </div>
                    </div>

                    {selectedRun.score.feedback?.length > 0 && (
                      <ul className="space-y-1 text-xs text-slate-700">
                        {selectedRun.score.feedback.map((line, i) => (
                          <li key={i}>{line}</li>
                        ))}
                      </ul>
                    )}
                  </div>
                )}

                <div>
                  <h3 className="text-xs font-bold text-slate-700 mb-2">
                    📜 Audit Event Timeline ({selectedEvents.length} events)
                  </h3>
                  <div className="space-y-2">
                    {selectedEvents.map((ev) => (
                      <div key={ev.id} className="rounded-lg border border-slate-200 bg-white p-2.5 text-xs">
                        <div className="flex items-center justify-between gap-2">
                          <div className="font-semibold text-slate-800">
                            <span className="mr-1.5 rounded bg-slate-100 px-1.5 py-0.5 font-mono text-[10px]">
                              #{ev.seq}
                            </span>
                            <span className="uppercase text-[10px] font-bold text-indigo-700 mr-1.5">{ev.action}</span>
                            {ev.node_label && <span>{ev.node_label}</span>}
                          </div>
                          <span className="font-mono text-[10px] text-slate-400">
                            {new Date(ev.timestamp).toLocaleTimeString()}
                            {ev.duration_ms ? ` · ${ev.duration_ms}ms` : ''}
                          </span>
                        </div>
                        {ev.verdict && (
                          <div className="mt-1 text-[11px] text-slate-700">
                            Verdict: <strong className="uppercase">{ev.verdict}</strong>
                            {ev.notes ? ` — ${ev.notes}` : ''}
                          </div>
                        )}
                        {ev.output && (
                          <pre className="mt-1.5 max-h-32 overflow-y-auto rounded bg-slate-900 p-2 font-mono text-[10px] text-slate-200 whitespace-pre-wrap break-all">
                            {JSON.stringify(ev.output, null, 2)}
                          </pre>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
