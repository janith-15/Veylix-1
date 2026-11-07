import { useState } from 'react'
import { Check, Copy, Terminal } from 'lucide-react'
import { Dialog, Button, Input } from '../ui'

/**
 * One-command installer.
 *
 * Renders the `curl | bash` line an operator pastes on a fresh Linux box. The
 * command is BUILT HERE from the two fields — this dialog never claims to have
 * installed anything and never contacts a server: it is a command builder, so a
 * missing install endpoint cannot make it lie (the honesty rule in
 * `audit-ui-truth.mjs`).
 *
 * The hostname is validated as a bare domain because it is interpolated into a
 * shell command: a value containing `;` or a newline would let a typo become a
 * second command on the operator's server.
 */
const DOMAIN_RE = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/i

export function InstallerDialog({
  open,
  onClose,
  defaultDomain = '',
}: {
  open: boolean
  onClose: () => void
  defaultDomain?: string
}) {
  const [domain, setDomain] = useState(defaultDomain)
  const [email, setEmail] = useState('')
  const [copied, setCopied] = useState(false)

  const domainOk = DOMAIN_RE.test(domain.trim())
  const emailOk = /^[^\s@;`]+@[^\s@;`]+\.[^\s@;`]+$/.test(email.trim())
  const ready = domainOk && emailOk

  const command = ready
    ? `curl -sSL https://get.veylix.io/install.sh | sudo bash -s -- --domain ${domain.trim()} --email ${email.trim()} --port 443`
    : ''

  const copy = async () => {
    if (!command) return
    try {
      await navigator.clipboard.writeText(command)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard can be blocked (insecure origin, denied permission). Leaving
      // the text selectable is the honest fallback; a fake "Copied!" is not.
      setCopied(false)
    }
  }

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title="One-command installer"
      footer={
        <>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Close
          </Button>
          <Button
            size="sm"
            onClick={copy}
            disabled={!ready}
            leading={copied ? <Check size={14} /> : <Copy size={14} />}
            data-autofocus
          >
            {copied ? 'Copied' : 'Copy command'}
          </Button>
        </>
      }
    >
      <p className="ui-muted t-sm2">
        Paste this on a fresh Debian, Ubuntu or Alpine host (1 vCPU / 500 MB is
        enough). It installs the binary, creates the database and prints the first
        Super Admin setup link.
      </p>

      <Input
        label="Domain"
        placeholder="vpn.example.com"
        value={domain}
        onChange={(e) => setDomain(e.target.value)}
        error={domain && !domainOk ? 'Enter a bare hostname, no scheme or path' : undefined}
      />
      <Input
        label="Contact e-mail"
        placeholder="ops@example.com"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        error={email && !emailOk ? 'Enter a valid e-mail address' : undefined}
      />

      <div className="ui-col" style={{ gap: 6 }}>
        <span className="ui-label">Command</span>
        <pre
          className="ui-code"
          data-testid="installer-command"
          style={{
            margin: 0,
            padding: 'var(--space-3)',
            borderRadius: 'var(--radius-sm)',
            border: '1px solid var(--border)',
            background: 'var(--bg-surface)',
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-all',
            fontSize: '0.78rem',
          }}
        >
          <Terminal size={13} aria-hidden="true" /> {command || 'Fill in a domain and e-mail first.'}
        </pre>
      </div>
    </Dialog>
  )
}
