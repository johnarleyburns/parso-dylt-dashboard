import { useState } from 'react'
import { Settings, Zap, RefreshCw, Lock } from 'lucide-react'

const API_BASE = import.meta.env.VITE_API_BASE ?? 'https://ctrl.oilfield.parso.guru'

interface AdminResult {
  ok: boolean
  message: string
}

async function adminFetch(path: string, method: string, token: string, body?: object): Promise<AdminResult> {
  try {
    const resp = await fetch(API_BASE + path, {
      method,
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
        'Accept': 'application/json',
      },
      body: body ? JSON.stringify(body) : undefined,
    })
    const data = await resp.json().catch(() => ({}))
    if (!resp.ok) {
      return { ok: false, message: (data as { error?: string }).error ?? `HTTP ${resp.status}` }
    }
    return { ok: true, message: JSON.stringify(data) }
  } catch (e) {
    return { ok: false, message: String(e) }
  }
}

interface AdminPanelProps {
  nodeNames: string[]
}

export default function AdminPanel({ nodeNames }: AdminPanelProps) {
  const [open, setOpen] = useState(false)
  const [token, setToken] = useState('')
  const [authed, setAuthed] = useState(false)
  const [interval, setInterval] = useState(300)
  const [result, setResult] = useState<AdminResult | null>(null)
  const [loading, setLoading] = useState(false)

  function showResult(r: AdminResult) {
    setResult(r)
    setTimeout(() => setResult(null), 4000)
  }

  async function run(action: () => Promise<AdminResult>) {
    setLoading(true)
    setResult(null)
    try {
      showResult(await action())
    } finally {
      setLoading(false)
    }
  }

  function handleTokenSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (token.trim()) setAuthed(true)
  }

  return (
    <div style={{ position: 'relative' }}>
      <button
        onClick={() => setOpen((o) => !o)}
        title="Admin controls"
        style={{
          background: 'transparent',
          border: '1px solid #1e293b',
          borderRadius: 4,
          color: open ? '#e2e8f0' : '#475569',
          cursor: 'pointer',
          padding: '0.2rem 0.4rem',
          display: 'flex',
          alignItems: 'center',
        }}
      >
        <Settings size={14} />
      </button>

      {open && (
        <div
          style={{
            position: 'absolute',
            top: '2rem',
            right: 0,
            zIndex: 100,
            background: '#111827',
            border: '1px solid #1e293b',
            borderRadius: 8,
            padding: '1rem',
            minWidth: 280,
            boxShadow: '0 8px 32px rgba(0,0,0,0.6)',
          }}
        >
          <div style={{ color: '#94a3b8', fontSize: '0.7rem', fontWeight: 700, letterSpacing: '0.08em', marginBottom: '0.75rem' }}>
            ADMIN CONTROLS
          </div>

          {/* Token gate */}
          {!authed ? (
            <form onSubmit={handleTokenSubmit} style={{ display: 'flex', gap: '0.5rem' }}>
              <input
                type="password"
                placeholder="Admin token"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                style={{
                  flex: 1,
                  background: '#0a0e1a',
                  border: '1px solid #1e293b',
                  borderRadius: 4,
                  color: '#e2e8f0',
                  padding: '0.3rem 0.5rem',
                  fontSize: '0.75rem',
                  fontFamily: 'inherit',
                }}
              />
              <button
                type="submit"
                style={{
                  background: '#1e3a5f',
                  border: 'none',
                  borderRadius: 4,
                  color: '#93c5fd',
                  cursor: 'pointer',
                  padding: '0.3rem 0.6rem',
                  fontSize: '0.75rem',
                  fontFamily: 'inherit',
                  display: 'flex',
                  alignItems: 'center',
                  gap: '0.25rem',
                }}
              >
                <Lock size={11} /> Unlock
              </button>
            </form>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>

              {/* Force scrape */}
              <div>
                <div style={{ color: '#64748b', fontSize: '0.65rem', marginBottom: '0.25rem' }}>SCRAPER</div>
                <button
                  disabled={loading}
                  onClick={() => run(() => adminFetch('/api/v1/admin/scrape-lock', 'DELETE', token))}
                  style={btnStyle('#14532d', '#86efac')}
                >
                  <Zap size={11} /> Force Scrape Now
                </button>
              </div>

              {/* Scrape interval */}
              <div>
                <div style={{ color: '#64748b', fontSize: '0.65rem', marginBottom: '0.25rem' }}>SCRAPE INTERVAL</div>
                <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
                  <input
                    type="number"
                    min={60}
                    max={3600}
                    step={60}
                    value={interval}
                    onChange={(e) => setInterval(Number(e.target.value))}
                    style={{
                      width: 70,
                      background: '#0a0e1a',
                      border: '1px solid #1e293b',
                      borderRadius: 4,
                      color: '#e2e8f0',
                      padding: '0.3rem 0.4rem',
                      fontSize: '0.75rem',
                      fontFamily: 'inherit',
                    }}
                  />
                  <span style={{ color: '#475569', fontSize: '0.7rem' }}>sec</span>
                  <button
                    disabled={loading}
                    onClick={() => run(() =>
                      adminFetch('/api/v1/admin/config/scrape-interval', 'PUT', token, { seconds: interval })
                    )}
                    style={btnStyle('#1e3a5f', '#93c5fd')}
                  >
                    <RefreshCw size={11} /> Save
                  </button>
                </div>
              </div>

              {/* Node bounce */}
              <div>
                <div style={{ color: '#64748b', fontSize: '0.65rem', marginBottom: '0.25rem' }}>RESTART SERVICES</div>
                <div style={{ display: 'flex', gap: '0.4rem' }}>
                  {nodeNames.map((name) => (
                    <button
                      key={name}
                      disabled={loading}
                      onClick={() => run(() =>
                        adminFetch(`/api/v1/admin/nodes/${name}/bounce`, 'POST', token)
                      )}
                      style={btnStyle('#3b0764', '#d8b4fe')}
                    >
                      {name.toUpperCase()}
                    </button>
                  ))}
                </div>
              </div>

              {/* Result banner */}
              {result && (
                <div
                  style={{
                    padding: '0.4rem 0.6rem',
                    borderRadius: 4,
                    background: result.ok ? '#14532d' : '#450a0a',
                    color: result.ok ? '#86efac' : '#fca5a5',
                    fontSize: '0.7rem',
                    wordBreak: 'break-all',
                  }}
                >
                  {result.message}
                </div>
              )}

              <button
                onClick={() => { setAuthed(false); setToken('') }}
                style={{ background: 'transparent', border: 'none', color: '#475569', cursor: 'pointer', fontSize: '0.65rem', textAlign: 'left', padding: 0 }}
              >
                Lock admin
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function btnStyle(bg: string, fg: string): React.CSSProperties {
  return {
    background: bg,
    border: 'none',
    borderRadius: 4,
    color: fg,
    cursor: 'pointer',
    padding: '0.3rem 0.6rem',
    fontSize: '0.7rem',
    fontFamily: 'inherit',
    display: 'flex',
    alignItems: 'center',
    gap: '0.25rem',
  }
}
