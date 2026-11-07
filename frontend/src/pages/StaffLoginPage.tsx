import { useEffect, useState, type FormEvent } from 'react'
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { ArrowRight, ShieldCheck, Store } from 'lucide-react'
import { Button, Card, Input, useToast } from '../components/ui'
import { VeylixLogo } from '../components/brand'
import { useAuthStore } from '../stores/authStore'
import { roleHome } from '../components/auth/RequireAuth'
import './auth.css'

/**
 * The STAFF door (D-041): administrators and the Super Admin.
 *
 * Clients sign in at `/login`. The server enforces the split — this page only
 * sends `portal: "staff"`, and an account that is not staff is refused with
 * `wrong_portal`, which is shown as "use the client login", never as a bad
 * password.
 *
 * The role chips are a convenience for the person (they land on the right home
 * screen); they grant nothing. Authorization is decided on every API call.
 */
export function StaffLoginPage() {
  const { user, status, signIn, pending, error: authError, clearError, bootstrapRequired } =
    useAuthStore()
  const toast = useToast()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [identifier, setIdentifier] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [hint, setHint] = useState<'admin' | 'super'>(
    params.get('role') === 'admin' ? 'admin' : 'super',
  )

  // A signed-in client must not sit on this page.
  useEffect(() => {
    if (status === 'authed' && user && user.role === 'client') {
      navigate('/client', { replace: true })
    }
  }, [status, user, navigate])

  if (status === 'authed' && user && user.role !== 'client') {
    return <Navigate to={roleHome(user.role)} replace />
  }
  if (status === 'anon' && bootstrapRequired) return <Navigate to="/setup" replace />

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!identifier.trim() || !password) {
      setError('Username (or e-mail) and password are required.')
      return
    }
    setError('')
    clearError()
    const ok = await signIn(identifier.trim(), password, 'staff')
    if (!ok) return // the store surfaces the reason (wrong door / unverified / bad password)
    toast('success', 'Signed in', 'Staff portal')
    const now = useAuthStore.getState().user
    if (now) navigate(roleHome(now.role), { replace: true })
  }

  return (
    <div className="auth" data-testid="staff-login">
      <aside className="auth__brand">
        <div>
          <VeylixLogo size={40} showText />
          <h1 className="auth__brand-title">Staff sign-in</h1>
          <p className="auth__brand-sub ui-muted">
            Administration and the Super Admin console. Customer accounts sign in from
            the client login page.
          </p>
        </div>
        <ul className="auth__points">
          <li className="auth__point">
            <span className="auth__point-dot" aria-hidden="true" />
            <div>
              <strong>Administrators are created by a Super Admin</strong>
              <span className="ui-muted">
                There is no self-service admin sign-up: an account exists only because an
                operator made it.
              </span>
            </div>
          </li>
          <li className="auth__point">
            <span className="auth__point-dot" aria-hidden="true" />
            <div>
              <strong>Every action is attributed</strong>
              <span className="ui-muted">Writes are audited against your account.</span>
            </div>
          </li>
        </ul>
        <p className="ui-muted t-sm">© 2026 Veylix — self-hosted VPN platform.</p>
      </aside>
      <main className="auth__main">
        <Card className="auth__card">
          <div>
            <h2 className="auth__title">Staff sign-in</h2>
            <p className="ui-muted t-base" style={{ margin: '4px 0 0' }}>
              Sign in with your username or e-mail.
            </p>
          </div>

          <div className="auth__roles" role="tablist" aria-label="Sign in as">
            <button
              type="button"
              role="tab"
              aria-selected={hint === 'admin'}
              className={`auth__role ${hint === 'admin' ? 'is-active' : ''}`}
              onClick={() => setHint('admin')}
            >
              <Store size={14} aria-hidden="true" /> Administrator
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={hint === 'super'}
              className={`auth__role ${hint === 'super' ? 'is-active' : ''}`}
              onClick={() => setHint('super')}
            >
              <ShieldCheck size={14} aria-hidden="true" /> Super Admin
            </button>
          </div>
          <p className="auth__hint ui-muted">
            {hint === 'super'
              ? 'The Super Admin console: servers, VPNs, clients, administrators and the audit log.'
              : 'The reseller console: your assigned servers, your clients, sales and payments.'}
          </p>

          <form onSubmit={submit} className="ui-col" noValidate>
            <Input
              label="Username or e-mail"
              autoComplete="username"
              placeholder="owner"
              value={identifier}
              onChange={(e) => setIdentifier(e.target.value)}
            />
            <Input
              label="Password"
              type="password"
              autoComplete="current-password"
              placeholder="••••••••"
              error={error || authError || undefined}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            <Button type="submit" loading={pending} leading={<ArrowRight size={15} />}>
              Sign in to staff portal
            </Button>
          </form>

          <div className="auth__switch auth__switch--split">
            <Link to="/forgot?portal=staff">Forgot your password?</Link>
            <Link to="/login">Customer login</Link>
          </div>
        </Card>
      </main>
    </div>
  )
}
