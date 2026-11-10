import { useEffect, useState } from 'react'
import {
  Button, Card, Badge, Input, Select, MetricCard, StatusBadge, Table, Switch, Tabs,
  Dialog, useToast, PageHeader, type Column,
} from '../components/ui'
import { useThemeStore } from '../theme/themeStore'
import {
  PRESET_LABELS,
  SCENE_LABELS,
  UI_VARIANT_LABELS,
  PRESET_COLORS,
  type ThemePreset,
} from '../theme/themes'
import { Server, Users, ShieldCheck, Wifi, Monitor, Sparkles, Palette, LayoutPanelTop } from 'lucide-react'
import { VeylixLogo } from '../components/brand'
import { LineChart, BarChart, DonutChart, Sparkline, Progress } from '../components/charts'
import './sample.css'
import { SubThemePicker } from '../components/theme-studio/SubThemePicker'

const PRESETS: ThemePreset[] = ['modern', 'dark', 'light', 'classic', 'gaming']

interface Row {
  id: string
  name: string
  status: string
  clients: number
}

const ROWS: Row[] = [
  { id: '1', name: 'Panel Frankfurt-01', status: 'ACTIVE', clients: 842 },
  { id: '2', name: 'Panel Singapore-02', status: 'REAUTH_REQUIRED', clients: 311 },
  { id: '3', name: 'Panel Tokyo-03', status: 'CONNECTION_UNAVAILABLE', clients: 0 },
]

const COLUMNS: Column<Row>[] = [
  { key: 'name', header: 'Panel', render: (r) => <span className="ui-strong">{r.name}</span> },
  { key: 'status', header: 'Status', render: (r) => <StatusBadge status={r.status} /> },
  { key: 'clients', header: 'Clients', render: (r) => r.clients },
]

/** Sticky theme switcher so every UI language can be compared in one click. */
function ThemeBar() {
  const { draft, settings: live, draftSetPreset } = useThemeStore()
  const settings = draft ?? live
  return (
    <div className="sample-bar">
      <span className="sample-bar__label">Preset</span>
      {PRESETS.map((p) => (
        <button
          key={p}
          className={`sample-bar__btn ${settings.preset === p ? 'sample-bar__btn--on' : ''}`}
          onClick={() => draftSetPreset(p)}
        >
          <span className="sample-bar__dot" style={{ background: PRESET_COLORS[p].primary }} />
          {PRESET_LABELS[p]}
        </button>
      ))}
      <span className="sample-bar__meta">
        {UI_VARIANT_LABELS[settings.uiVariant]} · {SCENE_LABELS[settings.scene]}
      </span>
    </div>
  )
}

/**
 * Visual QA page: every primitive, live, under the active theme.
 * Visit /sample to compare the five UI languages side by side.
 */
export function SamplePage() {
  const { draft, settings: live, draftSetPreset } = useThemeStore()
  const settings = draft ?? live
  const [on, setOn] = useState(true)
  const [tab, setTab] = useState('general')
  const [dialog, setDialog] = useState(false)
  const toast = useToast()

  /** Live swatches straight from the active theme settings — never stale. */
  const swatches: [string, string][] = [
    ['Primary', settings.primary],
    ['Secondary', settings.secondary],
    ['Page', settings.bgPage],
    ['Sidebar', settings.bgSidebar],
    ['Card', settings.bgCard],
    ['Border', settings.border],
    ['Success', settings.success],
    ['Warning', settings.warning],
    ['Error', settings.error],
    ['Info', settings.info],
  ]

  // Let ?theme=gaming deep-link straight to a preset (handy for screenshots).
  useEffect(() => {
    const q = new URLSearchParams(window.location.search).get('theme') as ThemePreset | null
    if (q && PRESETS.includes(q)) draftSetPreset(q)
  }, [draftSetPreset])

  return (
    <div className="sample" data-testid="sample">
      <ThemeBar />

      <PageHeader
        title="Theme showcase — new palettes"
        subtitle={
          <>
            Active: <strong>{PRESET_LABELS[settings.preset]}</strong> — {UI_VARIANT_LABELS[settings.uiVariant]} /{' '}
            {SCENE_LABELS[settings.scene]} / shell "{settings.shell}"
          </>
        }
        actions={<Badge tone="info"><Sparkles size={12} /> {settings.uiVariant}</Badge>}
      />

      {/* New-palette hero: brand + live gradient of the active accents */}
      <section className="sample-hero">
        <div className="sample-hero__brand">
          <VeylixLogo size={44} showText />
          <p>
            Rebuilt palettes — vibrant accents, tinted surfaces and aurora gradients.
            Use the preset bar above to flip through all five looks; every swatch below is live.
          </p>
        </div>
        <div className="sample-hero__chips">
          <span className="sample-chip sample-chip--a">{settings.primary}</span>
          <span className="sample-chip sample-chip--b">{settings.secondary}</span>
        </div>
      </section>

      {/* Live palette: the ten tunable colours straight from the active theme */}
      <Card
        title="Live palette"
        actions={<Badge tone="primary"><Palette size={12} /> {PRESET_LABELS[settings.preset]}</Badge>}
      >
        <div className="sample-swatches">
          {swatches.map(([label, value]) => (
            <div key={label} className="sample-swatch">
              <span className="sample-swatch__chip" style={{ background: value }} />
              <span className="sample-swatch__label">{label}</span>
              <span className="sample-swatch__hex">{value || '—'}</span>
            </div>
          ))}
        </div>
      </Card>

      <div className="ui-grid">
        <MetricCard label="Servers" value="24 / 30" delta="2 need attention" deltaDirection="down" icon={<Server size={18} />} />
        <MetricCard label="Active clients" value="1,284" delta="12 this week" deltaDirection="up" icon={<Users size={18} />} />
        <MetricCard label="Free offers" value="3" icon={<Wifi size={18} />} />
        <MetricCard label="Healthy" value="80%" delta="All synced" deltaDirection="up" icon={<ShieldCheck size={18} />} />
      </div>

      <div className="sample__cols">
        <Card title="Buttons">
          <div className="ui-row">
            <Button>Primary</Button>
            <Button variant="secondary">Secondary</Button>
            <Button variant="outline">Outline</Button>
            <Button variant="ghost">Ghost</Button>
            <Button variant="danger">Delete</Button>
            <Button loading>Loading</Button>
          </div>
          <div className="ui-row flow-top-3">
            <Button size="sm">Small</Button>
            <Button size="md">Medium</Button>
            <Button size="lg">Large</Button>
          </div>
        </Card>

        <Card title="Inputs">
          <Input label="Server name" placeholder="frankfurt-01" hint="Lowercase, no spaces" />
          <Select label="Region" defaultValue="eu">
            <option value="eu">Europe</option>
            <option value="ap">Asia Pacific</option>
          </Select>
          <Input label="API key" error="This field is required" placeholder="••••••••" />
        </Card>

        <Card title="Badges & tabs">
          <div className="ui-row">
            <Badge tone="success">Active</Badge>
            <Badge tone="warning">Pending</Badge>
            <Badge tone="error">Expired</Badge>
            <Badge tone="info">Info</Badge>
            <Badge tone="primary">Primary</Badge>
            <Badge>Default</Badge>
          </div>
          <div style={{ marginTop: 14 }}>
            <Tabs
              items={[
                { value: 'general', label: 'General' },
                { value: 'network', label: 'Network' },
                { value: 'billing', label: 'Billing' },
              ]}
              value={tab}
              onChange={setTab}
            />
          </div>
          <div style={{ marginTop: 14 }}>
            <Switch checked={on} onChange={setOn} label="Enable auto-reconnect" />
          </div>
        </Card>
      </div>

      <div className="sample__cols">
        <Card title="Notifications — toasts">
          <p className="ui-muted flow-top-0">
            Toast style follows the theme (bar / line / badge — Theme Studio → Components).
          </p>
          <div className="ui-row">
            <Button size="sm" variant="outline" onClick={() => toast('success', 'Server synced', 'Frankfurt-01 is back online')}>Success</Button>
            <Button size="sm" variant="outline" onClick={() => toast('info', 'Sync queued', 'Singapore-02 update scheduled')}>Info</Button>
            <Button size="sm" variant="outline" onClick={() => toast('warning', 'Traffic 80%', 'Panel Tokyo-03 near quota')}>Warning</Button>
            <Button size="sm" variant="outline" onClick={() => toast('error', 'Reauth required', 'Credentials rejected by panel')}>Error</Button>
          </div>
        </Card>
        <Card title="Dialog & overlay">
          <p className="ui-muted flow-top-0">
            Glass dialog surfaces — blur, edge light and shadow all come from the active theme.
          </p>
          <div className="ui-row">
            <Button leading={<LayoutPanelTop size={15} />} onClick={() => setDialog(true)}>Open dialog</Button>
          </div>
        </Card>
      </div>

      <Card title="Shell polish — avatar, counts, indicator">
        <div className="ui-row" style={{ marginBottom: 12 }}>
          <span className="app-topbar__user">
            <span className="app-avatar" aria-hidden="true">S</span>
            <span className="app-topbar__user-name">Super Admin</span>
          </span>
          <span className="app-nav__count" style={{ marginLeft: 0 }}>30</span>
          <span
            className="app-nav__count"
            style={{ marginLeft: 0, color: 'var(--c-primary)', background: 'color-mix(in srgb, var(--c-primary) 16%, transparent)' }}
          >
            99+
          </span>
          <span style={{ position: 'relative' }} aria-hidden="true">
            <kbd className="app-search__kbd">Ctrl K</kbd>
          </span>
        </div>
        <div
          className="app-user"
          style={{ maxWidth: 240, padding: '6px 8px', borderRadius: 'var(--radius-sm)', border: '1px solid var(--border)' }}
        >
          <span className="app-user__avatar" aria-hidden="true">S</span>
          <span className="app-user__meta">
            <span className="app-user__name">Super Admin</span>
            <span className="app-user__role">Platform owner</span>
          </span>
        </div>
        <p className="ui-muted t-sm" style={{ margin: '10px 0 0' }}>
          Same classes the real shell uses: sidebar count chips, active-item indicator, topbar avatar, search kbd hint —
          all theme-driven.
        </p>
      </Card>

      <div className="sample__cols">
        <Card title="Traffic (7 days)">
          <LineChart
            series={[{ label: 'Traffic', values: [12, 26, 18, 34, 28, 44, 38] }]}
            labels={['M', 'T', 'W', 'T', 'F', 'S', 'S']}
            height={150}
          />
        </Card>
        <Card title="Clients per panel">
          <BarChart
            data={[{ label: 'FRA', value: 842 }, { label: 'SIN', value: 311 }, { label: 'TYO', value: 128 }, { label: 'NYC', value: 470 }]}
            height={150}
          />
        </Card>
        <Card title="Capacity">
          <div className="ui-row" style={{ gap: 18 }}>
            <DonutChart
              segments={[{ label: 'Used', value: 68 }, { label: 'Free', value: 32 }]}
              size={132}
              thickness={18}
              centerLabel="68%"
            />
            <div className="ui-col" style={{ flex: 1 }}>
              <Progress value={68} label="Bandwidth" />
              <Progress value={42} label="Storage" tone="warning" />
              <Progress value={91} label="Sessions" tone="success" />
            </div>
          </div>
          <div className="ui-row flow-top" style={{ gap: 8 }}>
            <span className="ui-muted">Trend</span>
            <Sparkline values={[8, 14, 11, 19, 16, 24, 21]} />
          </div>
        </Card>
      </div>

      <Card title="Server integration status" actions={<Button size="sm" variant="outline">Refresh</Button>}>
        <Table columns={COLUMNS} data={ROWS} rowKey={(r) => r.id} />
      </Card>

      <Card title={"Sub-themes - " + PRESET_LABELS[settings.preset]}>
        <p className="ui-muted flow-top-0">
          Each main theme ships its own variants. Picking one re-skins cards, buttons, notifications, badges and charts at once.
        </p>
        <SubThemePicker preset={settings.preset} />
      </Card>

      <Card title="Switch theme — UI language picker">
        <div className="ui-grid">
          {PRESETS.map((p) => (
            <button
              key={p}
              className="sample-switch"
              onClick={() => draftSetPreset(p)}
              type="button"
            >
              <span className="sample-switch__icon">
                <Monitor size={16} />
              </span>
              <span className="ui-strong">{PRESET_LABELS[p]}</span>
              <span className="ui-muted t-sm">
                {UI_VARIANT_LABELS[PRESET_COLORS[p].uiVariant ?? 'soft']}
              </span>
            </button>
          ))}
        </div>
      </Card>

      <Dialog
        open={dialog}
        onClose={() => setDialog(false)}
        title="Confirm panel restart"
        footer={
          <>
            <Button variant="ghost" onClick={() => setDialog(false)}>Cancel</Button>
            <Button
              onClick={() => {
                setDialog(false)
                toast('success', 'Restart queued', 'Frankfurt-01 is restarting…')
              }}
            >
              Restart
            </Button>
          </>
        }
      >
        <p className="flow-top-0">
          Sample dialog using the active theme's glass tokens. Accent:{' '}
          <strong style={{ color: 'var(--c-primary)' }}>{settings.primary}</strong>
        </p>
      </Dialog>
    </div>
  )
}


