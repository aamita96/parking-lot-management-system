import { QueryClient } from '@tanstack/react-query'
import { ApiError } from '@/api/client'

// One shared React Query client. It caches API answers and refetches them when needed.
// It lives in its own file so non-React code (like the simulator) can refresh data too.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 2000,
      // Retry server or network errors twice; don't retry 4xx errors (they won't fix themselves).
      retry: (count, err) => !(err instanceof ApiError && err.status >= 400 && err.status < 500) && count < 2,
    },
  },
})

// Names for cached data. Refreshing a name re-downloads it everywhere it's shown.
export const keys = {
  availability: ['availability'],
  config: ['config'],
  layout: ['layout'],
  events: ['events'],
  tickets: ['tickets'],
  exceptions: ['exceptions'],
  vehicles: ['vehicles'],
  owners: ['owners'],
  spots: ['spots'],
  pricing: ['pricing'],
  settings: ['settings'],
  gates: ['gates'],
  audit: ['audit'],
  me: ['me'],
  myVehicles: ['my-vehicles'],
  myTickets: ['my-tickets'],
}

/** Re-download everything a gate scan, payment or admin action can change. */
export function refreshLotData() {
  for (const key of [keys.layout, keys.events, keys.availability, keys.tickets, keys.exceptions, keys.spots, keys.vehicles, keys.owners, keys.audit]) {
    queryClient.invalidateQueries({ queryKey: key })
  }
}
