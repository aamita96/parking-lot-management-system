import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

const iconColors = {
  default: 'bg-brand-50 text-brand-700',
  success: 'bg-emerald-50 text-emerald-600',
  danger: 'bg-red-50 text-red-600',
  warning: 'bg-amber-50 text-amber-600',
}

/**
 * A popup window. Radix handles the hard parts: it keeps keyboard focus inside the popup
 * and closes it with Esc (SRS 10.6).
 *
 *   <Modal open={isOpen} onClose={() => setOpen(false)} title="Remove vehicle?">
 *     ...content...
 *     <ModalFooter><Button>OK</Button></ModalFooter>
 *   </Modal>
 */
export function Modal({
  open,
  onClose,
  title,
  description,
  icon,
  color = 'default',
  wide,
  children,
}: {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  icon?: ReactNode
  color?: keyof typeof iconColors
  wide?: boolean
  children?: ReactNode
}) {
  return (
    <Dialog.Root open={open} onOpenChange={(isOpen) => !isOpen && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-slate-900/50 backdrop-blur-[2px]" />
        <Dialog.Content
          className={cn(
            'fixed left-1/2 top-1/2 z-50 max-h-[90vh] w-[calc(100vw-2rem)] -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-2xl bg-white p-6 shadow-2xl focus:outline-none',
            wide ? 'max-w-2xl' : 'max-w-md',
          )}
        >
          <div className="flex items-start gap-3">
            {icon && <div className={cn('grid size-10 shrink-0 place-items-center rounded-full [&_svg]:size-5', iconColors[color])}>{icon}</div>}
            <div className="min-w-0 flex-1">
              <Dialog.Title className="text-lg font-semibold text-slate-900">{title}</Dialog.Title>
              <Dialog.Description className={description ? 'mt-0.5 text-sm text-slate-500' : 'sr-only'}>{description ?? title}</Dialog.Description>
            </div>
            <Dialog.Close className="rounded-md p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-600" aria-label="Close">
              <X className="size-5" />
            </Dialog.Close>
          </div>
          <div className="mt-4">{children}</div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

/** Row of buttons at the bottom of a modal. */
export function ModalFooter({ children }: { children: ReactNode }) {
  return <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:items-center sm:justify-end">{children}</div>
}

/** Label/value list used in the gate and payment popups. */
export function DetailList({ items }: { items: [string, ReactNode][] }) {
  return (
    <dl className="divide-y divide-slate-100 rounded-xl border border-slate-200">
      {items.map(([label, value]) => (
        <div key={label} className="grid grid-cols-[9rem_1fr] gap-3 px-3 py-2 text-sm">
          <dt className="text-slate-500">{label}</dt>
          <dd className="min-w-0 break-words font-medium text-slate-900">{value}</dd>
        </div>
      ))}
    </dl>
  )
}
