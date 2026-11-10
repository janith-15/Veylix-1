import { useEffect, useState, type FormEvent } from 'react'
import {
  AsyncState,
  Button,
  Card,
  Dialog,
  EmptyState,
  Input,
  PageHeader,
  Skeleton,
  StatusBadge,
  Table,
  useToast,
  type Column,
} from '../components/ui'
import { Plus, Server as ServerIcon, UserCog } from 'lucide-react'
import { useAdminsStore, type AdminAccount } from '../stores/adminsStore'
import { useServersStore } from '../stores/serversStore'
import { passwordHint, passwordViolations } from '../lib/passwordPolicy'
import './admins.css'

/**
 * Admin (reseller) management — real API (`GET/POST/PATCH /api/admins`).
 *
 * Phase 5 changes, all forced by what the backend can actually confirm:
 *  - the "Assigned servers" column is gone (no assignment endpoint yet, P5);
 *    showing "0 assigned" would read as "this reseller has no servers" when the
 *    truth is "nobody can assign one yet";
 *  - the quota column shows the ceiling only — `GET /api/admins` does not return
 *    usage, so a progress bar would be invented;
 *  - a password field was added, because `POST /api/admins` requires one;
 *  - "remove" is gone: removal is `SUSPENDED` by design (the audit trail and any
 *    VPNs they sold keep a valid owner).
 */
export function AdminsPage() {
  const admins = useAdminsStore((s) => s.admins)
  const loading = useAdminsStore((s) => s.loading)
  const error = useAdminsStore((s) => s.error)
  const busy = useAdminsStore((s) => s.busy)
  const load = useAdminsStore((s) => s.load)
  const addAdmin = useAdminsStore((s) => s.addAdmin)
  const setStatus = useAdminsStore((s) => s.setStatus)
  const setQuota = useAdminsStore((s) => s.setQuota)
  const adminServers = useAdminsStore((s) => s.adminServers)
  const setAdminServers = useAdminsStore((s) => s.setAdminServers)
  const toast = useToast()

  const [formOpen, setFormOpen] = useState(false)
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [quotaMax, setQuotaMax] = useState('10')
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  // Server assignments, loaded lazily per admin (one GET each when the dialog opens).
  const [scopeTarget, setScopeTarget] = useState<AdminAccount | null>(null)
  const [scopeIds, setScopeIds] = useState<number[]>([])
  const [scopeSaving, setScopeSaving] = useState(false)

  const servers = useServersStore((s) => s.servers)
  const loadServers = useServersStore((s) => s.load)

  useEffect(() => {
    void load()
    void loadServers()
  }, [load, loadServers])

  const openScope = async (a: AdminAccount) => {
    setScopeTarget(a)
    setScopeIds([])
    try {
      setScopeIds(await adminServers(a.apiId))
    } catch {
      toast('error', 'Could not read assignments', 'Showing an empty set')
    }
  }

  const saveScope = async () => {
    if (!scopeTarget) return
    setScopeSaving(true)
    try {
      await setAdminServers(scopeTarget.apiId, scopeIds)
      toast('success', 'Servers assigned', `${scopeIds.length} server(s) for ${scopeTarget.name}`)
      setScopeTarget(null)
    } catch {
      toast('error', 'Could not save', 'The backend rejected the assignment')
    } finally {
      setScopeSaving(false)
    }
  }

  const saveForm = async (e: FormEvent) => {
    e.preventDefault()
    setFormError('')
    if (!name.trim() || !email.trim()) {
      setFormError('Name and email are required.')
      return
    }
    // The backend enforces the same policy; checking here avoids a pointless
    // round trip and gives the user the reason immediately.
    const problems = passwordViolations(password)
    if (problems.length > 0) {
      setFormError(`Password ${problems.join(', ')}`)
      return
    }
    setSaving(true)
    try {
      await addAdmin({
        name: name.trim(),
        email: email.trim(),
        password,
        quotaMax: Math.max(0, Number.parseInt(quotaMax, 10) || 0),
      })
      toast('success', 'Admin created', `${name.trim()} can sign in now`)
      setFormOpen(false)
      setName('')
      setEmail('')
      setPassword('')
      setQuotaMax('10')
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Could not create the admin')
    } finally {
      setSaving(false)
    }
  }

  const COLUMNS: Column<(typeof admins)[number]>[] = [
    {
      key: 'name',
      header: 'Admin',
      sortable: true,
      render: (a) => (
        <span>
          <span className="ui-strong">{a.name}</span>
          <span className="adm-email">{a.email}</span>
        </span>
      ),
    },
    {
      key: 'status',
      header: 'Status',
      sortable: true,
      render: (a) => <StatusBadge status={a.status} />,
    },
    {
      key: 'quotaMax',
      header: 'Quota ceiling',
      sortable: true,
      render: (a) => (
        <span className="ui-muted">{a.quotaMax > 0 ? `${a.quotaMax} assignments` : 'unlimited'}</span>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (a) => (
        <div className="sv-actions">
          <Button
            size="sm"
            variant="ghost"
            leading={<ServerIcon size={14} />}
            onClick={() => void openScope(a)}
          >
            Servers
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={busy === a.apiId}
            onClick={() =>
              void setQuota(a.apiId, a.quotaMax > 0 ? a.quotaMax + 5 : 10)
                .then(() => toast('success', 'Quota raised', `${a.name} → +5`))
                .catch(() => toast('error', 'Could not change the quota', 'Nothing was saved'))
            }
          >
            +5 quota
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={busy === a.apiId}
            onClick={() => {
              const next = a.status === 'ACTIVE' ? 'SUSPENDED' : 'ACTIVE'
              void setStatus(a.apiId, next)
                .then(() =>
                  toast(
                    next === 'SUSPENDED' ? 'warning' : 'success',
                    `Admin ${next.toLowerCase()}`,
                    next === 'SUSPENDED' ? `${a.name} — live sessions revoked` : a.name,
                  ),
                )
                .catch(() => toast('error', 'Could not change the status', 'Nothing was saved'))
            }}
          >
            {a.status === 'ACTIVE' ? 'Suspend' : 'Activate'}
          </Button>
        </div>
      ),
    },
  ]

  return (
    <div className="dash" data-testid="admins-page">
      <PageHeader
        title="Admin management"
        subtitle="Resellers — account status and how much of their quota is in use"
        actions={
          <Button size="sm" leading={<Plus size={14} />} onClick={() => setFormOpen(true)}>
            Add admin
          </Button>
        }
      />

      <Card
        title="Admins"
        actions={<span className="ui-muted t-sm">{admins.filter((a) => a.status === 'ACTIVE').length} active</span>}
      >
        <AsyncState
          status={error ? 'error' : loading ? 'loading' : admins.length === 0 ? 'empty' : 'ready'}
          error={error}
          onRetry={() => void load()}
          skeleton={<Skeleton height={180} />}
          empty={
            <EmptyState
              icon={<UserCog size={22} />}
              title="No resellers yet"
              description="Create the first reseller with “New admin” — they sell plans on the servers you assign."
              action={
                <Button size="sm" variant="primary" onClick={() => setFormOpen(true)}>
                  New admin
                </Button>
              }
            />
          }
        >
          <Table columns={COLUMNS} data={admins} rowKey={(r) => r.id} initialSort={{ key: 'name', dir: 'asc' }} />
        </AsyncState>
      </Card>

      <Dialog
        open={formOpen}
        onClose={() => setFormOpen(false)}
        title="Add admin"
        footer={
          <>
            <Button variant="ghost" onClick={() => setFormOpen(false)}>Cancel</Button>
            <Button type="submit" form="adm-form" disabled={saving}>
              {saving ? 'Creating…' : 'Create admin'}
            </Button>
          </>
        }
      >
        <form id="adm-form" onSubmit={saveForm} className="ui-col" noValidate>
          <Input label="Full name" placeholder="Nimal Silva" value={name} onChange={(e) => setName(e.target.value)} />
          <Input
            label="Email"
            type="email"
            autoComplete="off"
            placeholder="admin@resell.veylix"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
          <Input
            label="Initial password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          {password.length > 0 && (
            <span className="ui-muted t-sm">{passwordHint(password)}</span>
          )}
          <Input
            label="Quota — max active premium assignments (0 = unlimited)"
            type="number"
            min={0}
            value={quotaMax}
            onChange={(e) => setQuotaMax(e.target.value)}
          />
          {formError && (
            <p className="ui-field__error" role="alert">
              {formError}
            </p>
          )}
        </form>
      </Dialog>
      <Dialog
        open={!!scopeTarget}
        onClose={() => setScopeTarget(null)}
        title={`Assigned servers — ${scopeTarget?.name ?? ''}`}
        footer={
          <>
            <Button variant="ghost" onClick={() => setScopeTarget(null)}>Cancel</Button>
            <Button onClick={() => void saveScope()} disabled={scopeSaving}>
              {scopeSaving ? 'Saving…' : 'Save assignment'}
            </Button>
          </>
        }
      >
        <div className="ui-col">
          <p className="ui-muted t-sm2" style={{ margin: 0 }}>
            A reseller only sees the panels ticked here. Saving replaces the whole
            set, so unticking a server really removes it.
          </p>
          {servers.length === 0 ? (
            <p className="ui-muted" style={{ margin: 0 }}>
              No panel integrations exist yet — add one under Servers first.
            </p>
          ) : (
            <div className="adm-pick">
              {servers.map((s) => (
                <label key={s.apiId} className="adm-pick__row">
                  <input
                    type="checkbox"
                    checked={scopeIds.includes(s.apiId)}
                    onChange={() =>
                      setScopeIds((cur) =>
                        cur.includes(s.apiId) ? cur.filter((x) => x !== s.apiId) : [...cur, s.apiId],
                      )
                    }
                    style={{ accentColor: 'var(--c-primary)' }}
                  />
                  <span>
                    {s.name}
                    {s.status !== 'ACTIVE' ? ` (${s.status.toLowerCase().replaceAll('_', ' ')})` : ''}
                  </span>
                </label>
              ))}
            </div>
          )}
          <span className="ui-muted t-sm">
            {scopeIds.length} selected
          </span>
        </div>
      </Dialog>
    </div>
  )
}