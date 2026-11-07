import { useState, type FormEvent } from 'react'
import { Link, Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { ArrowRight, Chrome } from 'lucide-react'
import { Button, Card, Input, useToast } from '../components/ui'
import { VeylixLogo } from '../components/brand'
import { useAuthStore } from '../stores/authStore'
import { googleErrorMessage, useGoogleButton } from '../hooks/useGoogleButton'
import { MIN_PASSWORD_LENGTH, passwordHint } from '../lib/passwordPolicy'
import './auth.css'

export function RegisterPage() {
  const { user, status, signUp, pending, error: authError, clearError } = useAuthStore()
  const toast = useToast()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const google = useGoogleButton()
  const googleError = googleErrorMessage(params.get('google'))
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')

  if (status === 'authed' && user) return <Navigate to="/client" replace />

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !email.trim() || !password || !confirm) {
      setError('All fields are required.')
      return
    }
    // The backend enforces 12 characters; check here for instant feedback.
    if (password.length < MIN_PASSWORD_LENGTH) {
      setError(`Password must be at least ${MIN_PASSWORD_LENGTH} characters.`)
      return
    }
    if (password !== confirm) {
      setError('Passwords do not match.')
      return
    }
    setError('')
    clearError()
    const result = await signUp(name.trim(), email.trim(), password)
    if (!result) return // the store surfaces the reason inline

    // 'pending' is the normal case: the account is created UNVERIFIED, so the
    // server issued NO session — signing the person in here would send them to a
    // screen whose every call answers 403. The honest next step is the mailbox.
    // 'authed' only happens for an auto-verified account (operator-created).
    if (result === 'authed') {
      toast('success', 'Account created', 'Welcome to Veylix')
      navigate('/client', { replace: true })
      return
    }
    toast(
      'success',
      'Check your e-mail',
      `We sent a confirmation link to ${email.trim()} — open it, then sign in`,
    )
    navigate(`/login?pending=${encodeURIComponent(email.trim())}`, { replace: true })
  }

  return (
    <div className="auth" data-testid="register">
      <aside className="auth__brand">
        <div>
          <VeylixLogo size={40} showText />
          <h1 className="auth__brand-title">Create your Veylix account</h1>
          <p className="auth__brand-sub ui-muted">
            Pick a free offer, buy Premium from a reseller, and manage every VPN
            you own from a single premium dashboard.
          </p>
        </div>
        <ul className="auth__points">
          <li className="auth__point">
            <span className="auth__point-dot" aria-hidden="true" />
            <div>
              <strong>Free offers with your own credentials</strong>
              <span className="ui-muted">Never a shared login — a distinct client per user.</span>
            </div>
          </li>
          <li className="auth__point">
            <span className="auth__point-dot" aria-hidden="true" />
            <div>
              <strong>Everything in one place</strong>
              <span className="ui-muted">Usage, limits, QR, links, guides and seller chat.</span>
            </div>
          </li>
        </ul>
        <p className="ui-muted t-sm">© 2026 Veylix — self-hosted VPN platform.</p>
      </aside>
      <main className="auth__main">
        <Card className="auth__card">
          <div>
            <h2 className="auth__title">Create account</h2>
            <p className="ui-muted t-base" style={{ margin: '4px 0 0' }}>
              Free to join — takes less than a minute.
            </p>
          </div>
          {googleError && (
            <p className="auth__notice auth__notice--bad" role="alert">
              {googleError}
            </p>
          )}
          <Button
            variant="outline"
            className="auth__google"
            leading={<Chrome size={16} />}
            disabled={!google.available}
            title={google.available ? undefined : 'Google sign-up is not configured on this server'}
            onClick={google.start}
          >
            Sign up with Google
          </Button>
          {google.configured === false && (
            <p className="auth__hint ui-muted">
              Google sign-up is not enabled on this server — create an account with your e-mail.
            </p>
          )}
          <div className="auth__divider">or</div>
          <form onSubmit={submit} className="ui-col" noValidate>
            <Input label="Full name" placeholder="Kasun Perera" value={name} onChange={(e) => setName(e.target.value)} />
            <Input
              label="Email"
              type="email"
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
              Create account
            </Button>
          </form>
          <p className="auth__switch">
            Already have an account? <Link to="/login">Sign in</Link>
          </p>
        </Card>
      </main>
    </div>
  )
}