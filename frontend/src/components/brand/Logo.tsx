import { cn } from '@/lib/utils'

export function LogoMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 64" className={cn('size-8', className)} aria-hidden>
      <rect width="64" height="64" rx="14" fill="#1e40af" />
      <path
        d="M22 48V16h13.5c6.9 0 11.5 4.3 11.5 10.6S42.4 37.3 35.5 37.3H29V48z M29 31.4h5.9c3.2 0 5.1-1.8 5.1-4.8s-1.9-4.8-5.1-4.8H29z"
        fill="#fff"
      />
      <rect x="44" y="44" width="10" height="10" rx="3" fill="#fbbf24" />
    </svg>
  )
}

export function Logo({ className, subtitle }: { className?: string; subtitle?: string }) {
  return (
    <span className={cn('inline-flex items-center gap-2.5', className)}>
      <LogoMark />
      <span className="leading-tight">
        <span className="block text-base font-extrabold tracking-tight text-slate-900">PLMS Parking</span>
        {subtitle && <span className="block text-[11px] font-medium uppercase tracking-wider text-slate-500">{subtitle}</span>}
      </span>
    </span>
  )
}
