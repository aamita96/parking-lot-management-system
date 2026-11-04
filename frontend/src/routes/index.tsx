import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { ArrowRight, Mail, QrCode, ShieldCheck } from 'lucide-react'
import { api } from '@/api/client'
import type { Availability } from '@/api/types'
import { AvailabilityPanel } from '@/components/AvailabilityPanels'
import { HeroIllustration } from '@/components/brand/HeroIllustration'
import { PublicLayout } from '@/components/layout/PublicLayout'
import { ErrorBox, LinkButton, Spinner } from '@/components/ui'
import { errorMessage } from '@/lib/errors'
import { keys } from '@/lib/queryClient'

// URL: /  — the public availability page (FR-10 to FR-13).
export const Route = createFileRoute('/')({
  component: AvailabilityPage,
})

function AvailabilityPage() {
  // Ask the API for free spots, and ask again every 5 seconds while the tab is open (FR-12).
  const availability = useQuery({
    queryKey: keys.availability,
    queryFn: () => api<Availability>('/availability'),
    refetchInterval: 5000,
  })
  const data = availability.data
  const totalFree = data ? data.panels.reduce((sum, panel) => sum + panel.available, 0) : 0

  return (
    <PublicLayout>
      <section className="border-b border-slate-200 bg-gradient-to-b from-white to-brand-50/60">
        <div className="mx-auto grid max-w-6xl items-center gap-8 px-4 py-10 md:grid-cols-[1.1fr_1fr] md:py-14">
          <div>
            <p className="inline-flex items-center gap-2 rounded-full bg-white px-3 py-1 text-xs font-semibold text-brand-800 ring-1 ring-brand-200">
              <span className="size-2 animate-pulse rounded-full bg-emerald-500" />
              Live · {totalFree} spots free now
            </p>
            <h1 className="mt-4 text-4xl font-extrabold tracking-tight text-slate-900 sm:text-5xl">
              Drive in. <span className="text-brand-700">No ticket queue.</span>
            </h1>
            <p className="mt-4 max-w-lg text-lg text-slate-600">
              Register your vehicle once. The gate reads your plate, assigns you a spot and opens the barrier. Pay at the kiosk on the way out.
            </p>
            <div className="mt-6 flex flex-wrap gap-3">
              <LinkButton to="/register" size="large">
                Register your vehicle <ArrowRight />
              </LinkButton>
              <LinkButton to="/my" size="large" variant="secondary">
                View my vehicles
              </LinkButton>
            </div>
            <ul className="mt-8 grid gap-3 text-sm text-slate-600 sm:grid-cols-3">
              <li className="flex items-center gap-2">
                <QrCode className="size-4 text-brand-700" aria-hidden /> Register from the gate QR
              </li>
              <li className="flex items-center gap-2">
                <Mail className="size-4 text-brand-700" aria-hidden /> Email on entry and exit
              </li>
              <li className="flex items-center gap-2">
                <ShieldCheck className="size-4 text-brand-700" aria-hidden /> No password needed
              </li>
            </ul>
          </div>
          <HeroIllustration className="mx-auto w-full max-w-md drop-shadow-xl" />
        </div>
      </section>

      <section className="mx-auto max-w-6xl px-4 py-10">
        <h2 className="text-2xl font-bold text-slate-900">Spaces right now</h2>
        <p className="mb-5 text-sm text-slate-500">Updates automatically every few seconds.</p>

        {availability.isLoading && <Spinner />}
        {availability.error && <ErrorBox>{errorMessage(availability.error)}</ErrorBox>}
        {data && (
          <div className="grid gap-5 md:grid-cols-3">
            {data.panels.map((panel) => (
              <AvailabilityPanel key={panel.vehicleType} panel={panel} />
            ))}
          </div>
        )}
        {data?.overflowEnabled && (
          <p className="mt-4 text-sm text-slate-500">Overflow is on: smaller vehicles may use larger bays when their own type is full.</p>
        )}
      </section>
    </PublicLayout>
  )
}
