import { ChevronLeft, ChevronRight } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from './Button'
import { Spinner } from './Card'

/**
 * A styled HTML table. Write normal <thead>/<tbody>/<tr>/<th>/<td> inside it:
 *
 *   <Table>
 *     <thead><tr><th>Ticket</th><th>Vehicle</th></tr></thead>
 *     <tbody>{tickets.map((t) => <tr key={t.id}><td>{t.ticketNumber}</td><td>{t.vehicleNumber}</td></tr>)}</tbody>
 *   </Table>
 */
export function Table({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table
        className="w-full text-left text-sm
          [&_thead]:border-b [&_thead]:border-slate-200 [&_thead]:bg-slate-50/80
          [&_th]:whitespace-nowrap [&_th]:px-4 [&_th]:py-2.5 [&_th]:text-xs [&_th]:font-semibold [&_th]:uppercase [&_th]:tracking-wide [&_th]:text-slate-500
          [&_td]:px-4 [&_td]:py-2.5 [&_td]:align-top
          [&_tbody_tr]:border-b [&_tbody_tr]:border-slate-100"
      >
        {children}
      </table>
    </div>
  )
}

/** One full-width row for "loading" and "nothing found" messages. */
export function MessageRow({ columns, loading, children }: { columns: number; loading?: boolean; children?: ReactNode }) {
  return (
    <tr>
      <td colSpan={columns} className="py-10 text-center text-slate-500">
        {loading ? <Spinner /> : children}
      </td>
    </tr>
  )
}

/** "1–25 of 80" with previous/next buttons. */
export function Pager({ page, pageSize, total, onPage }: { page: number; pageSize: number; total: number; onPage: (page: number) => void }) {
  const pages = Math.max(1, Math.ceil(total / pageSize))
  const from = total === 0 ? 0 : (page - 1) * pageSize + 1
  const to = Math.min(total, page * pageSize)
  return (
    <div className="flex items-center justify-between gap-3 px-4 py-3 text-sm text-slate-500">
      <span className="tabular">
        {from}–{to} of {total}
      </span>
      <div className="flex items-center gap-1">
        <Button variant="ghost" size="icon" disabled={page <= 1} onClick={() => onPage(page - 1)} aria-label="Previous page">
          <ChevronLeft />
        </Button>
        <span className="tabular px-2">
          {page} / {pages}
        </span>
        <Button variant="ghost" size="icon" disabled={page >= pages} onClick={() => onPage(page + 1)} aria-label="Next page">
          <ChevronRight />
        </Button>
      </div>
    </div>
  )
}
