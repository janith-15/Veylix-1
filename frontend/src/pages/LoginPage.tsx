import { useState, type FormEvent } from 'react'
import { Link, Navigate, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { ArrowRight, Chrome } from 'lucide-react'
import { Button, Card, Input, useToast } from '../components/ui'
import { VeylixLogo } from '../components/brand'
import { useAuthStore } from '../stores/authStore'
import { googleErrorMessage, useGoogleButton } from '../hooks/useGoogleButton'
import './auth.css'

export function LoginPage() {
  const { user, status, signIn, pending, error: authError, clearError, bootstrapRequired, needsVerification, resendVerification } =
    useAuthStore()
  const toast = useToast()
  const navigate = useNavigate()
  const location = useLocation()
  const [params] = useSearchParams()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  /*
   * Resend state (D-078). Declared here, with the other state, because this
   * component returns early (Navigate to /dashboard or /setup) — a hook after
   * those returns is not called on every render, which `oxlint` refuses.
   */
  const [resendNote, setResendNote] = useState('')
  const [resendBusy, setResendBusy] = useState(false)
  // "Continue with Google" (D-039): the server does the whole exchange.
  const google = useGoogleButton()
  const googleError = googleErrorMessage(params.get('google'))
  const verified = params.get('verified')
  const welcome = params.get('welcome') === '1'
  // Set by /register: the account exists but must confirm its address first.
  // Named `awaitingEmail` because `pending` is already the store's in-flight flag.
  const awaitingEmail = params.get('pending')

  const from = (location.state as { from?: string } | null)?.from ?? '/dashboard'

  if (status === 'authed' && user) return <Navigate to={from} replace />
  // Nothing to sign in as until the first Super Admin exists.
  if (status === 'anon' && bootstrapRequired) return <Navigate to="/setup" replace />

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!email.trim() || !password) {
      setError('Email and password are required.')
      return
    }
    setError('')
    clearError()
    const ok = await signIn(email.trim(), password)
    if (!ok) return // the store surfaces the reason inline
    toast('success', 'Welcome back', 'Signed in to the Veylix control plane')
    navigate(from, { replace: true })
  }

  /**
   * "Send me a new link" (D-078).
   *
   * Offered only after a refusal that was specifically `email_unverified`, using
   * the address the person just typed. The reply is shown as the SERVER worded it:
   * the endpoint answers identically for an unknown address, so a cheerful "check
   * your inbox" would be a claim neither side can make.
   */
  const resend = async () => {
    setResendBusy(true)
    setResendNote('')
    const res = await resendVerification(email.trim())
    setResendBusy(false)
    if (res.error) {
      toast('error', 'Could not request a link', res.error)
      return
    }
    setResendNote(res.note ?? 'If that address needs confirming, a new link is on its way.')
  }

  return (
    <div className="auth" data-testid="login">
      <aside className="auth__brand">
        <div>
          <VeylixLogo size={40} showText />
          <h1 className="auth__brand-title">The control plane for your VPN business</h1>
          <p className="auth__brand-sub ui-muted">
            Manage 3X-UI panels, sell VPNs through resellers, and give clients a
            premium self-serve experience — one lightweight platform.
          </p>
        </div>
        <ul className="auth__points">
          <li className="auth__point">
            <span className="auth__point-dot" aria-hidden="true" />
            <div>
              <strong>Multi-panel management</strong>
              <span className="ui-muted">Health, sync and lifecycle for every 3X-UI server.</span>
            </div>
          </li>
          <li className="auth__point">
            <span className="auth__point-dot" aria-hidden="true" />
            <div>
              <strong>Reseller-ready</strong>
              <span className="ui-muted">Assigned servers, quotas, retail pricing and payments.</span>
            </div>
          </li>
          <li className="auth__point">
            <span className="auth__point-dot" aria-hidden="true" />
            <div>
              <strong>Client self-serve</strong>
              <span className="ui-muted">Free offers, Premium purchases, QR/link and setup guides.</span>
            </div>
          </li>
        </ul>
        <p className="ui-muted t-sm">© 2026 Veylix — self-hosted VPN platform.</p>
      </aside>
      <main className="auth__main">
        <Card className="auth__card">
          <div>
            <h2 className="auth__title">Sign in</h2>
            <p className="ui-muted t-base" style={{ margin: '4px 0 0' }}>
              Welcome back — enter your credentials.
            </p>
          </div>
          {welcome && (
            <p className="auth__notice auth__notice--ok" role="status">
              Your Google account created a Veylix account. Welcome!
            </p>
          )}
          {verified === '1' && (
            <p className="auth__notice auth__notice--ok" role="status">
              Your e-mail is verified — you can sign in now.
            </p>
          )}
          {verified === 'invalid' && (
            <p className="auth__notice auth__notice--bad" role="alert">
              That verification link is invalid or expired. Sign in and request a new one.
            </p>
          )}
          {awaitingEmail && (
            <p className="auth__notice auth__notice--ok" role="status">
              Account created. We e-mailed a confirmation link to <strong>{awaitingEmail}</strong> —
              open it, then sign in here.
            </p>
          )}
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
            title={google.available ? undefined : 'Google sign-in is not configured on this server'}
            onClick={google.start}
          >
            Continue with Google
          </Button>
          {google.configured === false && (
            <p className="auth__hint ui-muted">
              Google sign-in is not enabled on this server — use your e-mail and password.
            </p>
          )}
          <div className="auth__divider">or</div>
          <form onSubmit={submit} className="ui-col" noValidate>
            <Input
              label="Email or username"
              autoComplete="username"
              placeholder="you@veylix.app"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
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
              Sign in
            </Button>
            {needsVerification && (
              <div className="auth__notice auth__notice--bad" role="status">
                <p style={{ margin: 0 }}>
                  This account still needs its e-mail confirmed. If the link never arrived,
                  send a new one to <strong>{email.trim()}</strong>.
                </p>
                <Button
                  size="sm"
                  variant="outline"
                  loading={resendBusy}
                  onClick={() => void resend()}
                  style={{ marginTop: 8 }}
                >
                  Send a new confirmation link
                </Button>
                {resendNote && (
                  <p className="ui-muted t-sm2" style={{ margin: '8px 0 0' }}>
                    {resendNote}
                  </p>
                )}
              </div>
            )}
          </form>
          <p className="auth__switch">
            No account? <Link to="/register">Create one</Link>
          </p>
          <p className="auth__switch auth__switch--split">
            <Link to="/forgot">Forgot your password?</Link>
            {/* D-041: staff have their own door. */}
            <Link to="/staff/login">Staff sign-in</Link>
          </p>
        </Card>
      </main>
    </div>
  )
}
