import { useMutation } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { LockKeyhole } from 'lucide-react'
import { useState } from 'react'
import { api } from '@/api/client'
import type { AdminSession } from '@/api/types'
import { Logo } from '@/components/brand/Logo'
import { Button, Card, ErrorBox, Field, Input } from '@/components/ui'
import { errorMessage } from '@/lib/errors'
import { useAdminAuth } from '@/stores/admin'

// URL: /admin/login  (?redirect=/admin/tickets sends you back there after signing in)
export const Route = createFileRoute('/admin/login')({
  validateSearch: (search: Record<string, unknown>): { redirect?: string } => ({
    redirect: typeof search.redirect === 'string' ? search.redirect : undefined,
  }),
  component: AdminLogin,
})

function AdminLogin() {
  const navigate = useNavigate()
  const { redirect } = Route.useSearch()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')

  const login = useMutation({
    mutationFn: () => api<AdminSession>('/admin/auth/login', { method: 'POST', body: { email, password } }),
    onSuccess: (session) => {
      useAdminAuth.getState().signIn(session) // the server also set the refresh cookie
      navigate({ to: redirect?.startsWith('/admin') ? redirect : '/admin/simulator' })
    },
  })

  return (
    <div className="grid min-h-screen place-items-center bg-gradient-to-br from-brand-900 via-brand-800 to-slate-900 px-4">
      <Card className="w-full max-w-sm p-6">
        <Link to="/">
          <Logo subtitle="Admin console" />
        </Link>
        <h1 className="mt-6 text-xl font-bold">Sign in</h1>
        <p className="text-sm text-slate-500">Manage spots, pricing, gates and exceptions, and run the simulator.</p>
        <form
          className="mt-5 space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            login.mutate()
          }}
        >
          <Field label="Email">
            <Input type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} />
          </Field>
          <Field label="Password">
            <Input type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
          </Field>
          {login.error && <ErrorBox>{errorMessage(login.error)}</ErrorBox>}
          <Button type="submit" className="w-full" loading={login.isPending}>
            <LockKeyhole /> Sign in
          </Button>
        </form>
        <p className="mt-4 text-xs text-slate-400">Local default: admin@plms.local / admin12345 (set in .env).</p>
      </Card>
    </div>
  )
}
