import { useNavigate } from 'react-router-dom'
import { BookOpen, ExternalLink, ShieldCheck } from 'lucide-react'
import { Button, Card } from '../../components/ui'
import { ClientPage, ClientSection } from '../../components/client/ClientPage'
import '../../components/client/client.css'

/**
 * VPN Guides — client shell (S9 outer layer).
 *
 * SCOPE OF THIS FILE: the frame and the platform list. It links to the guide
 * that already exists rather than copying it.
 *
 * WHY THIS IS NOT THE GUIDE ITSELF
 *
 *   A working, written guide per platform already lives inside
 *   `VPNDetailPage.tsx` (`GUIDES`, :69-140) and is opened from the VPN's own
 *   "Setup guide" card. It names real apps, real download URLs and real steps,
 *   and it is the only copy in the product. Reproducing it here would create a
 *   second copy that drifts the moment one is edited — the exact duplication
 *   `docs/DOC_INDEX.md` forbids for documentation and which is just as wrong in
 *   code.
 *
 *   So this screen is a DIRECTORY: it tells the reader which platforms are
 *   covered, and sends them to the guide where their connection link lives.
 *   A guide is only useful next to the credential it configures.
 *
 * WHAT IS NOT COVERED — stated, not hidden
 *
 *   The existing guide set covers Android, iOS and Windows. macOS and Linux
 *   have no written guide, so this screen does not list them as if they did.
 *   That absence is a content gap in the S9 stage, recorded here in code so a
 *   reader sees the truth instead of a dead platform tile.
 */
const PLATFORMS: { key: string; label: string; apps: string }[] = [
  { key: 'Android', label: 'Android', apps: 'v2rayNG, NekoBox' },
  { key: 'iOS', label: 'iPhone / iPad', apps: 'Streisand, Shadowrocket' },
  { key: 'Windows', label: 'Windows', apps: 'v2rayN, NekoRay' },
]

export function ClientGuidesPage() {
  const navigate = useNavigate()

  return (
    <ClientPage
      testId="client-guides"
      title="VPN Guides"
      description="How to install a VPN app and import your connection on each platform"
      actions={
        <Button size="sm" onClick={() => navigate('/client/vpns')}>
          My VPNs
        </Button>
      }
    >
      <ClientSection title="Supported platforms">
        <Card>
          <div className="ui-col" style={{ gap: 10 }}>
            <span className="ui-row" style={{ gap: 8, alignItems: 'center' }}>
              <BookOpen size={16} aria-hidden="true" style={{ color: 'var(--text-muted)' }} />
              <strong>Each guide opens next to your own connection link</strong>
            </span>
            <p className="ui-muted t-sm2" style={{ margin: 0 }}>
              A setup guide is only useful beside the credential it configures, so the full steps
              live on each VPN&apos;s own screen. Open a VPN, then use its “Setup guide” card to pick
              your platform.
            </p>
            <div className="ui-col" style={{ gap: 6 }}>
              {PLATFORMS.map((p) => (
                <span
                  key={p.key}
                  className="ui-row"
                  style={{ gap: 8, alignItems: 'baseline', flexWrap: 'wrap' }}
                >
                  <span className="ui-strong">{p.label}</span>
                  <span className="ui-muted t-sm2">— {p.apps}</span>
                </span>
              ))}
            </div>
            <p className="ui-muted t-sm2" style={{ margin: 0 }}>
              macOS and Linux have no written guide in this build yet. The connection link and QR
              code work on those systems with any compatible client — ask your seller if you need
              help.
            </p>
            <div className="ui-row" style={{ gap: 8 }}>
              <Button size="sm" variant="primary" onClick={() => navigate('/client/vpns')}>
                Open my VPNs
              </Button>
              <Button
                size="sm"
                variant="ghost"
                leading={<ExternalLink size={13} />}
                onClick={() => navigate('/chat')}
              >
                Ask the seller
              </Button>
            </div>
          </div>
        </Card>
      </ClientSection>

      <ClientSection title="Before you start">
        <Card>
          <div className="ui-col" style={{ gap: 8 }}>
            <span className="ui-row" style={{ gap: 8, alignItems: 'center' }}>
              <ShieldCheck size={16} aria-hidden="true" style={{ color: 'var(--success)' }} />
              <strong>Only install an app you can verify</strong>
            </span>
            <p className="ui-muted t-sm2" style={{ margin: 0 }}>
              Every app named in a guide is open source or published on its own official store page.
              Anything else asking for your connection link is not part of this service — never paste
              a link into a client you did not choose.
            </p>
          </div>
        </Card>
      </ClientSection>
    </ClientPage>
  )
}
