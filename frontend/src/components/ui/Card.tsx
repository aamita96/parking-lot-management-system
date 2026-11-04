import { Loader2 } from 'lucide-react'
import type { HTMLAttributes, ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function Card({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('rounded-xl border border-slate-200 bg-white shadow-card', className)} {...props} />
}

/** Title bar at the top of a card, with optional buttons on the right. */
export function CardHeader({ title, actions }: { title: string; actions?: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-slate-100 px-5 py-4">
      <h2 className="text-base font-semibold text-slate-900">{title}</h2>
      {actions}
    </div>
  )
}

const badgeColors = {
  slate: 'bg-slate-100 text-slate-700 ring-slate-200',
  blue: 'bg-brand-50 text-brand-800 ring-brand-200',
  green: 'bg-emerald-50 text-emerald-700 ring-emerald-200',
  amber: 'bg-amber-50 text-amber-800 ring-amber-200',
  red: 'bg-red-50 text-red-700 ring-red-200',
  violet: 'bg-violet-50 text-violet-700 ring-violet-200',
}

export type BadgeColor = keyof typeof badgeColors

/** A small coloured label. Always put words in it: colour must never be the only signal (WCAG). */
export function Badge({ color = 'slate', children, className }: { color?: BadgeColor; children: ReactNode; className?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold ring-1 ring-inset', badgeColors[color], className)}>
      {children}
    </span>
  )
}

export function ErrorBox({ children }: { children: ReactNode }) {
  return (
    <div role="alert" className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
      {children}
    </div>
  )
}

export function Spinner({ label = 'Loading' }: { label?: string }) {
  return (
    <span role="status" className="inline-flex items-center gap-2 text-sm text-slate-500">
      <Loader2 className="size-4 animate-spin" aria-hidden />
      {label}
    </span>
  )
}

export function EmptyState({ icon, title, children }: { icon?: ReactNode; title: string; children?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 px-6 py-12 text-center">
      {icon && <div className="text-slate-300 [&_svg]:size-10">{icon}</div>}
      <p className="font-semibold text-slate-700">{title}</p>
      {children && <div className="max-w-sm text-sm text-slate-500">{children}</div>}
    </div>
  )
}
