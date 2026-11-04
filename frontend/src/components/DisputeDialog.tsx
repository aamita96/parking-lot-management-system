import { useMutation } from '@tanstack/react-query'
import { ShieldAlert } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { ApiError, api } from '@/api/client'
import { Button, ErrorBox, Field, Input, Modal, ModalFooter, Textarea } from '@/components/ui'
import { errorMessage } from '@/lib/errors'

/** "Report a problem" when someone else registered your vehicle number (FR-5). */
export function DisputeDialog({ open, onClose, vehicleNumber, email }: { open: boolean; onClose: () => void; vehicleNumber: string; email: string }) {
  const [reporterEmail, setReporterEmail] = useState(email)
  const [mobile, setMobile] = useState('')
  const [registrationId, setRegistrationId] = useState('')
  const [message, setMessage] = useState('')

  const send = useMutation({
    mutationFn: () =>
      api<{ message: string }>('/public/disputes', {
        method: 'POST',
        body: { vehicleNumber, reporterEmail, mobile: mobile || null, registrationId: registrationId || null, message },
      }),
    onSuccess: (result) => {
      toast.success('Problem reported', { description: result.message })
      onClose()
    },
  })
  const fieldError = (field: string) => (send.error instanceof ApiError ? send.error.fieldError(field) : undefined)

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Report a problem"
      description={`Tell us why ${vehicleNumber} should be registered to you. The car park office checks the registration (RC) and contacts you.`}
      icon={<ShieldAlert />}
      color="warning"
    >
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault()
          send.mutate()
        }}
      >
        <Field label="Your email" error={fieldError('reporterEmail')}>
          <Input type="email" required value={reporterEmail} onChange={(e) => setReporterEmail(e.target.value)} />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Mobile" optional error={fieldError('mobile')}>
            <Input type="tel" value={mobile} onChange={(e) => setMobile(e.target.value)} />
          </Field>
          <Field label="Registration (RC) number" optional error={fieldError('registrationId')}>
            <Input value={registrationId} onChange={(e) => setRegistrationId(e.target.value)} />
          </Field>
        </div>
        <Field label="What happened?" error={fieldError('message')}>
          <Textarea maxLength={1000} value={message} onChange={(e) => setMessage(e.target.value)} placeholder="This is my car; I didn't register it." />
        </Field>
        {send.error && !fieldError('reporterEmail') && <ErrorBox>{errorMessage(send.error)}</ErrorBox>}
        <ModalFooter>
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" loading={send.isPending}>
            Send report
          </Button>
        </ModalFooter>
      </form>
    </Modal>
  )
}
