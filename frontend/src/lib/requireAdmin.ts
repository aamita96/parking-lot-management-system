import { redirect } from '@tanstack/react-router'
import { refreshAdminToken } from '@/api/client'
import { useAdminAuth } from '@/stores/admin'

/**
 * Put this in every admin page's route as `beforeLoad: requireAdmin`.
 * It runs before the page shows. If nobody is signed in (and the login cookie can't
 * sign them back in), it sends the browser to the admin login page instead.
 */
export async function requireAdmin({ location }: { location: { href: string } }) {
  const signedIn = useAdminAuth.getState().token !== null || (await refreshAdminToken())
  if (!signedIn) {
    throw redirect({ to: '/admin/login', search: { redirect: location.href } })
  }
}
