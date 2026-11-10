import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Input, PageHeader, Skeleton, useToast } from '../components/ui'
import { ArrowUpCircle, Copy, KeyRound, Server, ShieldCheck } from 'lucide-react'
import {
  ApiError,
  api,
  type ApiGoogleSettings,
  type ApiPublicURL,
  type ApiSystemVersion,
  type ApiUpdateCheck,
} from '../lib/api'
import './super-settings.css'

/** Copy text to the clipboard, reporting whether it worked. */
async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

/**
 * Super Admin → Settings (D-080).
 *
 * Two things live here that an operator used to do only over SSH: seeing which
 * release is actually running, and editing the Google OAuth client. Both are
 * honest about their limits — an update cannot be installed from the browser
 * (that needs root), so the page hands over the exact command instead of a
 * button that would fail; and the OAuth secret is write-only, never read back.
 */
export function SuperSettingsPage() {
  const toast = useToast()

  const [version, setVersion] = useState<ApiSystemVersion | null>(null)
  const [update, setUpdate] = useState<ApiUpdateCheck | null>(null)
  const [loadErr, setLoadErr] = useState<string | null>(null)

  const [google, setGoogle] = useState<ApiGoogleSettings | null>(null)
  const [publicURL, setPublicURL] = useState<ApiPublicURL | null>(null)
  const [clientId, setClientId] = useState('')
  const [secret, setSecret] = useState('')
  const [redirect, setRedirect] = useState('')
  const [saving, setSaving] = useState(false)
  const [formErr, setFormErr] = useState<string | null>(null)

  const loadSystem = useCallback(async () => {
    const [v, u, p] = await Promise.allSettled([
      api.systemVersion(),
      api.updateCheck(),
      api.publicURL(),
    ])
    if (v.status === 'fulfilled') setVersion(v.value)
    if (u.status === 'fulfilled') setUpdate(u.value)
    // A failed read leaves publicURL null, which hides the card rather than
    // showing an invented address.
    if (p.status === 'fulfilled') setPublicURL(p.value)
  }, [])

  const loadGoogle = useCallback(async () => {
    try {
      const g = await api.googleSettings()
      setGoogle(g)
      setClientId(g.client_id)
      setRedirect(g.redirect_url)
      setSecret('')
      setFormErr(null)
    } catch (err) {
      setFormErr(err instanceof ApiError ? err.message : 'Could not read the settings.')
    }
  }, [])

  useEffect(() => {
    void loadSystem().catch(() => setLoadErr('Could not reach the server.'))
    void loadGoogle()
  }, [loadSystem, loadGoogle])

  async function save() {
    setSaving(true)
    setFormErr(null)
    try {
      const g = await api.saveGoogleSettings({
        client_id: clientId,
        client_secret: secret,
        redirect_url: redirect,
      })
      setGoogle(g)
      setSecret('')
      toast(
        'success',
        'Settings saved and applied',
        g.configured ? 'Google sign-in is live now — no restart needed.' : 'Saved, but the client is not complete yet.',
      )
    } catch (err) {
      setFormErr(err instanceof ApiError ? err.message : 'Could not save the settings.')
    } finally {
      setSaving(false)
    }
  }

  const updateCmd = 'sudo vpn-platformctl update'
  // Derived server-side from the origin this page is served on (see
  // settings_routes.go), so it is the URL the running server actually answers
  // at — not the stored value, which can be mistyped.
  const expectedRedirect = google?.expected_redirect_url ?? ''

  return (
    <div className="settings-page" data-testid="settings-page">
      <PageHeader
        title="Settings"
        subtitle="The running version, available releases, and the Google sign-in client — editable here and applied live."
      />

      {loadErr && (
        <Card>
          <p className="t-sm2" role="alert">
            {loadErr}
          </p>
        </Card>
      )}

      <Card title="Platform version" actions={<Server size={16} aria-hidden />}>
        {version ? (
          <dl className="settings-facts">
            <div>
              <dt>Version</dt>
              <dd>
                <strong>{version.version}</strong>
              </dd>
            </div>
            <div>
              <dt>Schema</dt>
              <dd>v{version.schema_version}</dd>
            </div>
            <div>
              <dt>Interface</dt>
              <dd>{version.ui === 'not_embedded' ? 'API only (no embedded UI)' : version.ui}</dd>
            </div>
            <div>
              <dt>Runtime</dt>
              <dd>
                {version.go_version} · {version.platform}
              </dd>
            </div>
          </dl>
        ) : (
          <Skeleton height={90} />
        )}
      </Card>

      <Card title="Updates" actions={<ArrowUpCircle size={16} aria-hidden />}>
        {update ? (
          update.error ? (
            <p className="t-sm2">{`Could not check for updates: ${update.error}`}</p>
          ) : update.update_available ? (
            <>
              <p className="t-sm2">
                Update available: <strong>{update.latest}</strong> — you are running {update.current}.
              </p>
              <p className="ui-muted t-sm2">
                Installing an update needs root, so it is not run from the browser. On the server:
              </p>
              <div className="settings-copyline">
                <code>{updateCmd}</code>
                <Button
                  size="sm"
                  variant="outline"
                  leading={<Copy size={14} />}
                  onClick={() => void copyText(updateCmd).then((ok) => ok && toast('success', 'Copied', updateCmd))}
                >
                  Copy
                </Button>
              </div>
            </>
          ) : (
            <p className="t-sm2">You are on the latest release ({update.current}).</p>
          )
        ) : (
          <Skeleton height={60} />
        )}
      </Card>

      {publicURL && (
        <Card title="This deployment" actions={<Server size={16} aria-hidden />}>
          <p className="ui-muted t-sm2">
            The address Veylix is served at right now. It is read from the request you are making, not
            from a stored setting, so it cannot drift from the reverse proxy after a domain change.
          </p>
          <div className="settings-copyline" data-testid="public-url">
            <code>{publicURL.url}</code>
            <Button
              size="sm"
              variant="outline"
              leading={<Copy size={14} />}
              onClick={() => void copyText(publicURL.url).then((ok) => ok && toast('success', 'Copied', publicURL.url))}
            >
              Copy
            </Button>
          </div>
          <div className="settings-copyline" data-testid="public-login-url">
            <code>{publicURL.login_url}</code>
            <Button
              size="sm"
              variant="outline"
              leading={<Copy size={14} />}
              onClick={() =>
                void copyText(publicURL.login_url).then((ok) => ok && toast('success', 'Copied', publicURL.login_url))
              }
            >
              Copy sign-in link
            </Button>
          </div>
          <p className="ui-muted t-sm2">
            To change it, run <code>sudo vpn-platformctl</code> on the server and choose{' '}
            <strong>Change domain</strong>. A new address needs a matching DNS record and a certificate before
            it will work.
          </p>
        </Card>
      )}

      <Card title="Google sign-in" actions={<KeyRound size={16} aria-hidden />}>
        <p className="ui-muted t-sm2">
          The OAuth client used by “Continue with Google”. Saving here applies immediately, without a restart. The
          secret is write-only: it is stored sealed and can be replaced but never read back.
        </p>
        {google && (
          <p className="t-sm2 settings-google-state">
            <ShieldCheck size={14} aria-hidden />{' '}
            {google.configured ? 'Configured' : 'Not configured'}
            {google.has_secret ? ' · a secret is stored' : ' · no secret stored yet'}
            {` · source: ${google.source}`}
          </p>
        )}
        {expectedRedirect && (
          <div className="settings-callback" data-testid="google-redirect-hint">
            <p className="ui-muted t-sm2">
              Register this exact URL in Google Cloud Console → <strong>Authorised redirect URIs</strong>. It is
              derived from the address you are on right now, so it cannot disagree with the server.
            </p>
            <div className="settings-copyline">
              <code data-testid="google-expected-redirect">{expectedRedirect}</code>
              <Button
                size="sm"
                variant="outline"
                leading={<Copy size={14} />}
                onClick={() =>
                  void copyText(expectedRedirect).then(
                    (ok) => ok && toast('success', 'Copied', expectedRedirect),
                  )
                }
              >
                Copy
              </Button>
              <Button size="sm" variant="outline" onClick={() => setRedirect(expectedRedirect)}>
                Use this URL
              </Button>
            </div>
          </div>
        )}

        <div className="settings-form">
          <Input
            label="Client ID"
            hint="Ends in .apps.googleusercontent.com"
            value={clientId}
            onChange={(e) => setClientId(e.target.value)}
          />
          <Input
            label="Client secret"
            hint={google?.has_secret ? 'Leave blank to keep the stored secret' : 'Starts with GOCSPX-'}
            type="password"
            autoComplete="new-password"
            placeholder={google?.has_secret ? '•••••••• (stored)' : ''}
            value={secret}
            onChange={(e) => setSecret(e.target.value)}
          />
          <Input
            label="Redirect URL"
            hint="Must match Google Cloud Console exactly"
            value={redirect}
            onChange={(e) => setRedirect(e.target.value)}
          />
        </div>
        {formErr && (
          <p className="t-sm2" role="alert">
            {formErr}
          </p>
        )}
        <div className="settings-actions">
          <Button leading={<ShieldCheck size={14} />} onClick={() => void save()} disabled={saving}>
            {saving ? 'Saving…' : 'Save and apply'}
          </Button>
        </div>
      </Card>
    </div>
  )
}