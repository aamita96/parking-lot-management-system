import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { Owner, OwnerSession } from '@/api/types'

// Who is signed in to "My vehicles". Owners have no password: they get a token after
// entering an email code. `persist` saves it in localStorage so it survives a page reload.
//
// Read it in a component:      const owner = useOwnerAuth((s) => s.owner)
// Read it outside React:       useOwnerAuth.getState().token

type OwnerAuth = {
  token: string | null
  owner: Owner | null
  signIn: (session: OwnerSession) => void
  signOut: () => void
}

export const useOwnerAuth = create<OwnerAuth>()(
  persist(
    (set) => ({
      token: null,
      owner: null,
      signIn: (session) => set({ token: session.ownerToken, owner: session.owner }),
      signOut: () => set({ token: null, owner: null }),
    }),
    { name: 'plms-owner' },
  ),
)
