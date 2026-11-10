import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  AsyncState,
  Button,
  Card,
  Checkbox,
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
import { ArrowUpRight, Pencil, PlugZap, Plus, ShieldOff, Terminal, Trash2 } from 'lucide-react'
import { useServersStore, type ServerIntegration } from '../stores/serversStore'
import { InstallerDialog } from '../components/vpn/InstallerDialog'
import { ApiError } from '../lib/api'
import './servers.css'
import { activeTimeZone } from '../lib/time'

interface FormData {
  name: string
  panelUrl: string
  apiKey: string
  allowPrivateNetwork: boolean
}

const EMPTY_FORM: FormData = { name: '', panelUrl: '', apiKey: '', allowPrivateNetwork: false }

export function ServersPage() {
  const { servers, testing, loading, error, load, addServer, updateServer, testConnection, disableServer, removeServer } =
    useServersStore()
  const toast = useToast()
  const navigate = useNavigate()

  const [formOpen, setFormOpen] = useState(false)
  const [installerOpen, setInstallerOpen] = useState(false)
  /** null = "Add"; a row = "Edit" (the dialog is reused for both). */
  const [editTarget, setEditTarget] = useState<ServerIntegration | null>(null)
  const [removeTarget, setRemoveTarget] = useState<ServerIntegration | null>(null)
  const [form, setForm] = useState<FormData>(EMPTY_FORM)
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [removeError, setRemoveError] = useState('')

  /* Load the real integrations from the backend on mount. */
  useEffect(() => {
    void load()
  }, [load])

  const openAdd = () => {
    setEditTarget(null)
    setForm({ ...EMPTY_FORM, panelUrl: 'https://' })
    setFormError('')
    setFormOpen(true)
  }

  const openEdit = (s: ServerIntegration) => {
    setEditTarget(s)
    // The stored token can never be read back (D-016), so the field starts
    // empty and "leave blank to keep the current token" is the default.
    setForm({ name: s.name, panelUrl: s.panelUrl, apiKey: '', allowPrivateNetwork: s.allowPrivateNetwork })
    setFormError('')
    setFormOpen(true)
  }

  const saveForm = async (e: FormEvent) => {
    e.preventDefault()
    const editing = editTarget !== null
    const urlChanged = editing && form.panelUrl.trim() !== editTarget.panelUrl

    if (!form.name.trim()) {
      setFormError('Name is required.')
      return
    }
    if (!/^https:\/\/[^\s]+$/i.test(form.panelUrl.trim())) {
      setFormError('Panel URL must be a valid HTTPS address.')
      return
    }
    if (!form.apiKey.trim() && !editing) {
      setFormError('The panel API token is required — it is encrypted at rest and never shown again.')
      return
    }
    // The sealed ciphertext is bound to the URL, so a URL change is only
    // possible together with a fresh token (the backend enforces this too).
    if (urlChanged && !form.apiKey.trim()) {
      setFormError('Re-enter the panel API token when changing the panel URL.')
      return
    }

    const input = {
      name: form.name.trim(),
      panelUrl: form.panelUrl.trim(),
      token: form.apiKey.trim(),
      allowPrivateNetwork: form.allowPrivateNetwork,
    }

    setSaving(true)
    setFormError('')
    try {
      const row = editing ? await updateServer(editTarget.id, input) : await addServer(input)
      if (row.status === 'ACTIVE') {
        toast(
          'success',
          editing ? 'Server updated' : 'Panel connected',
          `${row.name} verified — 3X-UI ${row.version}`,
        )
      } else {
        toast(
          'error',
          editing ? 'Saved but not verified' : 'Panel added but not verified',
          `Status: ${row.status}. Check the URL, base path and token.`,
        )
      }
      setFormOpen(false)
      setEditTarget(null)
    } catch (err) {
      setFormError(err instanceof Error ? err.message : 'Could not save the integration.')
    } finally {
      setSaving(false)
    }
  }

  const confirmRemove = async () => {
    if (!removeTarget) return
    setRemoving(true)
    setRemoveError('')
    try {
      await removeServer(removeTarget.id)
      toast('success', 'Server deleted', `${removeTarget.name} was removed permanently`)
      setRemoveTarget(null)
    } catch (err) {
      // 409 server_in_use: VPN instances still point at it. Surfacing the
      // backend's reason is the whole point — silently closing the dialog
      // would claim a deletion that never happened.
      const msg =
        err instanceof ApiError
          ? err.message
          : err instanceof Error
            ? err.message
            : 'Could not delete the integration.'
      setRemoveError(msg)
      toast('error', 'Server not deleted', msg)
    } finally {
      setRemoving(false)
    }
  }


  const runTest = async (s: ServerIntegration) => {
    const result = await testConnection(s.id)
    if (result === 'ok') toast('success', 'Connection OK', `${s.name} authenticated against the live panel`)
    else toast('error', 'Connection failed', 'The panel rejected the token or could not be reached')
  }

  const counts = {
    active: servers.filter((s) => s.status === 'ACTIVE').length,
    attention: servers.filter((s) => s.status === 'REAUTH_REQUIRED' || s.status === 'CONNECTION_UNAVAILABLE').length,
    disabled: servers.filter((s) => s.status === 'DISABLED').length,
  }

  const COLUMNS: Column<ServerIntegration>[] = [
    {
      key: 'name',
      header: 'Panel',
      sortable: true,
      render: (s) => (
        // The whole name is a button: opening the editor must not depend on
        // finding a small icon at the far end of the row.
        <button
          type="button"
          className="sv-name"
          onClick={() => openEdit(s)}
          title={`Edit ${s.name}`}
          aria-label={`Edit ${s.name}`}
        >
          <span className="ui-strong">{s.name}</span>
          <span className="sv-url">{s.panelUrl}</span>
        </button>
      ),
    },
    { key: 'status', header: 'Status', sortable: true, render: (s) => <StatusBadge status={s.status} /> },
    { key: 'version', header: 'Panel', hideOnMobile: true, render: (s) => s.version },
    { key: 'xrayVersion', header: 'Xray', hideOnMobile: true, render: (s) => s.xrayVersion },
    {
      key: 'lastHealthCheck',
      header: 'Last check',
      hideOnMobile: true,
      render: (s) => (s.lastHealthCheck ? new Date(s.lastHealthCheck * 1000).toLocaleString(undefined, { timeZone: activeTimeZone() }) : 'never'),
    },
    {
      key: 'actions',
      header: 'Actions',
      align: 'right',
      sticky: true,
      render: (s) => (
        <div className="sv-actions">
          <Button
            size="sm"
            variant="outline"
            leading={<ArrowUpRight size={14} />}
            onClick={() => navigate(`/servers/${s.apiId}`)}
            title={`Open ${s.name} — its VPNs, owners and usage`}
          >
            Open
          </Button>
          <Button
            size="sm"
            variant="outline"
            leading={<Pencil size={14} />}
            onClick={() => openEdit(s)}
            title={`Edit ${s.name}`}
          >
            Edit
          </Button>
          <Button size="sm" variant="ghost" loading={testing === s.apiId} onClick={() => void runTest(s)}>
            Test
          </Button>
          {s.status === 'DISABLED' ? (
            <Button size="sm" variant="ghost" onClick={() => void runTest(s)} title={`Re-verify ${s.name}`}>
              <PlugZap size={14} /> Re-verify
            </Button>
          ) : (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                void disableServer(s.id)
                toast('info', 'Integration disabled', s.name)
              }}
              title={`Disable ${s.name}`}
            >
              <ShieldOff size={14} /> Disable
            </Button>
          )}
          <Button
            size="sm"
            variant="ghost"
            className="sv-danger"
            onClick={() => setRemoveTarget(s)}
            title={`Delete ${s.name}`}
          >
            <Trash2 size={14} /> Delete
          </Button>
        </div>
      ),
    },
  ]

  return (
    <div className="dash" data-testid="servers-page">
      <PageHeader
        title="Server integrations"
        subtitle="3X-UI panels — credentials are encrypted at rest and never returned by the API"
        actions={
          <>
            {/* No panel yet? The first step is installing the binary, not adding
                credentials for a server that does not exist. */}
            <Button
              size="sm"
              variant="outline"
              leading={<Terminal size={14} />}
              onClick={() => setInstallerOpen(true)}
            >
              Install on a server
            </Button>
            <Button size="sm" leading={<Plus size={14} />} onClick={openAdd}>
              Add server
            </Button>
          </>
        }
      />

      <div className="sv-strip">
        <div className="sv-chip" data-tone="success">
          <strong>{counts.active}</strong>
          <span>Active</span>
        </div>
        <div className="sv-chip" data-tone="warning">
          <strong>{counts.attention}</strong>
          <span>Need attention</span>
        </div>
        <div className="sv-chip">
          <strong>{counts.disabled}</strong>
          <span>Disabled</span>
        </div>
      </div>

      <Card title="Integrations" actions={<span className="ui-muted t-sm">{servers.length} panels</span>}>
        {/*
         * Phase D: the table rendered straight from the store, so a first load
         * showed an EMPTY list - which reads as "you have no panels" while the
         * request is still in flight - and a failed fetch showed the same empty
         * list with no explanation and no retry. AsyncState separates the three.
         */}
        <AsyncState
          status={loading ? 'loading' : error ? 'error' : servers.length === 0 ? 'empty' : 'ready'}
          error={error}
          onRetry={() => void load()}
          skeleton={<Skeleton height={180} />}
          empty={
            <EmptyState
              title="No panels yet"
              description="Add a 3X-UI panel to start managing servers. The URL must include the panel's web base path, and the API token is stored encrypted."
              action={
                <Button size="sm" leading={<Plus size={14} />} onClick={openAdd}>
                  Add server
                </Button>
              }
            />
          }
        >
          <Table columns={COLUMNS} data={servers} rowKey={(r) => r.id} />
        </AsyncState>
      </Card>

      {/* Add / Edit integration — one dialog, the mode decides the copy */}
      <Dialog
        open={formOpen}
        onClose={() => {
          setFormOpen(false)
          setEditTarget(null)
        }}
        title={editTarget ? `Edit ${editTarget.name}` : 'Add server integration'}
        footer={
          <>
            <Button
              variant="ghost"
              onClick={() => {
                setFormOpen(false)
                setEditTarget(null)
              }}
            >
              Cancel
            </Button>
            <Button type="submit" form="sv-form" loading={saving}>
              {editTarget ? 'Save changes' : 'Add integration'}
            </Button>
          </>
        }
      >
        <form id="sv-form" onSubmit={saveForm} className="ui-col" noValidate>
          <Input
            label="Name"
            placeholder="Panel Frankfurt-01"
            value={form.name}
            onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          />
          <Input
            label="Panel URL"
            placeholder="https://panel.example.com:2053/<web-base-path>"
            hint="HTTPS only, including the panel's web base path — validated and capability-tested before activation"
            value={form.panelUrl}
            onChange={(e) => setForm((f) => ({ ...f, panelUrl: e.target.value }))}
          />
          <Input
            label="API credentials"
            type="password"
            placeholder={editTarget ? 'Leave blank to keep the current token' : '••••••••'}
            hint={
              editTarget
                ? 'Write-only: the stored token is never returned. Enter a new one only to rotate it (required when the URL changes).'
                : 'Stored encrypted on the backend (AES-256-GCM) and never returned by any API response.'
            }
            autoComplete="new-password"
            value={form.apiKey}
            onChange={(e) => setForm((f) => ({ ...f, apiKey: e.target.value }))}
          />
          <Checkbox
            checked={form.allowPrivateNetwork}
            onChange={(v) => setForm((f) => ({ ...f, allowPrivateNetwork: v }))}
            label="Panel is on a private / LAN address (opt in to the private-network policy)"
          />
          {!form.allowPrivateNetwork && (
            <p className="ui-muted t-sm2" style={{ margin: 0 }}>
              Private, loopback and link-local addresses are rejected by the SSRF guard unless this is ticked.
            </p>
          )}
          {formError && (
            <p className="ui-field__error" role="alert">
              {formError}
            </p>
          )}
        </form>
      </Dialog>

      {/* Delete confirmation — permanent, and refused while VPNs still reference it */}
      <Dialog
        open={!!removeTarget}
        onClose={() => {
          setRemoveTarget(null)
          setRemoveError('')
        }}
        title="Delete this server permanently?"
        footer={
          <>
            <Button
              variant="ghost"
              onClick={() => {
                setRemoveTarget(null)
                setRemoveError('')
              }}
            >
              Cancel
            </Button>
            <Button variant="danger" loading={removing} onClick={() => void confirmRemove()}>
              Delete server
            </Button>
          </>
        }
      >
        <p className="flow-top-0">
          Delete <strong>{removeTarget?.name}</strong> ({removeTarget?.panelUrl})?
        </p>
        <p className="ui-muted t-sm2">
          This destroys the stored panel credentials and the cached inbounds on the backend. Remote
          accounts in 3X-UI are <strong>not</strong> touched.
        </p>
        <p className="ui-muted t-sm2">
          If any VPN instance still references this server the deletion is refused — revoke or move
          those VPNs first so client history is never lost.
        </p>
        {removeError && (
          <p className="ui-field__error" role="alert">
            {removeError}
          </p>
        )}
      </Dialog>

      <InstallerDialog open={installerOpen} onClose={() => setInstallerOpen(false)} />
    </div>
  )
}