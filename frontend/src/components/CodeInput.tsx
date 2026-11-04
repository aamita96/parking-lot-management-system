import { useEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'

/** Six single-digit boxes with paste support (SRS 10.3 step 2). Calls onComplete with the full code. */
export function CodeInput({
  onComplete,
  disabled,
  invalid,
  autoFocus = true,
}: {
  onComplete: (code: string) => void
  disabled?: boolean
  invalid?: boolean
  autoFocus?: boolean
}) {
  const [digits, setDigits] = useState<string[]>(Array(6).fill(''))
  const refs = useRef<(HTMLInputElement | null)[]>([])

  useEffect(() => {
    if (autoFocus) refs.current[0]?.focus()
  }, [autoFocus])

  // Clear after a wrong code so the owner can type again.
  useEffect(() => {
    if (invalid) {
      setDigits(Array(6).fill(''))
      refs.current[0]?.focus()
    }
  }, [invalid])

  const update = (next: string[]) => {
    setDigits(next)
    if (next.every((d) => d !== '')) onComplete(next.join(''))
  }

  const fill = (from: number, text: string) => {
    const chars = text.replace(/\D/g, '').slice(0, 6 - from).split('')
    if (!chars.length) return
    const next = [...digits]
    chars.forEach((c, i) => (next[from + i] = c))
    refs.current[Math.min(5, from + chars.length)]?.focus()
    update(next)
  }

  return (
    <div className="flex justify-center gap-2" role="group" aria-label="6-digit code">
      {digits.map((d, i) => (
        <input
          key={i}
          ref={(el) => {
            refs.current[i] = el
          }}
          value={d}
          disabled={disabled}
          inputMode="numeric"
          autoComplete={i === 0 ? 'one-time-code' : 'off'}
          aria-label={`Digit ${i + 1}`}
          maxLength={6}
          className={cn(
            'size-12 rounded-xl border-2 bg-white text-center font-mono text-2xl font-bold text-slate-900 sm:size-14',
            'focus:border-brand-600 focus:outline-none focus:ring-4 focus:ring-brand-100 disabled:bg-slate-100',
            invalid ? 'border-red-400' : 'border-slate-300',
          )}
          onChange={(e) => {
            const v = e.target.value
            if (v.length > 1) return fill(i, v)
            if (v && !/\d/.test(v)) return
            const next = [...digits]
            next[i] = v
            if (v && i < 5) refs.current[i + 1]?.focus()
            update(next)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Backspace' && !digits[i] && i > 0) refs.current[i - 1]?.focus()
            if (e.key === 'ArrowLeft' && i > 0) refs.current[i - 1]?.focus()
            if (e.key === 'ArrowRight' && i < 5) refs.current[i + 1]?.focus()
          }}
          onPaste={(e) => {
            e.preventDefault()
            fill(i, e.clipboardData.getData('text'))
          }}
        />
      ))}
    </div>
  )
}

/** "Resend code" countdown (30 s, SRS 10.3). */
export function useCountdown(seconds: number, key: unknown) {
  const [left, setLeft] = useState(seconds)
  useEffect(() => {
    setLeft(seconds)
    const t = setInterval(() => setLeft((l) => (l > 0 ? l - 1 : 0)), 1000)
    return () => clearInterval(t)
  }, [seconds, key])
  return left
}
