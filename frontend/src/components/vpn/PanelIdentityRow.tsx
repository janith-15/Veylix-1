import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { panelIdentity } from '../../stores/vpnStore'
import './panel-identity.css'

/**
 * The VPN's identity on its server, rendered so a customer can READ it and COPY
 * it (D-091.4, non-negotiable 7).
 *
 * Three things had to be true at once, and the old screen got all three wrong:
 *
 *  1. **Readable.** `nimal@gmail.com_paid_2` is a machine string. Shown as
 *     `nimal@gmail.com` plus a `Premium · #2` chip it becomes a sentence a person
 *     understands, and the kind is stated rather than left to be decoded.
 *  2. **Copyable.** The copy button copies the EXACT panel email, not the pretty
 *     form — that string is what an operator will paste into 3X-UI to find the
 *     client. Copying the label would send support hunting for a row that does
 *     not exist.
 *  3. **Never substituted.** It is the identity, not a secret and not a
 *     decoration. If the panel email is missing the row says so instead of
 *     quietly rendering the package name in its place.
 *
 * The copy confirmation is announced through an `aria-live` region, because a
 * button whose only feedback is a 1.5-second icon swap is silent to a screen
 * reader and easy to miss for everyone else.
 */
export function PanelIdentityRow({
  panelEmail,
  serverLabel,
  /** `inline` for list rows, `block` for the detail page header. */
  layout = 'inline',
  testId = 'panel-identity',
}: {
  panelEmail: string
  serverLabel?: string
  layout?: 'inline' | 'block'
  testId?: string
}) {
  const [copied, setCopied] = useState(false)
  const id = panelIdentity(panelEmail)

  const copy = async () => {
    if (id.email === '') return
    try {
      await navigator.clipboard.writeText(id.email)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1600)
    } catch {
      // A clipboard refusal (insecure context, or the user declined) is not a
      // reason to claim success. The value stays on screen to be selected by
      // hand, and no "Copied" state is shown.
      setCopied(false)
    }
  }

  if (id.email === '') {
    return (
      <span className="pid pid--missing" data-testid={testId}>
        This VPN has no panel identity recorded — contact your seller before relying on it.
      </span>
    )
  }

  return (
    <span className={`pid pid--${layout}`} data-testid={testId}>
      <span className="pid__text">
        <span className="pid__account" title={id.email}>
          {id.account}
        </span>
        {id.kindLabel !== null && id.slot !== null && (
          <span className="pid__chip" data-kind={id.kindLabel.toLowerCase()}>
            {id.kindLabel} · #{id.slot}
          </span>
        )}
        {serverLabel && <span className="pid__server">{serverLabel}</span>}
      </span>
      <button
        type="button"
        className="pid__copy"
        onClick={() => void copy()}
        /* The accessible name says WHICH string is copied, so a screen-reader
           user is not asked to trust a bare "Copy". */
        aria-label={`Copy the panel identity ${id.email}`}
        title={`Copy “${id.email}”`}
        data-testid={`${testId}-copy`}
      >
        {copied ? <Check size={13} aria-hidden="true" /> : <Copy size={13} aria-hidden="true" />}
        <span className="pid__copy-label">{copied ? 'Copied' : 'Copy'}</span>
      </button>
      <span className="ui-sr-only" role="status" aria-live="polite">
        {copied ? `Copied the panel identity ${id.email}` : ''}
      </span>
    </span>
  )
}
