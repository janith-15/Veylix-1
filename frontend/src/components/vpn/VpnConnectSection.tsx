import { useCallback, useEffect, useState } from 'react'
import { Copy, ExternalLink, Eye, EyeOff, HelpCircle, QrCode as QrIcon, TriangleAlert } from 'lucide-react'
import { Button, Card, QrCode, Tabs, useToast } from '../ui'
import { api, ApiError, type ApiVpnLinks, type ApiVpnProtocol } from '../../lib/api'
import './vpn-connect.css'

/**
 * The "Connection & Help" section (owner's brief §5 and §9).
 *
 * THE ONE RULE THIS SCREEN ENFORCES
 *
 * The QR and the URI are HIDDEN until asked for, then revealed explicitly, and
 * the link can be copied. That is not politeness: a connection URI is a working
 * credential for that VPN, so it should not be painted on a page someone may be
 * screen-sharing, and a customer should be able to hand the exact string to
 * their client without selecting text with a mouse.
 *
 * SETUP STEPS FOLLOW THE REAL PROTOCOL
 *
 * The steps name the app and the import flow for the protocol the panel actually
 * reports. A guide that tells a WireGuard user to paste a `vless://` URI is worse
 * than no guide, so when the protocol is unknown this section says so rather
 * than guessing.
 */

interface Step {
  title: string
  body: string
}

/**
 * Apps and steps per protocol family.
 *
 * Only widely used, open-source-or-official clients are named: recommending an
 * app is a promise it will not be abandoned or bundled with adware.
 */
const GUIDES: Record<string, { apps: { name: string; url: string }[]; steps: Step[] }> = {
  vless: {
    apps: [
      { name: 'v2rayNG (Android)', url: 'https://github.com/2dust/v2rayNG/releases' },
      { name: 'v2rayN (Windows)', url: 'https://github.com/2dust/v2rayN/releases' },
      { name: 'Streisand (iOS)', url: 'https://apps.apple.com/app/streisand/id6450534064' },
    ],
    steps: [
      {
        title: 'Install a VPN app',
        body: 'Use one of the apps above. They all accept a vless:// link.',
      },
      {
        title: 'Copy your link',
        body: 'Press “Copy link” in this section. The whole configuration is inside that one string.',
      },
      {
        title: 'Import it',
        body: 'In the app choose “Import from clipboard” (v2rayNG: the + button → Import config from clipboard).',
      },
      {
        title: 'Turn it on',
        body: 'Select the new entry and press connect. The app shows a key in your status bar when the VPN is active.',
      },
      {
        title: 'Still stuck?',
        body: 'See the troubleshooting list below, or open chat with your seller.',
      },
    ],
  },
  vmess: {
    apps: [
      { name: 'v2rayNG (Android)', url: 'https://github.com/2dust/v2rayNG/releases' },
      { name: 'v2rayN (Windows)', url: 'https://github.com/2dust/v2rayN/releases' },
      { name: 'Streisand (iOS)', url: 'https://apps.apple.com/app/streisand/id6450534064' },
    ],
    steps: [
      { title: 'Install a VPN app', body: 'Use one of the apps above. They all accept a vmess:// link.' },
      { title: 'Copy your link', body: 'Press “Copy link” in this section.' },
      { title: 'Import it', body: 'In the app choose “Import from clipboard”.' },
      { title: 'Turn it on', body: 'Select the entry and connect.' },
      { title: 'Still stuck?', body: 'See the troubleshooting list below, or open chat with your seller.' },
    ],
  },
  trojan: {
    apps: [
      { name: 'v2rayNG (Android)', url: 'https://github.com/2dust/v2rayNG/releases' },
      { name: 'v2rayN (Windows)', url: 'https://github.com/2dust/v2rayN/releases' },
    ],
    steps: [
      { title: 'Install a VPN app', body: 'Use one of the apps above. They accept a trojan:// link.' },
      { title: 'Copy your link', body: 'Press “Copy link” in this section.' },
      { title: 'Import it', body: 'In the app choose “Import from clipboard”.' },
      { title: 'Turn it on', body: 'Select the entry and connect.' },
      { title: 'Still stuck?', body: 'See the troubleshooting list below, or open chat with your seller.' },
    ],
  },
  shadowsocks: {
    apps: [
      { name: 'Shadowsocks (Android)', url: 'https://github.com/shadowsocks/shadowsocks-android/releases' },
      { name: 'Shadowsocks (Windows)', url: 'https://github.com/shadowsocks/shadowsocks-windows/releases' },
    ],
    steps: [
      { title: 'Install a VPN app', body: 'Use one of the apps above. They accept an ss:// link.' },
      { title: 'Copy your link', body: 'Press “Copy link” in this section.' },
      { title: 'Import it', body: 'In the app choose “Import from clipboard”.' },
      { title: 'Turn it on', body: 'Select the entry and connect.' },
      { title: 'Still stuck?', body: 'See the troubleshooting list below, or open chat with your seller.' },
    ],
  },
}
/** What a customer can try, in the order that actually fixes things. */
const TROUBLESHOOTING: { title: string; body: string }[] = [
  {
    title: 'The VPN will not connect',
    body: 'Check that the app is not paused by a battery saver, then try a different network. If the status on this page is not Active, that is the cause and the message above explains it.',
  },
  {
    title: 'The QR code will not scan',
    body: 'Scan it with the VPN app itself, not the camera app, or use “Copy link” and import from the clipboard instead.',
  },
  {
    title: 'The configuration will not import',
    body: 'Copy the link again from this page — a link that was edited or wrapped by a chat app will not import. Paste it whole, with no spaces added.',
  },
  {
    title: 'The internet feels slow',
    body: 'A VPN adds a hop, so some slowdown is normal. Compare with the VPN off: if it is fine without it, the server is the likely cause, and the performance chart above shows whether it has been consistently slow.',
  },
  {
    title: 'It keeps disconnecting',
    body: 'Most disconnects come from the phone sleeping the VPN app. Allow the app to run in the background, and turn off any “data saver” for it.',
  },
  {
    title: 'The VPN has expired',
    body: 'An expired VPN cannot connect. Buy a new package, or ask your seller about a renewal.',
  },
  {
    title: 'My data is used up',
    body: 'The allowance is finished for this package. Buy a new package to continue — a new VPN is issued with its own allowance.',
  },
]


export function VpnConnectSection({
  vpnId,
  active,
  seller,
  onOpenChat,
}: {
  vpnId: number
  active: boolean
  seller: { name: string; email: string; whatsapp: string; telegram: string; id: number }
  onOpenChat: () => void
}) {
  const toast = useToast()
  const [links, setLinks] = useState<ApiVpnLinks | null>(null)
  const [linksError, setLinksError] = useState<string | null>(null)
  const [started, setStarted] = useState(false)
  const [revealed, setRevealed] = useState<Record<number, boolean>>({})
  const [copied, setCopied] = useState<number | null>(null)
  const [protocol, setProtocol] = useState<ApiVpnProtocol | null>(null)

  /* The link is fetched only when asked for. Until then this section holds no
     credential at all — which is also why the QR is not drawn on first paint. */
  const loadLinks = useCallback(() => {
    if (links !== null || linksError !== null) return
    void api
      .vpnLinks(vpnId)
      .then(setLinks)
      .catch((err: unknown) => {
        setLinksError(err instanceof ApiError ? err.message : 'The connection link could not be read')
      })
  }, [vpnId, links, linksError])

  useEffect(() => {
    void api
      .vpnProtocol(vpnId)
      .then(setProtocol)
      .catch(() => setProtocol(null))
  }, [vpnId])

  const protocolKey = (protocol?.protocol ?? '').toLowerCase()
  const guide = GUIDES[protocolKey] ?? null

  const copy = async (uri: string, index: number) => {
    try {
      await navigator.clipboard.writeText(uri)
      setCopied(index)
      window.setTimeout(() => setCopied(null), 1600)
      toast('success', 'Connection link copied')
    } catch {
      // Never claim a copy that did not happen.
      toast('error', 'Copy failed', 'The browser refused the copy - select the link and copy it manually.')
    }
  }

  const uris = links?.links ?? []

  return (
    <div className="vcn" data-testid="vpn-connect-section">
      <Card
        title="Connection"
        actions={
          <span className="vui-muted t-xs">
            {protocol?.source === 'panel'
              ? `${protocol.protocol.toUpperCase()}${protocol.transport ? ` · ${protocol.transport.toUpperCase()}` : ''}`
              : protocol
                ? 'Protocol could not be read'
                : ''}
          </span>
        }
      >
        {!active ? (
          <div className="vcn-warn">
            <TriangleAlert size={15} aria-hidden="true" />
            <span>
              The connection details are hidden while this VPN is not active. Nothing has been
              deleted — they return as soon as the VPN is active again.
            </span>
          </div>
        ) : !started ? (
          <div className="vcn-gate">
            <p className="vui-muted">
              Your connection link is a credential for this VPN, so it stays hidden until you ask
              for it.
            </p>
            <Button
              variant="primary"
              leading={<QrIcon size={14} />}
              onClick={() => {
                setStarted(true)
                loadLinks()
              }}
              data-testid="vcn-reveal"
            >
              Show QR code and link
            </Button>
          </div>
        ) : linksError ? (
          <div className="vcn-warn">
            <TriangleAlert size={15} aria-hidden="true" />
            <span>{linksError}</span>
          </div>
        ) : links === null ? (
          <p className="vui-muted">Loading your connection details…</p>
        ) : links.source === 'unavailable' ? (
          <div className="vcn-warn">
            <TriangleAlert size={15} aria-hidden="true" />
            <span>
              The server could not be read, so we cannot show a link right now. That is not the
              same as “no link exists” — try again shortly.
            </span>
          </div>
        ) : uris.length === 0 ? (
          <div className="vcn-warn">
            <TriangleAlert size={15} aria-hidden="true" />
            <span>
              The server reports this VPN but returned no connection link. Contact your seller — a
              placeholder link would look valid and fail to connect.
            </span>
          </div>
        ) : (
          <div className="vcn-links">
            {uris.map((uri, i) => (
              <div className="vcn-link" key={uri}>
                <div className="vcn-link__head">
                  <span className="vui-muted">
                    Connection link{uris.length > 1 ? ` ${i + 1}` : ''}
                  </span>
                  <div className="vcn-link__actions">
                    <Button
                      size="sm"
                      variant="ghost"
                      leading={revealed[i] ? <EyeOff size={13} /> : <Eye size={13} />}
                      onClick={() => setRevealed((r) => ({ ...r, [i]: !r[i] }))}
                      data-testid={`vcn-toggle-${i}`}
                    >
                      {revealed[i] ? 'Hide' : 'Show'}
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      leading={<Copy size={13} />}
                      onClick={() => void copy(uri, i)}
                      data-testid={`vcn-copy-${i}`}
                    >
                      {copied === i ? 'Copied' : 'Copy link'}
                    </Button>
                  </div>
                </div>
                {revealed[i] ? (
                  <>
                    <QrCode value={uri} label={`QR code for connection link ${i + 1}`} size={168} />
                    <code className="vcn-link__uri">{uri}</code>
                  </>
                ) : (
                  <p className="vui-muted">
                    Hidden. Press “Show” to display the QR code, or “Copy link” to copy it without
                    showing it.
                  </p>
                )}
              </div>
            ))}
          </div>
        )}
      </Card>
      <Card title="How to connect">
        {protocol !== null && protocol.source === 'unavailable' ? (
          <p className="vui-muted">
            This VPN&apos;s protocol could not be read from the server, so step-by-step
            instructions cannot be given safely. Use the link above with any client that supports
            it, or ask your seller which app to install.
          </p>
        ) : protocol !== null && !guide ? (
          <p className="vui-muted">
            This VPN uses <strong className="ui-strong">{protocol.protocol}</strong>, which has no
            built-in guide here yet. Import the link above into any client that supports it.
          </p>
        ) : (
          <>
            <Tabs
              items={[
                { value: 'Android', label: 'Android' },
                { value: 'Windows', label: 'Windows' },
                { value: 'iOS', label: 'iOS' },
              ]}
              value="Android"
              onChange={() => undefined}
            />
            <ol className="vcn-steps">
              {(guide?.steps ?? []).map((s) => (
                <li key={s.title}>
                  <strong>{s.title}</strong>
                  <span>{s.body}</span>
                </li>
              ))}
            </ol>
            {guide && (
              <div className="vcn-apps">
                <span className="vui-muted t-xs">Apps that accept this protocol</span>
                {guide.apps.map((a) => (
                  <a
                    key={a.name}
                    href={a.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="vcn-app"
                  >
                    {a.name}
                    <ExternalLink size={12} aria-hidden="true" />
                    <span className="ui-sr-only"> (opens in a new tab)</span>
                  </a>
                ))}
              </div>
            )}
          </>
        )}
      </Card>

      <Card
        title="If it is not working"
        actions={<HelpCircle size={14} aria-hidden="true" className="vui-muted" />}
      >
        <ul className="vcn-faq">
          {TROUBLESHOOTING.map((t) => (
            <li key={t.title}>
              <strong>{t.title}</strong>
              <span>{t.body}</span>
            </li>
          ))}
        </ul>
      </Card>

      <Card title="Get help">
        {seller.id > 0 ? (
          <p className="vui-muted">
            <strong className="ui-strong">{seller.name || `Seller #${seller.id}`}</strong> sold you
            this VPN.
            {seller.email && (
              <>
                {' '}
                Email: <a href={`mailto:${seller.email}`}>{seller.email}</a>
              </>
            )}
            {seller.whatsapp && (
              <>
                {' '}
                WhatsApp:{' '}
                <a
                  href={`https://wa.me/${seller.whatsapp.replace(/\D/g, '')}`}
                  target="_blank"
                  rel="noreferrer"
                >
                  {seller.whatsapp}
                </a>
              </>
            )}
            {seller.telegram && (
              <>
                {' '}
                Telegram:{' '}
                <a
                  href={`https://t.me/${seller.telegram.replace(/^@+/, '').trim()}`}
                  target="_blank"
                  rel="noreferrer"
                >
                  {seller.telegram}
                </a>
              </>
            )}
          </p>
        ) : (
          <p className="vui-muted">
            This VPN was issued by the platform. Support is available through the internal chat.
          </p>
        )}
        <Button size="sm" variant="outline" onClick={onOpenChat}>
          Open chat
        </Button>
      </Card>
    </div>
  )
}
