import type { InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

const inputClasses =
  'w-full rounded-lg border border-slate-300 bg-white px-3 text-sm text-slate-900 placeholder:text-slate-400 ' +
  'focus:border-brand-600 focus:outline-none focus:ring-2 focus:ring-brand-200 disabled:bg-slate-100'

/**
 * A form field: label on top, the input inside, then a hint or an error underneath.
 * Wrapping the input in <label> connects them, so clicking the label focuses the input
 * and screen readers read the label.
 *
 *   <Field label="Email" error={errors.email}>
 *     <Input value={email} onChange={(e) => setEmail(e.target.value)} />
 *   </Field>
 */
export function Field({
  label,
  hint,
  error,
  optional,
  children,
  className,
}: {
  label: string
  hint?: ReactNode
  error?: string
  optional?: boolean
  children: ReactNode
  className?: string
}) {
  return (
    <div className={className}>
      <label className="block space-y-1.5">
        <span className="block text-sm font-medium text-slate-700">
          {label}
          {optional && <span className="ml-1 font-normal text-slate-400">(optional)</span>}
        </span>
        {children}
      </label>
      {error ? (
        <p role="alert" className="mt-1.5 text-xs font-medium text-red-600">
          {error}
        </p>
      ) : (
        hint && <p className="mt-1.5 text-xs text-slate-500">{hint}</p>
      )}
    </div>
  )
}

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cn(inputClasses, 'h-10', className)} {...props} />
}

export function Textarea({ className, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea className={cn(inputClasses, 'min-h-20 py-2', className)} {...props} />
}

export function Select({ className, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className={cn(inputClasses, 'h-10 w-auto pr-8', className)} {...props} />
}

/** An on/off toggle. */
export function Switch({ checked, onChange, label, disabled }: { checked: boolean; onChange: (on: boolean) => void; label: string; disabled?: boolean }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cn(
        'inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full transition-colors disabled:opacity-50',
        checked ? 'bg-brand-700' : 'bg-slate-300',
      )}
    >
      <span className={cn('block size-5 rounded-full bg-white shadow transition-transform', checked ? 'translate-x-[22px]' : 'translate-x-0.5')} />
    </button>
  )
}
