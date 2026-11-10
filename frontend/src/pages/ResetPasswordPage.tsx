import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { ArrowLeft, CheckCircle2 } from 'lucide-react'
import { Button, Card, Input } from '../components/ui'
import { VeylixLogo } from '../components/brand'
import { api, ApiError } from '../lib/api'
import { passwordHint, passwordViolations } from '../lib/passwordPolicy'
import './auth.css'

/**
 * /reset — redeem the 6-digit code and set a new password (D-044).
 *
 * On success every session for the account is signed out by the server, so the
 * screen says so and sends the person to sign in again with the new password.
 */
export function ResetPasswordPage() {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [identifier, setIdentifier] = useState(params.get('identifier') ?? '')
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [done, setDone] = useState(false)

  const problems = passwordViolations(password)
  const strong = password.length > 0 && problems.length === 0

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    if (!identifier.trim()) {
      setError('Enter your e-mail address or username.')
      return
    }
    if (!/^\d{6}$/.test(code.trim())) {
      setError('The code is six digits.')
      return
    }
    if (!strong) {
      setError(`Password ${problems[0] ?? 'does not meet the policy'}.`)
      return
    }
    setBusy(true)
    try {
      await api.resetPassword(identifier.trim(), code.trim(), password)
      setDone(true)
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.code === 'invalid_code') {
          setError('That code is not valid or has expired. Request a new one.')
        } else if (err.code === 'rate_limited') {
          setError('Too many attempts. Wait a few minutes and try again.')
        } else if (err.code === 'weak_password' || err.code === 'password_repeated' ||
                   err.code === 'password_sequence' || err.code === 'password_common') {
          setError(err.message)
        } else {
          setError(err.message)
        }
      } else {
        setError('Could not reach the server.')
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="auth" data-testid="reset">
      <aside className="auth__brand">
        <div>
          <VeylixLogo size={40} />
          <h1 className="auth__brand-title">Choose a new password</h1>
          <p className="auth__brand-sub ui-muted">
            The code can be used once. Signing in again afterwards uses the new
            password, and every other device is signed out.
          </p>
        </div>
      </aside>

      <main className="auth__main">
        <Card className="auth__card">
          <h1 className="auth__title">Enter your code</h1>

          {done ? (
            <div>
              <div className="auth__notice auth__notice--ok">
                <CheckCircle2 size={18} />
                <strong>Password changed.</strong>
                <p>Every session was signed out. Sign in with the new password.</p>
              </div>
              <Button variant="primary" onClick={() => navigate('/login')}>
                Go to sign in
              </Button>
            </div>
          ) : (
            <form onSubmit={submit} noValidate>
              <Input
                label="E-mail or username"
                value={identifier}
                autoComplete="username"
                onChange={(e) => setIdentifier(e.target.value)}
                placeholder="you@example.com"
              />
              <Input
                label="6-digit code"
                value={code}
                inputMode="numeric"
                maxLength={6}
                autoFocus={Boolean(identifier)}
                onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))}
                placeholder="123456"
                hint="From the e-mail we sent. It expires in 10 minutes."
              />
              <Input
                label="New password"
                type="password"
                value={password}
                autoComplete="new-password"
                onChange={(e) => setPassword(e.target.value)}
                error={error || undefined}
                hint={
                  password && !strong
                    ? passwordHint(password)
                    : 'At least 12 characters, not a repeat or a sequence.'
                }
              />
              <Button type="submit" variant="primary" loading={busy}>
                {busy ? 'Saving…' : 'Set new password'}
              </Button>
            </form>
          )}

          <p className="auth__switch">
            <Link to="/forgot">
              <ArrowLeft size={14} /> Need a new code?
            </Link>
          </p>
        </Card>
      </main>
    </div>
  )
}
