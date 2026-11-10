import { useEffect } from 'react'
import {
  AsyncState,
  Button,
  Card,
  EmptyState,
  PageHeader,
  Skeleton,
  StatusBadge,
  Table,
  type Column,
} from '../components/ui'
import { Server } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { useServersStore } from '../stores/serversStore'

/**
 * Servers visible to this reseller.
 *
 * Phase 5: the client-side `me.servers.includes(s.id)` filter is gone. The
 * backend already scopes `GET /api/servers` to the caller's role
 * (`store.ListServers` joins `admin_server_assignments`), so re-filtering here
 * with mock/admin data could only ever *hide* real servers or invent an
 * assignment. One authority, server-side.
 */
export function AdminServersPage() {
  const navigate = useNavigate()
  const servers = useServersStore((s) => s.servers)
  const loading = useServersStore((s) => s.loading)
  const error = useServersStore((s) => s.error)
  const load = useServersStore((s) => s.load)

  useEffect(() => {
    void load()
  }, [load])

  const visible = servers

  const COLUMNS: Column<(typeof visible)[number]>[] = [
    {
      key: 'name',
      header: 'Panel',
      sortable: true,
      render: (s) => (
        <span>
          <span className="ui-strong">{s.name}</span>
          <span className="sv-url">{s.panelUrl}</span>
        </span>
      ),
    },
    { key: 'status', header: 'Status', sortable: true, render: (s) => <StatusBadge status={s.status} /> },
    { key: 'version', header: 'Version', hideOnMobile: true },
    { key: 'clients', header: 'Clients', sortable: true, align: 'right' },
    { key: 'lastSync', header: 'Last sync' },
  ]

  return (
    <div className="dash" data-testid="admin-servers">
      <PageHeader
        title="Assigned servers"
        subtitle="The servers assigned to your account — anything else is not yours to sell"
      />
      <Card title="Your panels" actions={<span className="ui-muted t-sm">{visible.length} panels</span>}>
        <AsyncState
          status={error ? 'error' : loading ? 'loading' : visible.length === 0 ? 'empty' : 'ready'}
          error={error}
          onRetry={() => void load()}
          skeleton={<Skeleton height={160} />}
          empty={
            <EmptyState
              icon={<Server size={22} />}
              title="No panels assigned to you yet"
              description="A Super Admin assigns servers to your account. Until one is assigned, you have nothing to sell on."
              /*
               * A dead end needs a door: the person reading this cannot fix it
               * themselves, so the empty state links to the only place that can —
               * and refuses to pretend a button here would work.
               */
              action={
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => navigate('/chat')}
                >
                  Ask the Super Admin
                </Button>
              }
            />
          }
        >
          <Table columns={COLUMNS} data={visible} rowKey={(r) => r.id} />
        </AsyncState>
      </Card>
    </div>
  )
}