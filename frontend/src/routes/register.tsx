import { useMutation, useQuery } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { AlertTriangle, CheckCircle2, Info, MailCheck, RotateCw, ShieldAlert } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { ApiError, api } from '@/api/client'
import type { Availability, MyVehicle, NumberCheck, OwnerSession, RegistrationResult, VehicleType } from '@/api/types'
import { VehicleIcon } from '@/components/brand/VehicleArt'
import { CodeInput, useCountdown } from '@/components/CodeInput'
import { DisputeDialog } from '@/components/DisputeDialog'
import { PublicLayout } from '@/components/layout/PublicLayout'
import { Button, Card, ErrorBox, Field, Input, LinkButton } from '@/components/ui'
import { errorMessage } from '@/lib/errors'
import { spotTypeLabel } from '@/lib/format'
import { keys, queryClient } from '@/lib/queryClient'
import { cn } from '@/lib/utils'
import { checkEmail, checkMobile, checkPlate, checkRegistrationId, checkVehicleType, isValidPlate, normalisePlate } from '@/lib/validation'
import { useOwnerAuth } from '@/stores/owner'

// URL: /register  (or /register?plate=KA01AB1234 from the QR code at the gate)
// Three steps (SRS 10.3): 1. details → 2. email code → 3. done.
export const Route = createFileRoute('/register')({
  validateSearch: (search: Record<string, unknown>): { plate?: string } => ({
    plate: typeof search.plate === 'string' ? search.plate : undefined,
  }),
  component: RegisterPage,
})

type Step = 'details' | 'code' | 'done' | 'already-yours'

function RegisterPage() {
  const [step, setStep] = useState<Step>('details')
  const [registration, setRegistration] = useState<RegistrationResult | null>(null)
  const [vehicleNumber, setVehicleNumber] = useState('')

  return (
    <PublicLayout>
      <div className="mx-auto max-w-xl px-4 py-8 sm:py-12">
        <Steps current={step === 'details' ? 1 : step === 'code' ? 2 : 3} />

        {step === 'details' && (
          <DetailsStep
            onCodeSent={(result) => {
              setRegistration(result)
              setStep('code')
            }}
            onAlreadyYours={(number) => {
              setVehicleNumber(number)
              setStep('already-yours')
            }}
            onAdded={(number) => {
              setVehicleNumber(number)
              setStep('done')
            }}
          />
        )}

        {step === 'code' && registration && (
          <CodeStep
            registration={registration}
            onBack={() => setStep('details')}
            onVerified={(number) => {
              setVehicleNumber(number)
              setStep('done')
            }}
          />
        )}

        {step === 'done' && (
          <Finished
            title={`${vehicleNumber} is ready to park`}
            text="You'll get an email each time it enters and leaves."
            againLabel="Register another vehicle"
            onAgain={() => setStep('details')}
          />
        )}

        {step === 'already-yours' && (
          <Finished
            title={`${vehicleNumber} is already registered to you`}
            text="It can park now. Sign in to see its tickets or edit details."
            againLabel="Register a different vehicle"
            onAgain={() => setStep('details')}
          />
        )}
      </div>
    </PublicLayout>
  )
}

function Steps({ current }: { current: number }) {
  const names = ['Details', 'Email code', 'Done']
  return (
    <ol className="mb-6 flex items-center gap-2" aria-label="Registration progress">
      {names.map((name, index) => {
        const number = index + 1
        return (
          <li key={name} className="flex flex-1 items-center gap-2" aria-current={number === current ? 'step' : undefined}>
            <span
              className={cn(
                'grid size-7 shrink-0 place-items-center rounded-full text-xs font-bold',
                number < current ? 'bg-emerald-500 text-white' : number === current ? 'bg-brand-700 text-white' : 'bg-slate-200 text-slate-500',
              )}
            >
              {number < current ? '✓' : number}
            </span>
            {/* On phones only the current step's name is shown. */}
            <span className={cn('whitespace-nowrap text-sm font-medium', number === current ? 'text-slate-900' : 'hidden text-slate-500 sm:inline')}>{name}</span>
            {number < names.length && <span className="h-px flex-1 bg-slate-200" aria-hidden />}
          </li>
        )
      })}
    </ol>
  )
}

// ---------------------------------------------------------------- Step 1: details

const vehicleTypes: { value: VehicleType; label: string; hue: number }[] = [
  { value: 'MOTORCYCLE', label: 'Motorcycle', hue: 28 },
  { value: 'CAR', label: 'Car', hue: 215 },
  { value: 'TRUCK', label: 'Truck', hue: 150 },
]

type FormValues = {
  vehicleNumber: string
  vehicleType: VehicleType | ''
  ownerEmail: string
  mobile: string
  registrationId: string
}

type FormErrors = Partial<Record<keyof FormValues, string>>

function validate(values: FormValues): FormErrors {
  const errors: FormErrors = {
    vehicleNumber: checkPlate(values.vehicleNumber),
    vehicleType: checkVehicleType(values.vehicleType),
    ownerEmail: checkEmail(values.ownerEmail),
    mobile: checkMobile(values.mobile),
    registrationId: checkRegistrationId(values.registrationId),
  }
  // Keep only the fields that actually have a message.
  return Object.fromEntries(Object.entries(errors).filter(([, message]) => message !== ''))
}

function DetailsStep({
  onCodeSent,
  onAlreadyYours,
  onAdded,
}: {
  onCodeSent: (result: RegistrationResult) => void
  onAlreadyYours: (vehicleNumber: string) => void
  onAdded: (vehicleNumber: string) => void
}) {
  const { plate } = Route.useSearch()
  const owner = useOwnerAuth((s) => s.owner) // set if the owner is already signed in
  const [values, setValues] = useState<FormValues>({
    vehicleNumber: plate ?? '',
    vehicleType: '',
    ownerEmail: owner?.email ?? '',
    mobile: owner?.mobile ?? '',
    registrationId: '',
  })
  const [errors, setErrors] = useState<FormErrors>({})
  const [disputeOpen, setDisputeOpen] = useState(false)

  function update(field: keyof FormValues, value: string) {
    setValues({ ...values, [field]: value })
    setErrors({ ...errors, [field]: undefined }) // hide the old error while typing
  }

  // Free spot counts shown on the vehicle type buttons.
  const availability = useQuery({ queryKey: keys.availability, queryFn: () => api<Availability>('/availability') })

  // Warn straight away if the number is already registered (SRS 10.3). We wait until the
  // user stops typing for 0.4 s before asking the server.
  const plateNow = normalisePlate(values.vehicleNumber)
  const [plateToCheck, setPlateToCheck] = useState('')
  
  useEffect(() => {
    const timer = setTimeout(() => setPlateToCheck(plateNow), 400)
    return () => clearTimeout(timer)
  }, [plateNow]);

  const numberCheck = useQuery({
    queryKey: ['number-check', plateToCheck],
    queryFn: () => api<NumberCheck>(`/public/vehicles/${plateToCheck}/check`),
    enabled: isValidPlate(plateToCheck),
  })

  const alreadyTaken = numberCheck.data?.registered === true && numberCheck.data.vehicleNumber === plateNow

  const submit = useMutation({
    mutationFn: async () => {
      const body = {
        vehicleNumber: values.vehicleNumber,
        vehicleType: values.vehicleType,
        ownerEmail: values.ownerEmail,
        mobile: values.mobile || null,
        registrationId: values.registrationId || null,
      }
      if (owner) {
        // Signed-in owners add vehicles straight away, without another code (FR-6).
        const vehicle = await api<MyVehicle>('/me/vehicles', { method: 'POST', auth: 'owner', body })
        if (values.mobile) await api('/me', { method: 'PATCH', auth: 'owner', body: { mobile: values.mobile } })
        queryClient.invalidateQueries({ queryKey: keys.myVehicles })
        onAdded(vehicle.vehicleNumber)
        return
      }
      const result = await api<RegistrationResult>('/public/registrations', { method: 'POST', body })
      if (result.status === 'ALREADY_REGISTERED') onAlreadyYours(result.vehicleNumber)
      else onCodeSent(result)
    },
    onError: (err) => {
      // Show the server's field errors next to the fields.
      if (err instanceof ApiError && err.code === 'VALIDATION_ERROR') {
        const fromServer: FormErrors = {}
        for (const e of err.body.errors ?? []) fromServer[e.field as keyof FormValues] = e.message
        setErrors(fromServer)
      }
    },
  })

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    const found = validate(values)
    setErrors(found)
    if (Object.keys(found).length === 0) submit.mutate()
  }

  const takenByOther = submit.error instanceof ApiError && submit.error.code === 'VEHICLE_ALREADY_REGISTERED'
  const otherError = submit.error && !takenByOther && !(submit.error instanceof ApiError && submit.error.code === 'VALIDATION_ERROR')

  return (
    <Card className="p-5 sm:p-6">
      <h1 className="text-2xl font-bold text-slate-900">Register your vehicle</h1>
      <p className="mt-1 text-sm text-slate-500">
        {owner ? `Signed in as ${owner.email}. No code needed.` : 'Takes under a minute. We email you a code to confirm — no password.'}
      </p>

      <form className="mt-6 space-y-5" noValidate onSubmit={handleSubmit}>
        <Field
          label="Vehicle number"
          error={errors.vehicleNumber}
          hint={plateNow && plateNow !== values.vehicleNumber ? `Saved as ${plateNow}` : 'Like KA 01 AB 1234 or 22 BH 1234 AB'}
        >
          <Input
            value={values.vehicleNumber}
            onChange={(e) => update('vehicleNumber', e.target.value)}
            placeholder="KA 01 AB 1234"
            autoCapitalize="characters"
            autoComplete="off"
            spellCheck={false}
            className="h-12 font-mono text-lg uppercase tracking-wider"
          />
        </Field>

        {alreadyTaken && (
          <div className="flex gap-3 rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900" role="status">
            <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden />
            <p>
              <strong>{plateNow} is already registered.</strong> If it's yours,{' '}
              <Link to="/my" className="font-semibold underline">
                sign in to My vehicles
              </Link>
              . If someone else registered it,{' '}
              <button type="button" className="font-semibold underline" onClick={() => setDisputeOpen(true)}>
                report a problem
              </button>
              .
            </p>
          </div>
        )}

        <fieldset>
          <legend className="text-sm font-medium text-slate-700">Vehicle type</legend>
          <div className="mt-1.5 grid grid-cols-3 gap-2" role="radiogroup">
            {vehicleTypes.map((type) => {
              const panel = availability.data?.panels.find((p) => p.vehicleType === type.value)
              const selected = values.vehicleType === type.value
              return (
                <button
                  key={type.value}
                  type="button"
                  role="radio"
                  aria-checked={selected}
                  onClick={() => update('vehicleType', type.value)}
                  className={cn(
                    'flex flex-col items-center gap-1 rounded-xl border-2 px-2 py-3 text-center',
                    selected ? 'border-brand-600 bg-brand-50 shadow-sm' : 'border-slate-200 bg-white hover:border-slate-300',
                  )}
                >
                  <VehicleIcon type={type.value} hue={type.hue} size={60} />
                  <span className="text-sm font-bold text-slate-900">{type.label}</span>
                  {panel && (
                    <>
                      <span className="text-[11px] text-slate-500">{spotTypeLabel[panel.spotType]} spot</span>
                      <span className={cn('text-[11px] font-semibold', panel.full ? 'text-red-600' : 'text-emerald-700')}>
                        {panel.full ? 'Full now' : `${panel.available} free`}
                      </span>
                    </>
                  )}
                </button>
              )
            })}
          </div>
          {errors.vehicleType && (
            <p role="alert" className="mt-1.5 text-xs font-medium text-red-600">
              {errors.vehicleType}
            </p>
          )}
        </fieldset>

        <Field label="Your email" error={errors.ownerEmail} hint={owner ? undefined : "We'll send the code here, plus an email each time you enter and leave."}>
          <Input
            type="email"
            autoComplete="email"
            readOnly={!!owner}
            value={values.ownerEmail}
            onChange={(e) => update('ownerEmail', e.target.value)}
            placeholder="you@example.com"
          />
        </Field>

        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Mobile" optional error={errors.mobile}>
            <Input type="tel" autoComplete="tel" value={values.mobile} onChange={(e) => update('mobile', e.target.value)} placeholder="+91 98765 43210" />
          </Field>
          <Field label="Registration (RC) number" optional error={errors.registrationId} hint="Helps settle ownership disputes.">
            <Input value={values.registrationId} onChange={(e) => update('registrationId', e.target.value)} />
          </Field>
        </div>

        {takenByOther && (
          <div className="flex items-start gap-3 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-800" role="alert">
            <ShieldAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
            <div>
              <p className="font-medium">This number is registered to another owner.</p>
              <Button type="button" variant="secondary" size="small" className="mt-2" onClick={() => setDisputeOpen(true)}>
                Report a problem
              </Button>
            </div>
          </div>
        )}
        {otherError && <ErrorBox>{errorMessage(submit.error)}</ErrorBox>}

        <Button type="submit" size="large" className="w-full" loading={submit.isPending}>
          {owner ? 'Add vehicle' : 'Send me a code'}
        </Button>
      </form>

      {disputeOpen && <DisputeDialog open onClose={() => setDisputeOpen(false)} vehicleNumber={plateNow} email={values.ownerEmail} />}
    </Card>
  )
}

// ---------------------------------------------------------------- Step 2: email code

function CodeStep({ registration, onBack, onVerified }: { registration: RegistrationResult; onBack: () => void; onVerified: (vehicleNumber: string) => void }) {
  const [current, setCurrent] = useState(registration) // replaced when a new code is sent
  const secondsLeft = useCountdown(current.resendAfterSeconds ?? 30, current.codeExpiresAt)

  const verify = useMutation({
    mutationFn: (code: string) => api<OwnerSession>(`/public/registrations/${current.registrationId}/verify`, { method: 'POST', body: { code } }),
    onSuccess: (session) => {
      useOwnerAuth.getState().signIn(session) // the owner is now signed in to "My vehicles"
      onVerified(current.vehicleNumber)
    },
  })

  const resend = useMutation({
    mutationFn: () => api<RegistrationResult>(`/public/registrations/${current.registrationId}/resend`, { method: 'POST' }),
    onSuccess: (result) => {
      setCurrent(result)
      verify.reset()
    },
  })

  const attemptsLeft = verify.error instanceof ApiError ? verify.error.body.attemptsLeft : undefined

  return (
    <Card className="p-5 text-center sm:p-6">
      <div className="mx-auto grid size-14 place-items-center rounded-full bg-brand-50 text-brand-700">
        <MailCheck className="size-7" aria-hidden />
      </div>
      <h1 className="mt-4 text-2xl font-bold text-slate-900">Check your email</h1>
      <p className="mt-1 text-sm text-slate-600">
        We sent a 6-digit code to <strong>{current.codeSentTo}</strong>. It's valid for 10 minutes.
      </p>

      {/* Local demo without real email: the server returns the code so you can test. */}
      {current.devCode && (
        <p className="mx-auto mt-3 inline-flex items-center gap-2 rounded-lg bg-amber-50 px-3 py-1.5 text-xs text-amber-900 ring-1 ring-amber-200">
          <Info className="size-3.5" aria-hidden /> Demo mode (email off): your code is <strong className="font-mono">{current.devCode}</strong>
        </p>
      )}

      <div className="mt-6">
        <CodeInput onComplete={(code) => verify.mutate(code)} disabled={verify.isPending} invalid={!!verify.error} />
      </div>
      {verify.isPending && <p className="mt-3 text-sm text-slate-500">Checking…</p>}
      {verify.error && (
        <p className="mt-3 text-sm font-medium text-red-600" role="alert">
          {errorMessage(verify.error)} {attemptsLeft > 0 && `${attemptsLeft} tries left.`}
        </p>
      )}
      {resend.error && <ErrorBox>{errorMessage(resend.error)}</ErrorBox>}

      <div className="mt-6 flex flex-col items-center gap-2">
        <Button variant="ghost" disabled={secondsLeft > 0} loading={resend.isPending} onClick={() => resend.mutate()}>
          <RotateCw /> {secondsLeft > 0 ? `Resend code in ${secondsLeft}s` : 'Resend code'}
        </Button>
        <Button variant="link" onClick={onBack}>
          Change details
        </Button>
      </div>
    </Card>
  )
}

// ---------------------------------------------------------------- Step 3: done

function Finished({ title, text, againLabel, onAgain }: { title: string; text: string; againLabel: string; onAgain: () => void }) {
  return (
    <Card className="p-6 text-center">
      <div className="mx-auto grid size-16 place-items-center rounded-full bg-emerald-50 text-emerald-600">
        <CheckCircle2 className="size-9" aria-hidden />
      </div>
      <h1 className="mt-4 text-2xl font-bold text-slate-900">{title}</h1>
      <p className="mt-2 text-slate-600">{text}</p>
      <div className="mt-6 flex flex-col gap-2 sm:flex-row sm:justify-center">
        <Button variant="secondary" onClick={onAgain}>
          {againLabel}
        </Button>
        <LinkButton to="/my">View my vehicles</LinkButton>
      </div>
    </Card>
  )
}
