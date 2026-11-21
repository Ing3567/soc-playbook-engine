import type { ToolAuth, ToolAuthLocation, ToolAuthType } from './types'

interface ToolAuthSectionProps {
  auth: ToolAuth
  editingId: string | null
  isConfiguredInEnv: boolean
  onChange: (nextAuth: ToolAuth) => void
}

export function ToolAuthSection({
  auth,
  editingId,
  isConfiguredInEnv,
  onChange,
}: ToolAuthSectionProps) {
  return (
    <div className="rounded-lg border border-slate-200 bg-white p-2.5 space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-[11px] font-bold text-slate-700">
          🔑 Authentication (อ้างอิง .env / Session Key)
        </span>
        {editingId && auth.type !== 'none' && (
          <span
            className={`rounded px-1.5 py-0.5 text-[10px] font-bold ${
              isConfiguredInEnv ? 'bg-emerald-100 text-emerald-800' : 'bg-amber-100 text-amber-800'
            }`}
          >
            {isConfiguredInEnv ? `● พบใน .env (${auth.env_var})` : '○ ยังไม่พบใน .env'}
          </span>
        )}
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-12 gap-2">
        <label className="sm:col-span-3 text-[11px] font-semibold text-slate-600">
          Auth Type
          <select
            value={auth.type}
            onChange={(e) => {
              const nextType = e.target.value as ToolAuthType
              onChange({
                ...auth,
                type: nextType,
                in: nextType === 'api_key' ? auth.in || 'header' : 'header',
                key_name:
                  nextType === 'bearer' || nextType === 'basic'
                    ? 'Authorization'
                    : auth.key_name || 'X-API-Key',
                prefix: nextType === 'bearer' ? 'Bearer ' : nextType === 'basic' ? 'Basic ' : '',
              })
            }}
            className="mt-0.5 w-full rounded border border-slate-300 px-1.5 py-1 text-xs font-normal"
          >
            <option value="none">None (ไม่ต้อง Auth)</option>
            <option value="api_key">API Key</option>
            <option value="bearer">Bearer Token</option>
            <option value="basic">Basic Auth</option>
          </select>
        </label>

        {auth.type !== 'none' && (
          <>
            {auth.type === 'api_key' && (
              <label className="sm:col-span-2 text-[11px] font-semibold text-slate-600">
                ตำแหน่ง
                <select
                  value={auth.in || 'header'}
                  onChange={(e) => onChange({ ...auth, in: e.target.value as ToolAuthLocation })}
                  className="mt-0.5 w-full rounded border border-slate-300 px-1.5 py-1 text-xs font-normal"
                >
                  <option value="header">header</option>
                  <option value="query">query</option>
                </select>
              </label>
            )}
            <label
              className={`${auth.type === 'api_key' ? 'sm:col-span-3' : 'sm:col-span-4'} text-[11px] font-semibold text-slate-600`}
            >
              Key / Header Name
              <input
                type="text"
                value={auth.key_name || ''}
                onChange={(e) => onChange({ ...auth, key_name: e.target.value })}
                placeholder={auth.type === 'api_key' ? 'x-apikey' : 'Authorization'}
                className="mt-0.5 w-full rounded border border-slate-300 px-2 py-1 font-mono text-xs font-normal"
              />
            </label>
            <label
              className={`${auth.type === 'api_key' ? 'sm:col-span-4' : 'sm:col-span-5'} text-[11px] font-semibold text-slate-600`}
            >
              Env Var Name (.env)
              <input
                type="text"
                value={auth.env_var || ''}
                onChange={(e) => onChange({ ...auth, env_var: e.target.value.toUpperCase() })}
                placeholder="VT_API_KEY"
                className="mt-0.5 w-full rounded border border-slate-300 px-2 py-1 font-mono text-xs font-normal"
              />
            </label>
          </>
        )}
      </div>

      {auth.type !== 'none' && (
        <label className="flex items-center gap-1.5 text-[11px] text-slate-700 cursor-pointer">
          <input
            type="checkbox"
            checked={Boolean(auth.allow_runtime)}
            onChange={(e) => onChange({ ...auth, allow_runtime: e.target.checked })}
            className="accent-blue-600"
          />
          อนุญาตให้กรอก API Key ชั่วคราวตอนรัน (Session Key — ไม่บันทึกลงฐานข้อมูล)
        </label>
      )}
    </div>
  )
}
