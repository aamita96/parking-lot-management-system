import { Link } from '@tanstack/react-router'
import { CarFront, LayoutDashboard, ParkingSquare, UserRound } from 'lucide-react'
import type { ReactNode } from 'react'
import { Logo } from '@/components/brand/Logo'
import { useOwnerAuth } from '@/stores/owner'

// Header and footer around the public pages (/, /register, /my).
// Each public page wraps its content in <PublicLayout>...</PublicLayout>.

// TanStack Router adds the "active" class to the link of the current page.
const navLink =
  'inline-flex items-center gap-1.5 rounded-lg px-3 py-2 text-sm font-medium text-slate-600 hover:bg-slate-100 hover:text-slate-900 ' +
  '[&.active]:bg-brand-50 [&.active]:text-brand-800'

export function PublicLayout({ children }: { children: ReactNode }) {
  const owner = useOwnerAuth((s) => s.owner)
  return (
    <div className="flex min-h-screen flex-col">
      <header className="sticky top-0 z-30 border-b border-slate-200 bg-white/90 backdrop-blur">
        <div className="mx-auto flex max-w-6xl items-center justify-between gap-3 px-4 py-3">
          <Link to="/" aria-label="PLMS Parking home">
            <Logo />
          </Link>
          <nav className="flex items-center gap-1" aria-label="Main">
            <Link to="/" className={navLink} activeOptions={{ exact: true }}>
              <ParkingSquare className="size-4" aria-hidden />
              <span className="hidden sm:inline">Availability</span>
            </Link>
            <Link to="/register" className={navLink}>
              <CarFront className="size-4" aria-hidden />
              <span className="hidden sm:inline">Register</span>
            </Link>
            <Link to="/my" className={navLink}>
              <UserRound className="size-4" aria-hidden />
              <span className="hidden sm:inline">{owner ? 'My vehicles' : 'Sign in'}</span>
            </Link>
          </nav>
        </div>
      </header>

      <main className="flex-1">{children}</main>

      <footer className="border-t border-slate-200 bg-white">
        <div className="mx-auto flex max-w-6xl flex-col gap-2 px-4 py-5 text-xs text-slate-500 sm:flex-row sm:items-center sm:justify-between">
          <p>PLMS Parking · One unattended car park · Times shown in IST</p>
          <div className="flex items-center gap-4">
            <span>Icons: Lucide (ISC licence) · Illustrations: original SVG</span>
            <Link to="/admin" className="inline-flex items-center gap-1 font-medium text-slate-600 hover:text-slate-900">
              <LayoutDashboard className="size-3.5" aria-hidden /> Admin
            </Link>
          </div>
        </div>
      </footer>
    </div>
  )
}
