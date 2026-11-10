import { useEffect, useMemo, useState } from 'react'
import { Check, X, Palette, LayoutDashboard, Server, Package, UserCog, CreditCard, Bell, Search } from 'lucide-react'
import { Badge, Button, Card, Input, Switch, Tabs, Table, type Column } from '../ui'
import { DonutChart, LineChart, Progress, Sparkline } from '../charts'
import { VeylixLogo } from '../brand'
import { SceneLayer } from '../effects/SceneBackground'
import { PRESET_LABELS, themeToCssVars, type ThemeSettings } from '../../theme/themes'
import { resolveVariant } from '../../theme/subthemes'
import './preview.css'

interface Row {
  id: string
  panel: string
  status: string
  clients: number
  load: number
}

const ROWS: Row[] = [
  { id: '1', panel: 'Frankfurt-01', status: 'ACTIVE', clients: 842, load: 86 },
  { id: '2', panel: 'Singapore-02', status: 'ACTIVE', clients: 311, load: 64 },
  { id: '3', panel: 'Tokyo-03', status: 'REAUTH_REQUIRED', clients: 128, load: 42 },
]

const COLUMNS: Column<Row>[] = [
  { key: 'panel', header: 'Panel', render: (r) => <span className="ui-strong">{r.panel}</span> },
  {
    key: 'status',
    header: 'Status',
    render: (r) => (
      <Badge tone={r.status === 'ACTIVE' ? 'success' : 'warning'}>{r.status.replace('_', ' ')}</Badge>
    ),
  },
  { key: 'clients', header: 'Clients', align: 'right', render: (r) => r.clients },
  { key: 'load', header: 'Load', render: (r) => <Progress value={r.load} height={6} /> },
]

/**
 * Full-fidelity theme preview. The draft theme is scoped to THIS container
 * only — every `[data-theme]`, `[data-ui]` and CSS variable is set locally,
 * so the real app behind it is untouched until the user saves.
 */
export function ThemePreview({
  draft,
  onSave,
  onDiscard,
}: {
  draft: ThemeSettings
  onSave: () => void
  onDiscard: () => void
}) {
  const [tab, setTab] = useState('overview')
  const [on, setOn] = useState(true)
  const variant = resolveVariant(draft.preset, draft.variant)

  /* Draft tokens -> inline CSS variables on the preview shell. */
  const vars = useMemo(() => themeToCssVars(draft), [draft])
  const variantVars = variant?.vars ?? {}

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onDiscard()
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
        e.preventDefault()
        onSave()
      }
    }
    window.addEventListener('keydown', onKey)
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      window.removeEventListener('keydown', onKey)
      document.body.style.overflow = prev
    }
  }, [onDiscard, onSave])

  const style = { ...vars, ...variantVars } as React.CSSProperties

  return (
    <div
      className="preview-root"
      role="dialog"
      aria-modal="true"
      aria-label="Theme preview"
      data-theme={draft.preset}
      data-ui={draft.uiVariant}
      data-shell={draft.shell}
      data-scene={draft.scene}
      data-effect={draft.effectLevel}
      data-glass={draft.glassEffect ? 'on' : 'off'}
      style={style}
    >
      {/*
        The draft's own background. `ThemePreview` used to set `data-scene` and
        nothing else, so choosing a different effect changed an ATTRIBUTE and
        left the preview looking identical — the one control whose result a user
        most wants to see was the one control that showed nothing. `SceneLayer`
        is the same renderer the live app uses, fed the DRAFT, so what is
        previewed is exactly what Save will produce.
      */}
      <SceneLayer
        scene={draft.scene}
        effectLevel={draft.effectLevel}
        primary={draft.primary}
        secondary={draft.secondary}
        bgPage={draft.bgPage}
        speed={draft.sceneSpeed}
        lowResource={draft.lowResourceMode}
        reducedMotion={draft.reducedMotion}
      />

      {/* Preview action bar */}
      <div className="preview-bar">
        <span className="preview-bar__title">
          <Palette size={16} aria-hidden="true" />
          Previewing <strong>{PRESET_LABELS[draft.preset]}</strong>
          {variant && <span className="preview-bar__sub"> / {variant.name}</span>}
        </span>
        <div className="preview-bar__actions">
          <Button variant="ghost" size="sm" leading={<X size={14} />} onClick={onDiscard}>
            Discard
          </Button>
          <Button size="sm" leading={<Check size={14} />} onClick={onSave}>
            Save &amp; lock
          </Button>
        </div>
      </div>

      {/* Themed app mock */}
      <div className="preview-shell">
        {/* Mini sidebar */}
        <aside className="preview-sidebar glass">
          <div className="preview-sidebar__brand">
            <VeylixLogo size={26} />
            <span>VEYLIX</span>
          </div>
          <nav className="preview-nav">
            {[
              { icon: LayoutDashboard, label: 'Dashboard', count: 0 },
              { icon: Server, label: 'Servers', count: 30 },
              { icon: Package, label: 'VPN Offers', count: 12 },
              { icon: UserCog, label: 'Admins', count: 8 },
              { icon: CreditCard, label: 'Payments', count: 0 },
            ].map(({ icon: Icon, label, count }, i) => (
              <span key={label} className={`preview-nav__item${i === 0 ? ' preview-nav__item--active' : ''}`}>
                <Icon size={15} aria-hidden="true" />
                <span className="preview-nav__label">{label}</span>
                {count > 0 && <span className="app-nav__count">{count}</span>}
              </span>
            ))}
          </nav>
          <div className="preview-sidebar__foot">
            <span className="app-avatar" aria-hidden="true">S</span>
            <span>
              <strong>Super Admin</strong>
              <small>Platform owner</small>
            </span>
          </div>
        </aside>

        {/* Mini content */}
        <main className="preview-main">
          <header className="preview-topbar glass">
            <span className="preview-search">
              <Search size={14} aria-hidden="true" />
              Search pages, actions…
              <kbd>Ctrl K</kbd>
            </span>
            <span className="preview-topbar__right">
              <span className="preview-icon-btn" aria-hidden="true">
                <Bell size={16} />
                <span className="preview-badge">2</span>
              </span>
              <span className="app-topbar__user">
                <span className="app-avatar" aria-hidden="true">S</span>
                <span className="app-topbar__user-name">Super Admin</span>
              </span>
            </span>
          </header>

          <div className="preview-content">
            <div className="preview-pagehead">
              <div>
                <h2>Overview</h2>
                <p className="ui-muted">This is how the app will look with this theme</p>
              </div>
              <div className="ui-row">
                <Button size="sm" variant="outline">Refresh</Button>
                <Button size="sm">New server</Button>
              </div>
            </div>

            <div className="ui-grid preview-grid">
              {[
                { label: 'Servers', value: '24 / 30' },
                { label: 'Active clients', value: '1,284' },
                { label: 'Free offers', value: '3' },
                { label: 'Healthy', value: '80%' },
              ].map((m) => (
                <div className="dash-3d" key={m.label}>
                  <div className="dash-3d__surface">
                    <div className="ui-metric">
                      <span className="ui-metric__label">{m.label}</span>
                      <span className="ui-metric__value">{m.value}</span>
                      <span className="ui-metric__chart">
                        <Sparkline values={[12, 18, 14, 22, 19, 27, 31]} />
                      </span>
                    </div>
                  </div>
                </div>
              ))}
            </div>

            <div className="preview-charts">
              <Card title="Client traffic" actions={<Badge tone="success">Live</Badge>}>
                <LineChart
                  series={[
                    { label: 'Clients', values: [820, 910, 880, 1040, 1180, 1240, 1284] },
                    { label: 'Sessions', values: [420, 505, 470, 610, 700, 760, 812] },
                  ]}
                  labels={['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']}
                  height={190}
                />
              </Card>
              <Card title="Network health">
                <DonutChart
                  segments={[{ label: 'Healthy', value: 80 }, { label: 'Attention', value: 20 }]}
                  size={130}
                  thickness={18}
                  centerLabel="80%"
                />
                <div className="ui-mt-1">
                  <Progress value={86} label="Bandwidth" />
                  <Progress value={42} label="Storage" tone="warning" />
                </div>
              </Card>
            </div>

            <Card title="Server integration status">
              <Table columns={COLUMNS} data={ROWS} rowKey={(r) => r.id} />
            </Card>

            <Card title="Components">
              <div className="preview-samples">
                <div className="ui-row">
                  <Button size="sm">Primary</Button>
                  <Button size="sm" variant="secondary">Secondary</Button>
                  <Button size="sm" variant="outline">Outline</Button>
                  <Button size="sm" variant="ghost">Ghost</Button>
                  <Button size="sm" variant="danger">Danger</Button>
                </div>
                <div className="ui-row">
                  <Badge tone="success" dot>Active</Badge>
                  <Badge tone="warning" dot>Pending</Badge>
                  <Badge tone="error" dot>Expired</Badge>
                  <Badge tone="info">Info</Badge>
                  <Badge tone="primary">Primary</Badge>
                </div>
                <Input label="Sample input" placeholder="Type here…" />
                <Switch checked={on} onChange={setOn} label="Enable auto-reconnect" />
                <Tabs
                  items={[
                    { value: 'overview', label: 'Overview' },
                    { value: 'network', label: 'Network' },
                    { value: 'billing', label: 'Billing' },
                  ]}
                  value={tab}
                  onChange={setTab}
                />
              </div>
            </Card>
          </div>
        </main>
      </div>
    </div>
  )
}