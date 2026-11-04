import { useMutation, useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { Copy, KeyRound, Plus, Power, RotateCw } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { api } from '@/api/client'
import type { GateDevice, Items } from '@/api/types'
import { AdminLayout } from '@/components/layout/AdminLayout'
import { Badge, Button, Card, CardHeader, ErrorBox, Field, Input, MessageRow, Modal, ModalFooter, Select, Table } from '@/components/ui'
import { errorMessage } from '@/lib/errors'
import { ago } from '@/lib/format'
import { keys, queryClient } from '@/lib/queryClient'
import { requireAdmin } from '@/lib/requireAdmin'

// URL: /admin/gates — gate scanners and their API keys (FR-37).
// A key is shown only once, right after it is created or rotated; the server stores a hash.
export const Route = createFileRoute('/admin/gates')({
  beforeLoad: requireAdmin,
  component: GatesPage,
})

function GatesPage() {
  const [newKey, setNewKey] = useState<GateDevice | null>(null) // device whose key we must show once
  const gates = useQuery({ queryKey: keys.gates, queryFn: () => api<Items<GateDevice>>('/admin/gate-devices', { auth: 'admin' }), refetchInterval: 10000 })
  const refresh = () => queryClient.invalidateQueries({ queryKey: keys.gates })

  const rotateKey = useMutation({
    mutationFn: (gate: GateDevice) => api<GateDevice>(`/admin/gate-devices/${gate.id}/rotate-key`, { method: 'POST', auth: 'admin' }),
    onSuccess: (gate) => {
      setNewKey(gate)
      refresh()
    },
    onError: (err) => toast.error(errorMessage(err)),
  })

  const toggleActive = useMutation({
    mutationFn: (gate: GateDevice) => api<GateDevice>(`/admin/gate-devices/${gate.id}`, { method: 'PATCH', auth: 'admin', body: { active: !gate.active } }),
    onSuccess: (gate) => {
      toast.success(`${gate.name} ${gate.active ? 'activated' : 'deactivated'}`)
      refresh()
    },
    onError: (err) => toast.error(errorMessage(err)),
  })

  return (
    <AdminLayout title="Gate devices" description="Each scanner calls the gate API with its own key (X-Gate-Key). Keys are stored hashed and shown once.">
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <Card>
          {gates.error && <ErrorBox>{errorMessage(gates.error)}</ErrorBox>}
          <Table>
            <thead>
              <tr>
                <th>Device</th>
                <th>Kind</th>
                <th>Source</th>
                <th>Status</th>
                <th>Last seen</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {gates.isLoading && <MessageRow columns={6} loading />}
              {gates.data?.items.map((gate) => (
                <tr key={gate.id}>
                  <td className="font-mono font-bold">{gate.name}</td>
                  <td>
                    <Badge color={gate.kind === 'ENTRY' ? 'blue' : 'green'}>{gate.kind === 'ENTRY' ? 'Entry (IN)' : 'Exit (PAY/OUT)'}</Badge>
                  </td>
                  <td>{gate.isSimulator ? <Badge color="violet">Simulator</Badge> : <Badge>Scanner</Badge>}</td>
                  <td>{gate.active ? <Badge color="green">Active</Badge> : <Badge color="red">Inactive</Badge>}</td>
                  <td>{gate.lastSeenAt ? ago(gate.lastSeenAt) : 'never'}</td>
                  <td className="whitespace-nowrap text-right">
                    {!gate.isSimulator && (
                      <Button
                        size="small"
                        variant="secondary"
                        onClick={() => confirm(`Rotate the key for ${gate.name}? The old key stops working at once.`) && rotateKey.mutate(gate)}
                      >
                        <RotateCw /> Rotate key
                      </Button>
                    )}
                    <Button size="small" variant="ghost" onClick={() => toggleActive.mutate(gate)}>
                      <Power /> {gate.active ? 'Deactivate' : 'Activate'}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Card>
        <AddGate
          onCreated={(gate) => {
            setNewKey(gate)
            refresh()
          }}
        />
      </div>

      {newKey && <ShowKeyOnce gate={newKey} onClose={() => setNewKey(null)} />}
    </AdminLayout>
  )
}

function AddGate({ onCreated }: { onCreated: (gate: GateDevice) => void }) {
  const [name, setName] = useState('')
  const [kind, setKind] = useState('ENTRY')
  const create = useMutation({
    mutationFn: () => api<GateDevice>('/admin/gate-devices', { method: 'POST', auth: 'admin', body: { name, kind } }),
    onSuccess: (gate) => {
      setName('')
      onCreated(gate)
    },
  })
  return (
    <Card className="h-fit">
      <CardHeader title="Register a device" />
      <form
        className="space-y-3 px-5 py-4"
        onSubmit={(e) => {
          e.preventDefault()
          create.mutate()
        }}
      >
        <Field label="Name" hint="For example IN-2 or OUT-2.">
          <Input required value={name} onChange={(e) => setName(e.target.value.toUpperCase())} className="font-mono" />
        </Field>
        <Field label="Kind">
          <Select value={kind} onChange={(e) => setKind(e.target.value)} className="w-full">
            <option value="ENTRY">Entry scanner (IN gate)</option>
            <option value="EXIT">Exit scanner + PAY kiosk</option>
          </Select>
        </Field>
        {create.error && <ErrorBox>{errorMessage(create.error)}</ErrorBox>}
        <Button type="submit" className="w-full" loading={create.isPending}>
          <Plus /> Create and show key
        </Button>
      </form>
    </Card>
  )
}

function ShowKeyOnce({ gate, onClose }: { gate: GateDevice; onClose: () => void }) {
  const path = gate.kind === 'EXIT' ? 'exit-scans' : 'entry-scans'
  return (
    <Modal open onClose={onClose} title={`API key for ${gate.name}`} description="Copy it now and install it on the device. It won't be shown again." icon={<KeyRound />} color="warning">
      <div className="flex items-center gap-2 rounded-lg bg-slate-900 p-3">
        <code className="min-w-0 flex-1 break-all font-mono text-xs text-emerald-300">{gate.apiKey}</code>
        <Button
          size="icon"
          variant="ghost"
          className="text-white hover:bg-white/10"
          aria-label="Copy key"
          onClick={() => navigator.clipboard.writeText(gate.apiKey ?? '').then(() => toast.success('Copied'))}
        >
          <Copy />
        </Button>
      </div>
      <pre className="mt-3 overflow-x-auto rounded-lg bg-slate-50 p-3 text-[11px] text-slate-600">
        {`curl -X POST /api/v1/gate/${path} \\\n  -H "X-Gate-Key: <key>" -H "Idempotency-Key: <uuid>" \\\n  -d '{"plate":"KA01AB1234"}'`}
      </pre>
      <ModalFooter>
        <Button onClick={onClose}>Done</Button>
      </ModalFooter>
    </Modal>
  )
}
