import { useMutation, useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { History, Save } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { api } from '@/api/client'
import type { PricingRule, Settings, VehicleType } from '@/api/types'
import { VehicleIcon } from '@/components/brand/VehicleArt'
import { AdminLayout } from '@/components/layout/AdminLayout'
import { Button, Card, ErrorBox, Field, Input, Spinner, Switch } from '@/components/ui'
import { errorMessage } from '@/lib/errors'
import { calculateFee, rateSummary } from '@/lib/fee'
import { dateTimeIST, rupees, vehicleTypeLabel } from '@/lib/format'
import { keys, queryClient } from '@/lib/queryClient'
import { requireAdmin } from '@/lib/requireAdmin'

// URL: /admin/pricing — rate card per vehicle type (FR-35) and the overflow switch (FR-36).
export const Route = createFileRoute('/admin/pricing')({
  beforeLoad: requireAdmin,
  component: PricingPage,
})

function PricingPage() {
  return (
    <AdminLayout title="Pricing" description="A change applies to tickets opened afterwards; parked vehicles keep the rate they entered with.">
      <OverflowSwitch />
      <div className="grid gap-5 lg:grid-cols-3">
        <RateCard vehicleType="MOTORCYCLE" />
        <RateCard vehicleType="CAR" />
        <RateCard vehicleType="TRUCK" />
      </div>
    </AdminLayout>
  )
}

function OverflowSwitch() {
  const settings = useQuery({ queryKey: keys.settings, queryFn: () => api<Settings>('/admin/settings', { auth: 'admin' }) })
  const toggle = useMutation({
    mutationFn: (enabled: boolean) => api<Settings>('/admin/settings/overflow', { method: 'PUT', auth: 'admin', body: { enabled } }),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: keys.settings })
      queryClient.invalidateQueries({ queryKey: keys.availability })
      toast.success(result.overflowEnabled ? 'Overflow is on' : 'Overflow is off')
    },
    onError: (err) => toast.error(errorMessage(err)),
  })
  const on = settings.data?.overflowEnabled ?? false

  return (
    <Card className="flex flex-wrap items-center gap-4 p-5">
      <div className="min-w-0 flex-1">
        <h2 className="font-semibold">Overflow allocation</h2>
        <p className="text-sm text-slate-500">
          When a vehicle's own spot type is full, use a larger one: motorcycles → regular → oversized, cars → oversized. Vehicles always pay their own rate.
        </p>
      </div>
      <span className="text-sm font-medium">{on ? 'On' : 'Off'}</span>
      <Switch label="Overflow allocation" checked={on} disabled={!settings.data || toggle.isPending} onChange={(value) => toggle.mutate(value)} />
    </Card>
  )
}

function RateCard({ vehicleType }: { vehicleType: VehicleType }) {
  const pricing = useQuery({
    queryKey: [...keys.pricing, vehicleType],
    queryFn: () => api<{ current: PricingRule; history: PricingRule[] }>(`/admin/pricing/${vehicleType}`, { auth: 'admin' }),
  })
  return (
    <Card className="p-5">
      <div className="mb-4 flex items-center gap-3">
        <VehicleIcon type={vehicleType} hue={vehicleType === 'CAR' ? 215 : vehicleType === 'TRUCK' ? 150 : 28} size={44} />
        <h2 className="text-base font-semibold">{vehicleTypeLabel[vehicleType]}</h2>
      </div>
      {pricing.isLoading && <Spinner />}
      {/* `key` makes the form start fresh whenever a new rate is saved. */}
      {pricing.data && <RateForm key={pricing.data.current.id} vehicleType={vehicleType} current={pricing.data.current} history={pricing.data.history} />}
    </Card>
  )
}

function RateForm({ vehicleType, current, history }: { vehicleType: VehicleType; current: PricingRule; history: PricingRule[] }) {
  // The form shows rupees; the API uses paise (₹1 = 100 paise).
  const [hourly, setHourly] = useState(String(current.hourlyRatePaise / 100))
  const [dailyCap, setDailyCap] = useState(String(current.dailyCapPaise / 100))
  const [grace, setGrace] = useState(String(current.graceMinutes))

  const newRate = {
    hourlyRatePaise: Math.round(Number(hourly) * 100),
    dailyCapPaise: Math.round(Number(dailyCap) * 100),
    graceMinutes: Number(grace),
  }
  const isValid = Number.isFinite(newRate.hourlyRatePaise) && Number.isFinite(newRate.dailyCapPaise) && Number.isInteger(newRate.graceMinutes)
  const isChanged =
    newRate.hourlyRatePaise !== current.hourlyRatePaise || newRate.dailyCapPaise !== current.dailyCapPaise || newRate.graceMinutes !== current.graceMinutes

  const save = useMutation({
    mutationFn: () => api(`/admin/pricing/${vehicleType}`, { method: 'PUT', auth: 'admin', body: newRate }),
    onSuccess: () => {
      toast.success(`${vehicleTypeLabel[vehicleType]} rate updated`)
      queryClient.invalidateQueries({ queryKey: [...keys.pricing, vehicleType] })
    },
  })

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <div className="grid grid-cols-3 gap-2">
        <Field label="₹ / hour">
          <Input inputMode="decimal" value={hourly} onChange={(e) => setHourly(e.target.value)} />
        </Field>
        <Field label="₹ cap / day">
          <Input inputMode="decimal" value={dailyCap} onChange={(e) => setDailyCap(e.target.value)} />
        </Field>
        <Field label="Free min">
          <Input inputMode="numeric" value={grace} onChange={(e) => setGrace(e.target.value)} />
        </Field>
      </div>

      {isValid && (
        <div className="rounded-lg bg-slate-50 p-3 text-xs text-slate-600">
          <p className="font-medium text-slate-700">{rateSummary(newRate)}</p>
          <p className="mt-1">
            Examples: 50 min → {rupees(calculateFee(50, newRate).feePaise)} · 3 h 20 min → {rupees(calculateFee(200, newRate).feePaise)} · 26 h 10 min →{' '}
            {rupees(calculateFee(1570, newRate).feePaise)}
          </p>
        </div>
      )}
      {save.error && <ErrorBox>{errorMessage(save.error)}</ErrorBox>}
      <Button type="submit" className="w-full" disabled={!isChanged || !isValid} loading={save.isPending}>
        <Save /> Save new rate
      </Button>

      <details className="text-xs text-slate-500">
        <summary className="flex cursor-pointer items-center gap-1 font-medium">
          <History className="size-3.5" /> History ({history.length})
        </summary>
        <ul className="mt-2 space-y-1">
          {history.map((rule) => (
            <li key={rule.id} className="flex justify-between gap-2">
              <span>
                {rupees(rule.hourlyRatePaise)}/h · cap {rupees(rule.dailyCapPaise)} · {rule.graceMinutes} min
              </span>
              <span className="text-slate-400">{rule.effectiveTo ? dateTimeIST(rule.effectiveFrom) : 'current'}</span>
            </li>
          ))}
        </ul>
      </details>
    </form>
  )
}
