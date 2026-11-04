import { Link, type LinkProps } from '@tanstack/react-router'
import { Loader2 } from 'lucide-react'
import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { cn } from '@/lib/utils'

const variantClasses = {
  primary: 'bg-brand-700 text-white shadow-sm hover:bg-brand-800',
  secondary: 'bg-white text-slate-800 ring-1 ring-inset ring-slate-300 hover:bg-slate-50',
  ghost: 'text-slate-700 hover:bg-slate-100',
  danger: 'bg-red-600 text-white shadow-sm hover:bg-red-700',
  success: 'bg-emerald-600 text-white shadow-sm hover:bg-emerald-700',
  link: 'text-brand-700 hover:underline',
}

const sizeClasses = {
  normal: 'h-10 px-4',
  small: 'h-8 px-3 text-xs',
  large: 'h-12 px-6 text-base',
  icon: 'h-9 w-9',
}

type Variant = keyof typeof variantClasses
type Size = keyof typeof sizeClasses

function buttonClasses(variant: Variant, size: Size, className?: string) {
  return cn(
    'inline-flex cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-lg text-sm font-semibold transition-colors',
    'disabled:pointer-events-none disabled:opacity-55 [&_svg]:size-4 [&_svg]:shrink-0',
    variantClasses[variant],
    sizeClasses[size],
    className,
  )
}

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant
  size?: Size
  /** Shows a spinner and disables the button while a request runs (SRS 10.6). */
  loading?: boolean
}

export function Button({ variant = 'primary', size = 'normal', loading, disabled, className, children, ...props }: ButtonProps) {
  return (
    <button className={buttonClasses(variant, size, className)} disabled={disabled || loading} {...props}>
      {loading && <Loader2 className="animate-spin" aria-hidden />}
      {children}
    </button>
  )
}

/** A link that looks like a button. */
export function LinkButton({
  to,
  variant = 'primary',
  size = 'normal',
  className,
  children,
}: {
  to: LinkProps['to']
  variant?: Variant
  size?: Size
  className?: string
  children: ReactNode
}) {
  return (
    <Link to={to} className={buttonClasses(variant, size, className)}>
      {children}
    </Link>
  )
}
