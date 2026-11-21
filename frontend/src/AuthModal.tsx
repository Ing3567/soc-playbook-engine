import { useState, type FormEvent } from 'react'
import type { AuthUser } from './types'
import { login, registerUser } from './api'

interface AuthModalProps {
  open: boolean
  required?: boolean
  onClose: () => void
  onAuthenticated: (user: AuthUser) => void
}

export function AuthModal({ open, required = false, onClose, onAuthenticated }: AuthModalProps) {
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  if (!open) return null

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      const res =
        mode === 'login'
          ? await login(username, password)
          : await registerUser({ username, password, display_name: displayName || undefined })
      onAuthenticated(res.user)
      setUsername('')
      setPassword('')
      setDisplayName('')
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'ดำเนินการไม่สำเร็จ')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/60 p-4">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-sm rounded-xl border border-slate-200 bg-white shadow-xl overflow-hidden"
      >
        <div className="flex items-center justify-between border-b bg-slate-50 px-4 py-3">
          <div>
            <h2 className="text-sm font-bold text-slate-900">
              {mode === 'login' ? '🔐 เข้าสู่ระบบ SOC Simulation' : '👤 สมัครบัญชี Trainee'}
            </h2>
            <p className="text-[11px] text-slate-500">
              {mode === 'login' ? 'ใช้ชื่อผู้ใช้และรหัสผ่านของคุณ' : 'บัญชีใหม่จะได้รับสิทธิ์ Trainee'}
            </p>
          </div>
          {!required && (
            <button
              type="button"
              onClick={onClose}
              className="rounded px-2 py-1 text-xs font-bold text-slate-500 hover:bg-slate-200"
            >
              ✕
            </button>
          )}
        </div>

        <div className="p-4 space-y-3 text-xs">
          <div className="flex rounded-lg border border-slate-200 bg-slate-100 p-0.5 text-[11px] font-bold">
            <button
              type="button"
              onClick={() => {
                setMode('login')
                setError('')
              }}
              className={`flex-1 rounded py-1 ${mode === 'login' ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-600'}`}
            >
              เข้าสู่ระบบ
            </button>
            <button
              type="button"
              onClick={() => {
                setMode('register')
                setError('')
              }}
              className={`flex-1 rounded py-1 ${mode === 'register' ? 'bg-white text-slate-900 shadow-sm' : 'text-slate-600'}`}
            >
              สมัครสมาชิก
            </button>
          </div>

          {error && (
            <div className="rounded border border-rose-200 bg-rose-50 px-2.5 py-1.5 text-[11px] text-rose-700">
              {error}
            </div>
          )}

          <label className="block font-semibold text-slate-700">
            Username
            <input
              type="text"
              required
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="instructor หรือ trainee"
              className="mt-1 w-full rounded-lg border border-slate-300 px-2.5 py-1.5 text-xs font-normal"
            />
          </label>

          {mode === 'register' && (
            <label className="block font-semibold text-slate-700">
              ชื่อที่แสดง (ไม่บังคับ)
              <input
                type="text"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                placeholder="เช่น สมชาย SOC Tier 1"
                className="mt-1 w-full rounded-lg border border-slate-300 px-2.5 py-1.5 text-xs font-normal"
              />
            </label>
          )}

          <label className="block font-semibold text-slate-700">
            Password
            <input
              type="password"
              required
              minLength={6}
              autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="อย่างน้อย 6 ตัวอักษร"
              className="mt-1 w-full rounded-lg border border-slate-300 px-2.5 py-1.5 text-xs font-normal"
            />
          </label>
        </div>

        <div className="flex items-center justify-end gap-2 border-t bg-slate-50 px-4 py-2.5">
          {!required && (
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-xs font-semibold text-slate-700 hover:bg-slate-100"
            >
              ยกเลิก
            </button>
          )}
          <button
            type="submit"
            disabled={submitting}
            className="rounded-lg bg-blue-600 px-4 py-1.5 text-xs font-bold text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {submitting ? 'กำลังตรวจสอบ...' : mode === 'login' ? 'เข้าสู่ระบบ' : 'สร้างบัญชี'}
          </button>
        </div>
      </form>
    </div>
  )
}
