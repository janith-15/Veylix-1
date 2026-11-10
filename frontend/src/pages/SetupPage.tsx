import { useState, type FormEvent } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import { ArrowRight, ShieldCheck } from 'lucide-react'
import { Button, Card, Input, useToast } from '../components/ui'
import { VeylixLogo } from '../components/brand'
import { useAuthStore } from '../stores/authStore'
import { MIN_PASSWORD_LENGTH, passwordHint, passwordViolations } from '../lib/passwordPolicy'
import './auth.css'

/**
 * First-run setup: creates the one and only Super Admin.
 *
 * This screen only renders while the backend reports `bootstrap_required`
 * (`GET /api/setup/status`). The server refuses a second bootstrap, so the
 * guard here is a convenience — never the security boundary.
 */
export function SetupPage() {
  const { user, status, bootstrapRequired, bootstrap, pending, error: authError, clearError } =
    useAuthStore()
  const toast = useToast()
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')

  if (status === 'authed' && user) return <Navigate to="/dashboard" replace />
  // Already bootstrapped → normal sign-in.
  if (status === 'anon' && !bootstrapRequired) return <Navigate to="/login" replace />

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !email.trim() || !password || !confirm) {
      setError('All fields are required.')
      return
    }
    const violations = passwordViolations(password)
    if (violations.length > 0) {
      setError(`Password ${violations[0]}.`)
      return
    }
    if (password !== confirm) {
      setError('Passwords do not match.')
      return
    }
    setError('')
    clearError()
    const ok = await bootstrap(name.trim(), email.trim(), password)
    if (!ok) return
    toast('success', 'Super Admin created', 'You are signed in and ready to go')
    navigate('/dashboard', { replace: true })
  }

  return (
    <div className="auth" data-testid="setup">
      <aside className="auth__brand">
        <div>
          <VeylixLogo size={40} showText />
          <h1 className="auth__brand-title">Set up your Veylix control plane</h1>
          <p className="auth__brand-sub ui-muted">
            This installation has no accounts yet. Create the first Super Admin —
            the only account with unrestricted access — and you will be signed in
            immediately.
          </p>
        </div>
        <ul className="auth__points">
          <li className="auth__point">
            <span className="auth__point-dot" aria-hidden="true" />
            <div>
              <strong>One-time only</strong>
              <span className="ui-muted">
                Once an account exists this screen is closed for good.
              </span>
            </div>
          </li>
          <li className="auth__point">
            <span className="auth__point-dot" aria-hidden="true" />
            <div>
              <strong>No default password</strong>
              <span className="ui-muted">
                You choose the credential; it is hashed with argon2id and never stored in plaintext.
              </span>
            </div>
          </li>
        </ul>
      </aside>
      <main className="auth__main">
        <Card className="auth__card">
          <div>
            <h2 className="auth__title">Create the first Super Admin</h2>
            <p className="ui-muted t-base" style={{ margin: '4px 0 0' }}>
              This account cannot be created again later.
            </p>
          </div>
          <form onSubmit={submit} className="ui-col" noValidate>
            <Input
              label="Full name"
              autoComplete="name"
              placeholder="Kasun Perera"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <Input
              label="Email"
              type="email"
              autoComplete="username"
              placeholder="you@example.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
            <Input
              label="Password"
              type="password"
              autoComplete="new-password"
              placeholder="••••••••"
              hint={
                password.length === 0
                  ? `Use at least ${MIN_PASSWORD_LENGTH} characters`
                  : passwordHint(password)
              }
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            <Input
              label="Confirm password"
              type="password"
              autoComplete="new-password"
              placeholder="••••••••"
              error={error || authError || undefined}
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
            />
            <Button type="submit" loading={pending} leading={<ArrowRight size={15} />}>
              Create Super Admin &amp; sign in
            </Button>
          </form>
          <p className="ui-muted t-sm" style={{ margin: 0, display: 'flex', gap: 6 }}>
            <ShieldCheck size={14} aria-hidden="true" />
            The server refuses this request once any account exists.
          </p>
        </Card>
      </main>
    </div>
  )
}
