import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { api, queryString } from '@/api/client'
import type { AdminAction, Page } from '@/api/types'
import { AdminLayout } from '@/components/layout/AdminLayout'
import { Badge, Card, MessageRow, Pager, Table } from '@/components/ui'
import { dateTimeIST } from '@/lib/format'
import { keys } from '@/lib/queryClient'
import { requireAdmin } from '@/lib/requireAdmin'

// URL: /admin/audit?page=2 — who did what, when and why (FR-40).
export const Route = createFileRoute('/admin/audit')({
  validateSearch: (search: Record<string, unknown>): { page?: number } => ({
    page: typeof search.page === 'number' ? search.page : undefined,
  }),
  beforeLoad: requireAdmin,
  component: AuditPage,
})

/** Turns { count: 6, row: "C" } into "count=6 · row=C". */
function describe(details: Record<string, unknown>): string {
  return Object.entries(details)
    .filter(([, value]) => value !== '' && value !== null && value !== undefined)
    .map(([key, value]) => `${key}=${typeof value === 'object' ? JSON.stringify(value) : value}`)
    .join(' · ')
}

function AuditPage() {
  const { page = 1 } = Route.useSearch()
  const navigate = Route.useNavigate()
  const actions = useQuery({
    queryKey: [...keys.audit, page],
    queryFn: () => api<Page<AdminAction>>(`/admin/audit${queryString({ page, pageSize: 50 })}`, { auth: 'admin' }),
  })

  return (
    <AdminLayout title="Audit log" description="Every admin action with who, when and why. Public disputes appear here too.">
      <Card>
        <Table>
          <thead>
            <tr>
              <th>When</th>
              <th>Who</th>
              <th>Action</th>
              <th>Target</th>
              <th>Why</th>
              <th>Details</th>
            </tr>
          </thead>
          <tbody>
            {actions.isLoading && <MessageRow columns={6} loading />}
            {actions.data?.items.length === 0 && <MessageRow columns={6}>No actions recorded yet.</MessageRow>}
            {actions.data?.items.map((action) => (
              <tr key={action.id}>
                <td className="whitespace-nowrap">{dateTimeIST(action.createdAt)}</td>
                <td>{action.adminEmail ?? <Badge color="amber">Public</Badge>}</td>
                <td className="font-mono text-xs font-semibold">{action.action}</td>
                <td>
                  <Badge>{action.targetType}</Badge>
                </td>
                <td>{action.reason ?? '—'}</td>
                <td>
                  <code className="block max-w-md truncate text-[11px] text-slate-500" title={JSON.stringify(action.details)}>
                    {describe(action.details)}
                  </code>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        {actions.data && <Pager page={page} pageSize={50} total={actions.data.total} onPage={(p) => navigate({ search: { page: p } })} />}
      </Card>
    </AdminLayout>
  )
}
