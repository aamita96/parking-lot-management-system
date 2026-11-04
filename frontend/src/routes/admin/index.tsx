import { createFileRoute, redirect } from '@tanstack/react-router'

// URL: /admin — there is no page here; go straight to the simulator.
export const Route = createFileRoute('/admin/')({
  beforeLoad: () => {
    throw redirect({ to: '/admin/simulator' })
  },
})
