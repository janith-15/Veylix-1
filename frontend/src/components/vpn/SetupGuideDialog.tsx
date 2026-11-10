import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { Dialog, Button } from '../ui'

/**
 * "How do I connect?" for a client.
 *
 * The steps are static text on purpose: they are the same four steps for every
 * protocol we ship, and the only variable part — the subscription link — is
 * passed in by the caller, which owns it. This dialog therefore needs no
 * endpoint and cannot show a stale credential: if there is no link it says so.
 */
const STEPS = [
  { title: 'Install a client', body: 'Use v2rayNG (Android), FoXray or Shadowrocket (iOS), or v2rayN on Windows.' },
  { title: 'Copy your link', body: 'Use the button below — it copies the link, not a screenshot, so nothing is retyped.' },
  { title: 'Import it', body: 'In the client choose “Import from clipboard” (or “+ → Import”). The profile appears automatically.' },
  { title: 'Connect', body: 'Tap the profile, then Connect. The first connection can take a few seconds while the tunnel opens.' },
]

export function SetupGuideDialog({
  open,
  onClose,
  link,
}: {
  open: boolean
  onClose: () => void
  /** The subscription/config link, if the caller has one. */
  link?: string
}) {
  const [copied, setCopied] = useState(false)

  const copy = async () => {
    if (!link) return
    try {
      await navigator.clipboard.writeText(link)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      setCopied(false)
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="How to connect"
      footer={
        <>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Close
          </Button>
          {link ? (
            <Button
              size="sm"
              onClick={copy}
              leading={copied ? <Check size={14} /> : <Copy size={14} />}
              data-autofocus
            >
              {copied ? 'Copied' : 'Copy link'}
            </Button>
          ) : null}
        </>
      }
    >
      <ol className="ui-col" style={{ gap: 'var(--space-3)', margin: 0, paddingLeft: 0, listStyle: 'none' }}>
        {STEPS.map((s, i) => (
          <li key={s.title} style={{ display: 'flex', gap: 'var(--space-3)' }}>
            <span
              aria-hidden="true"
              style={{
                flex: '0 0 auto',
                width: 22,
                height: 22,
                display: 'grid',
                placeItems: 'center',
                borderRadius: '999px',
                border: '1px solid var(--border)',
                background: 'var(--bg-surface)',
                fontSize: '0.72rem',
                fontWeight: 700,
              }}
            >
              {i + 1}
            </span>
            <div className="ui-col" style={{ gap: 2 }}>
              <strong style={{ fontSize: '0.86rem' }}>{s.title}</strong>
              <span className="ui-muted t-sm2">{s.body}</span>
            </div>
          </li>
        ))}
      </ol>

      {!link && (
        <p className="ui-muted t-sm2" role="status">
          No connection link is available for this VPN yet. It appears here once the
          server has provisioned it.
        </p>
      )}
    </Dialog>
  )
}
