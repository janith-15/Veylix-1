import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { ArrowLeft, ArrowRight, KeyRound } from 'lucide-react'
import { Button, Card, Input } from '../components/ui'
import { VeylixLogo } from '../components/brand'
import { api, ApiError } from '../lib/api'
import './auth.css'

/**
 * /forgot — ask for a 6-digit reset code (D-044).
 *
 * The server answers 202 whether or not the account exists, so this screen must
 * never say "we found you". It also reports how the code was delivered: with no
 * SMTP relay the API says `outbox`, and the honest message is "an operator will
 * hand you the code", not "check your inbox".
 */
export function ForgotPasswordPage() {
  const navigate = useNavigate()
  const [identifier, setIdentifier] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [sent, setSent] = useState<null | 'smtp' | 'outbox'>(null)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    if (!identifier.trim()) {
      setError('Enter your e-mail address or username.')
      return
    }
    setBusy(true)
    try {
      const res = await api.forgotPassword(identifier.trim())
      setSent(res.delivery === 'smtp' ? 'smtp' : 'outbox')
    } catch (err) {
      if (err instanceof ApiError && err.code === 'rate_limited') {
        setError('Too many requests. Wait a few minutes and try again.')
      } else {
        setError(err instanceof ApiError ? err.message : 'Could not reach the server.')
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="auth" data-testid="forgot">
      <aside className="auth__brand">
        <div>
          <VeylixLogo size={40} />
          <h1 className="auth__brand-title">Reset your password</h1>
          <p className="auth__brand-sub ui-muted">
            We send a 6-digit code, never a link. A code cannot be guessed by a mail
            scanner and works from any device.
          </p>
        </div>
      </aside>

      <main className="auth__main">
        <Card className="auth__card">
          <h1 className="auth__title">Forgotten password</h1>

          {sent === null ? (
            <form onSubmit={submit} noValidate>
              <p className="auth__hint ui-muted">
                Enter the e-mail address or username on your account and we will send a
                code that expires in 10 minutes.
              </p>
              <Input
                label="E-mail or username"
                value={identifier}
                autoComplete="username"
                autoFocus
                onChange={(e) => setIdentifier(e.target.value)}
                placeholder="you@example.com"
                error={error || undefined}
              />
              <Button type="submit" variant="primary" loading={busy}>
                <KeyRound size={16} />
                {busy ? 'Sending…' : 'Send me a code'}
              </Button>
            </form>
          ) : (
            <div>
              <div className="auth__notice auth__notice--ok">
                <strong>If that account exists, a code is on its way.</strong>
                <p>
                  {sent === 'smtp'
                    ? 'Check the inbox for that address. The code expires in 10 minutes.'
                    : 'No mail relay is configured on this server, so no e-mail was sent — ask an operator for the code with “.\\dev.ps1 -ResetLink”.'}
                </p>
              </div>
              <Button
                variant="primary"
                onClick={() =>
                  navigate(`/reset?identifier=${encodeURIComponent(identifier.trim())}`)
                }
              >
                I have the code
                <ArrowRight size={16} />
              </Button>
            </div>
          )}

          <p className="auth__switch">
            <Link to="/login">
              <ArrowLeft size={14} /> Back to sign in
            </Link>
          </p>
        </Card>
      </main>
    </div>
  )
}
