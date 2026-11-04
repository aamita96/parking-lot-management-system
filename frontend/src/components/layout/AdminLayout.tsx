import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { BadgeIndianRupee, Gamepad2, KeyRound, LayoutGrid, LogOut, ScrollText, ShieldAlert, Ticket, Users } from 'lucide-react'
import type { ReactNode } from 'react'
import { api } from '@/api/client'
import type { Exceptions } from '@/api/types'
import { Logo } from '@/components/brand/Logo'
import { Button } from '@/components/ui'
import { keys } from '@/lib/queryClient'
import { useAdminAuth } from '@/stores/admin'

// Sidebar + page title around every admin page. Each admin page wraps its content in
//   <AdminLayout title="Tickets" description="...">...</AdminLayout>

const menu = [
  { to: '/admin/simulator', label: 'Simulator', icon: Gamepad2 },
  { to: '/admin/exceptions', label: 'Exceptions', icon: ShieldAlert },
  { to: '/admin/tickets', label: 'Tickets', icon: Ticket },
  { to: '/admin/vehicles', label: 'Owners & vehicles', icon: Users },
  { to: '/admin/spots', label: 'Spots', icon: LayoutGrid },
  { to: '/admin/pricing', label: 'Pricing', icon: BadgeIndianRupee },
  { to: '/admin/gates', label: 'Gate devices', icon: KeyRound },
  { to: '/admin/audit', label: 'Audit log', icon: ScrollText },
] as const

export function AdminLayout({ title, description, actions, children }: { title: string; description?: string; actions?: ReactNode; children: ReactNode }) {
  const admin = useAdminAuth((s) => s.admin)
  const navigate = useNavigate()

  // Red counter next to "Exceptions" in the menu.
  const exceptions = useQuery({
    queryKey: keys.exceptions,
    queryFn: () => api<Exceptions>('/admin/exceptions', { auth: 'admin' }),
    refetchInterval: 15000,
  })
  const openExceptions = exceptions.data?.counts.total ?? 0

  async function signOut() {
    await fetch('/api/v1/admin/auth/logout', { method: 'POST' }) // clears the login cookie
    useAdminAuth.getState().signOut()
    navigate({ to: '/admin/login' })
  }

  return (
    <div className="flex min-h-screen bg-slate-100">
      <aside className="sticky top-0 hidden h-screen w-60 shrink-0 flex-col border-r border-slate-200 bg-white lg:flex">
        <Link to="/" className="px-4 py-4">
          <Logo subtitle="Admin console" />
        </Link>
        <nav className="flex-1 space-y-0.5 px-2" aria-label="Admin">
          {menu.map(({ to, label, icon: Icon }) => (
            <Link
              key={to}
              to={to}
              className="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium text-slate-600 hover:bg-slate-100 hover:text-slate-900 [&.active]:bg-brand-50 [&.active]:text-brand-800"
            >
              <Icon className="size-4" aria-hidden />
              <span className="flex-1">{label}</span>
              {to === '/admin/exceptions' && openExceptions > 0 && (
                <span className="rounded-full bg-red-600 px-1.5 text-[11px] font-bold text-white">{openExceptions}</span>
              )}
            </Link>
          ))}
        </nav>
        <div className="border-t border-slate-200 p-3">
          <p className="truncate px-1 text-xs text-slate-500">{admin?.email}</p>
          <Button variant="ghost" size="small" className="mt-1 w-full justify-start" onClick={signOut}>
            <LogOut /> Sign out
          </Button>
        </div>
      </aside>

      <div className="min-w-0 flex-1 pb-8">
        {/* Small screens: the menu becomes a scrolling row of links. */}
        <div className="flex items-center gap-1 overflow-x-auto border-b border-slate-200 bg-white px-2 py-2 lg:hidden">
          {menu.map(({ to, label }) => (
            <Link key={to} to={to} className="whitespace-nowrap rounded-md px-2.5 py-1.5 text-xs font-medium text-slate-600 [&.active]:bg-brand-50 [&.active]:text-brand-800">
              {label}
            </Link>
          ))}
        </div>

        <div className="flex flex-wrap items-end justify-between gap-3 px-6 pb-4 pt-6">
          <div>
            <h1 className="text-2xl font-bold text-slate-900">{title}</h1>
            {description && <p className="mt-0.5 text-sm text-slate-500">{description}</p>}
          </div>
          {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
        </div>
        <div className="space-y-5 px-6">{children}</div>
      </div>
    </div>
  )
}
