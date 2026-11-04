import { create } from 'zustand'
import type { AdminSession } from '@/api/types'

// Who is signed in to the admin console.
// The short-lived access token is kept in memory only. After a page reload it is empty, and
// `refreshAdminToken()` in api/client.ts gets a new one using the login cookie.

type AdminAuth = {
  token: string | null
  admin: AdminSession['admin'] | null
  signIn: (session: AdminSession) => void
  signOut: () => void
}

export const useAdminAuth = create<AdminAuth>()((set) => ({
  token: null,
  admin: null,
  signIn: (session) => set({ token: session.accessToken, admin: session.admin }),
  signOut: () => set({ token: null, admin: null }),
}))
