import type {
  ExecutionMode,
  FlowNode,
  RunStatus,
  ScenarioItem,
  ScenarioVerdict,
  SimulationRun,
  ToolDefinition,
} from './types'
import type { PipelineState } from './pipeline'
import type { SopInputSummary } from './runtimeParams'

interface ExecutionPanelProps {
  nodes: FlowNode[]
  scenarios: ScenarioItem[]
  selectedScenarioId: string
  activeScenario: ScenarioItem | null
  onSelectScenario: (scenarioId: string) => void
  activeRun: SimulationRun | null
  submittingRun: boolean
  finalVerdict: ScenarioVerdict
  runSummary: string
  onChangeFinalVerdict: (v: ScenarioVerdict) => void
  onChangeRunSummary: (s: string) => void
  onStartServerRun: () => void
  onCompleteServerRun: () => void
  allowLiveExecution: boolean
  executionMode: ExecutionMode
  onChangeExecutionMode: (mode: ExecutionMode) => void
  sessionAuthTools: ToolDefinition[]
  sessionAuthKeys: Record<string, string>
  onChangeSessionAuthKey: (toolId: string, val: string) => void
  sopInputData: SopInputSummary
  alertContext: Record<string, string>
  onChangeAlertField: (key: string, val: string) => void
  statuses: Record<string, RunStatus>
  pipelineState: PipelineState
  runtimeInputs: Record<string, Record<string, string>>
  onChangeRuntimeInput: (nodeId: string, key: string, val: string) => void
  isRunningAll: boolean
  hasPendingReadyStep: boolean
  onRunAll: (resume: boolean) => void
  onResetExecution: () => void
}

export function ExecutionPanel({
  nodes,
  scenarios,
  selectedScenarioId,
  activeScenario,
  onSelectScenario,
  activeRun,
  submittingRun,
  finalVerdict,
  runSummary,
  onChangeFinalVerdict,
  onChangeRunSummary,
  onStartServerRun,
  onCompleteServerRun,
  allowLiveExecution,
  executionMode,
  onChangeExecutionMode,
  sessionAuthTools,
  sessionAuthKeys,
  onChangeSessionAuthKey,
  sopInputData,
  alertContext,
  onChangeAlertField,
  statuses,
  pipelineState,
  runtimeInputs,
  onChangeRuntimeInput,
  isRunningAll,
  hasPendingReadyStep,
  onRunAll,
  onResetExecution,
}: ExecutionPanelProps) {
  return (
    <div className="border-t pt-2">
      <details className="text-xs" open>
        <summary className="cursor-pointer font-bold text-slate-700">
          Run Session Inputs (ไม่บันทึกลง SOP)
        </summary>
        <p className="mt-1 text-[10px] text-slate-500">
          ค่าสำหรับเคสปัจจุบันเท่านั้น (เชื่อม 2 ทางกับบล็อกในกราฟ และไม่เซฟลง SOP)
        </p>

        {scenarios.length > 0 && (
          <div className="mt-2 rounded border border-indigo-200 bg-indigo-50/60 p-2 space-y-1.5">
            <div className="flex items-center justify-between gap-1">
              <span className="text-[10px] font-bold text-indigo-950">🎯 Training Scenario (โจทย์จำลอง)</span>
              {activeScenario && (
                <span
                  className={`rounded px-1.5 py-0.5 text-[9px] font-bold uppercase ${
                    activeScenario.difficulty === 'hard'
                      ? 'bg-rose-100 text-rose-800'
                      : activeScenario.difficulty === 'medium'
                        ? 'bg-amber-100 text-amber-800'
                        : 'bg-emerald-100 text-emerald-800'
                  }`}
                >
                  {activeScenario.difficulty}
                </span>
              )}
            </div>
            <select
              value={selectedScenarioId}
              onChange={(e) => onSelectScenario(e.target.value)}
              aria-label="Training Scenario"
              className="w-full rounded border border-indigo-300 bg-white px-2 py-1 text-[11px] text-slate-800"
            >
              <option value="">-- โหมดปกติ (ไม่ใช้ Scenario) --</option>
              {scenarios.map((s) => (
                <option key={s.id} value={s.id}>
                  [{s.difficulty}] {s.title}
                </option>
              ))}
            </select>
            {activeScenario && (
              <p className="text-[10px] leading-snug text-indigo-900">{activeScenario.description}</p>
            )}
          </div>
        )}

        {nodes.length > 0 && (
          <div className="mt-2 rounded border border-emerald-200 bg-emerald-50/60 p-2 space-y-1.5">
            {!activeRun || activeRun.status !== 'in_progress' ? (
              <button
                type="button"
                onClick={onStartServerRun}
                disabled={submittingRun}
                className="w-full rounded bg-emerald-600 py-1.5 text-[11px] font-bold text-white hover:bg-emerald-700 disabled:opacity-50"
              >
                {submittingRun ? 'กำลังเริ่มรอบฝึก...' : '🧪 เริ่มรอบฝึก (บันทึกผลและให้คะแนน)'}
              </button>
            ) : (
              <>
                <div className="flex items-center justify-between gap-1 text-[10px] font-bold text-emerald-950">
                  <span>● กำลังฝึกบน Server</span>
                  <span className="font-mono text-[9px] text-emerald-700">{activeRun.id.slice(0, 14)}…</span>
                </div>
                <div className="flex items-center gap-1">
                  <select
                    value={finalVerdict}
                    onChange={(e) => onChangeFinalVerdict(e.target.value as ScenarioVerdict)}
                    aria-label="Final Verdict"
                    className="rounded border border-emerald-300 bg-white px-1.5 py-1 text-[10px] font-semibold text-slate-800"
                  >
                    <option value="malicious">⚠ Malicious</option>
                    <option value="suspicious">△ Suspicious</option>
                    <option value="clean">✓ Clean / FP</option>
                  </select>
                  <button
                    type="button"
                    onClick={onCompleteServerRun}
                    disabled={submittingRun}
                    className="flex-1 rounded bg-emerald-700 py-1 text-[10px] font-bold text-white hover:bg-emerald-800 disabled:opacity-50"
                  >
                    {submittingRun ? 'กำลังคำนวณ...' : '🏁 ส่งคำตอบ'}
                  </button>
                </div>
                <input
                  type="text"
                  value={runSummary}
                  onChange={(e) => onChangeRunSummary(e.target.value)}
                  placeholder="สรุปเหตุผลสั้น ๆ (ไม่บังคับ)..."
                  className="w-full rounded border border-emerald-200 bg-white px-2 py-1 text-[10px]"
                />
              </>
            )}
          </div>
        )}

        {allowLiveExecution && (
          <div className="mt-2 flex items-center justify-between rounded border border-slate-200 bg-slate-50 px-2 py-1">
            <span className="text-[10px] font-bold text-slate-700">โหมดรัน:</span>
            <div className="flex rounded border border-slate-300 bg-white p-0.5 text-[10px] font-bold">
              <button
                type="button"
                onClick={() => onChangeExecutionMode('sandbox')}
                className={`rounded px-2 py-0.5 ${
                  executionMode === 'sandbox' ? 'bg-slate-800 text-white' : 'text-slate-600'
                }`}
              >
                Sandbox
              </button>
              <button
                type="button"
                onClick={() => onChangeExecutionMode('live')}
                className={`rounded px-2 py-0.5 ${
                  executionMode === 'live' ? 'bg-emerald-600 text-white' : 'text-slate-600'
                }`}
              >
                Live API
              </button>
            </div>
          </div>
        )}

        {sessionAuthTools.length > 0 && (
          <div className="mt-2 rounded border border-amber-200 bg-amber-50/70 p-2">
            <div className="text-[10px] font-bold text-amber-900 mb-1">
              🔑 Session API Keys (ชั่วคราว ไม่เซฟลง SOP)
            </div>
            <div className="space-y-1.5">
              {sessionAuthTools.map((t) => (
                <label key={`auth-${t.id}`} className="block text-[10px] font-semibold text-amber-950">
                  <div className="flex items-center justify-between gap-1">
                    <span className="truncate">{t.name}</span>
                    <span className="font-mono text-[9px] font-normal text-amber-700">
                      {t.auth?.env_var || t.auth?.key_name || t.auth?.type}
                    </span>
                  </div>
                  <input
                    type="password"
                    value={sessionAuthKeys[t.id] || ''}
                    placeholder={
                      t.auth?.type === 'basic'
                        ? 'user:password หรือ base64...'
                        : `กรอก API Key สำหรับ ${t.name}...`
                    }
                    onChange={(e) => onChangeSessionAuthKey(t.id, e.target.value)}
                    className="mt-0.5 w-full rounded border border-amber-300 bg-white px-1.5 py-1 font-mono text-[11px] font-normal"
                  />
                </label>
              ))}
            </div>
          </div>
        )}

        {nodes.length === 0 ? (
          <div className="mt-2 rounded border border-dashed border-slate-200 bg-slate-50 p-2 text-[11px] text-slate-500">
            สร้างหรือโหลด SOP เพื่อแสดงช่องกรอก Input ตามบล็อก
          </div>
        ) : sopInputData.blocks.length === 0 ? (
          <div className="mt-2 rounded border border-slate-200 bg-slate-50 p-2 text-[11px] text-slate-600">
            SOP นี้ไม่มีช่องที่ต้องกรอกเพิ่ม สามารถกดรันได้เลย
          </div>
        ) : (
          <div className="mt-2 space-y-2">
            {sopInputData.sharedAlertKeys.length > 0 && (
              <div className="rounded border border-indigo-200 bg-indigo-50/70 p-2">
                <div className="text-[10px] font-bold text-indigo-900 mb-1">ค่าใช้ร่วมกัน (Shared)</div>
                <div className="grid grid-cols-1 gap-1.5">
                  {sopInputData.sharedAlertKeys.map((item) => (
                    <label key={`shared-${item.key}`} className="text-[10px] font-semibold text-indigo-900">
                      <div className="flex items-center justify-between">
                        <span className="font-mono">alert.{item.key}</span>
                        <span className="text-[9px] font-normal text-indigo-700">
                          ใช้ร่วมกัน {item.blockCount} บล็อก
                        </span>
                      </div>
                      <input
                        type="text"
                        value={alertContext[item.key] || ''}
                        placeholder={`กรอก ${item.key} (ใช้ทุกบล็อกที่ผูก)...`}
                        onChange={(e) => onChangeAlertField(item.key, e.target.value)}
                        className="mt-0.5 w-full rounded border border-indigo-200 bg-white px-1.5 py-1 font-mono text-[11px] font-normal"
                      />
                    </label>
                  ))}
                </div>
              </div>
            )}

            {sopInputData.blocks.map((block) => {
              const nodeStatus = statuses[block.nodeId] || 'idle'
              const isSkipped = pipelineState.nodeStates[block.nodeId]?.availability === 'skipped'
              const statusDot =
                nodeStatus === 'running'
                  ? 'bg-blue-500 animate-pulse'
                  : nodeStatus === 'success'
                    ? 'bg-emerald-500'
                    : nodeStatus === 'failed'
                      ? 'bg-rose-500'
                      : 'bg-slate-300'

              return (
                <div
                  key={`block-${block.nodeId}`}
                  className={`rounded border p-2 transition-opacity ${
                    isSkipped
                      ? 'border-slate-200 bg-slate-50 opacity-50'
                      : 'border-slate-200 bg-slate-50/80'
                  }`}
                >
                  <div className="flex items-center justify-between gap-1.5 mb-1.5">
                    <div className="flex items-center gap-1 min-w-0">
                      <span
                        className={`shrink-0 rounded px-1 py-0.5 text-[8px] font-bold uppercase ${
                          block.nodeType === 'lookup'
                            ? 'bg-sky-200 text-sky-900'
                            : 'bg-slate-200 text-slate-800'
                        }`}
                      >
                        {block.nodeType}
                      </span>
                      <span className="truncate text-[11px] font-semibold text-slate-800" title={block.label}>
                        {block.stepIndex}. {block.label}
                      </span>
                    </div>
                    <div className="flex items-center gap-1 shrink-0">
                      <span className="text-[9px] font-medium text-slate-500">{block.toolName}</span>
                      <span className={`inline-block h-2 w-2 rounded-full ${statusDot}`} title={nodeStatus} />
                    </div>
                  </div>

                  <div className="space-y-1.5">
                    {block.fields.map((field) => {
                      const missingRequired =
                        field.required && !field.runtimeVal.trim() && !field.sharedHint
                      return (
                        <label key={`block-${block.nodeId}-${field.key}`} className="block text-[10px]">
                          <div className="flex items-center justify-between gap-1 font-mono font-semibold text-slate-700">
                            <span>
                              {field.isSecret ? '🔑 ' : ''}
                              {field.key}
                              {field.required && <span className="text-rose-600">*</span>}
                            </span>
                            <span className="text-[9px] font-normal text-slate-400">{field.location}</span>
                          </div>
                          <input
                            type={field.isSecret ? 'password' : 'text'}
                            value={field.runtimeVal}
                            disabled={isSkipped}
                            placeholder={
                              field.sharedHint ? `Shared: ${field.sharedHint}` : `กรอก ${field.key}...`
                            }
                            onChange={(e) => onChangeRuntimeInput(block.nodeId, field.key, e.target.value)}
                            className={`mt-0.5 w-full rounded border px-1.5 py-1 font-mono text-[11px] font-normal outline-none ${
                              missingRequired
                                ? 'border-rose-300 bg-rose-50/70 text-rose-900 placeholder:text-rose-400'
                                : 'border-slate-300 bg-white text-slate-900 placeholder:text-slate-400 focus:border-blue-500'
                            }`}
                          />
                        </label>
                      )
                    })}
                  </div>
                </div>
              )
            })}
          </div>
        )}

        <div className="mt-2 grid grid-cols-2 gap-1.5">
          <button
            type="button"
            onClick={() => onRunAll(hasPendingReadyStep)}
            disabled={nodes.length === 0 || isRunningAll}
            className="rounded bg-indigo-600 py-1.5 text-[11px] font-bold text-white hover:bg-indigo-700 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isRunningAll ? 'กำลังรัน...' : hasPendingReadyStep ? '▶ รันต่อที่เหลือ' : '▶ รันทั้งหมด'}
          </button>
          <button
            type="button"
            onClick={onResetExecution}
            disabled={
              nodes.length === 0 &&
              Object.keys(runtimeInputs).length === 0 &&
              Object.keys(alertContext).length === 0 &&
              Object.keys(sessionAuthKeys).length === 0
            }
            className="rounded border border-slate-300 bg-white py-1.5 text-[11px] font-bold text-slate-700 hover:bg-slate-100 disabled:opacity-50"
          >
            ↺ รีเซ็ต / ล้างค่า
          </button>
        </div>
      </details>
    </div>
  )
}
