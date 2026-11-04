import { useAdminAuth } from '@/stores/admin'
import { useOwnerAuth } from '@/stores/owner'
import type { AdminSession } from './types'

// Every call to the Go API goes through `api()` below.
//
//   const tickets = await api<Page<Ticket>>('/admin/tickets', { auth: 'admin' })
//
// In development Vite forwards /api to the Go server; in Docker nginx does (see nginx.conf).

/** An error answer from the API. `code` is a stable name like "NOT_REGISTERED". */
export class ApiError extends Error {
  status: number
  code: string
  body: Record<string, any> // the full error body, e.g. body.registerUrl or body.receipt

  constructor(status: number, body: Record<string, any>) {
    super(body.detail ?? `Request failed (${status})`)
    this.status = status
    this.code = body.code ?? 'ERROR'
    this.body = body
  }

  /** Message for one form field, e.g. fieldError('ownerEmail'). */
  fieldError(field: string): string | undefined {
    const errors: { field: string; message: string }[] = this.body.errors ?? []
    return errors.find((e) => e.field === field)?.message
  }
}

type Options = {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  body?: unknown
  auth?: 'owner' | 'admin' // which login token to send
  headers?: Record<string, string>
}

export async function api<T>(path: string, options: Options = {}): Promise<T> {
  let response = await send(path, options)

  // Admin access tokens last 15 minutes. If one expired, get a new one and try once more.
  if (response.status === 401 && options.auth === 'admin' && (await refreshAdminToken())) {
    response = await send(path, options)
  }
  // An owner token that no longer works: sign the owner out.
  if (response.status === 401 && options.auth === 'owner') {
    useOwnerAuth.getState().signOut()
  }

  if (response.status === 204) return undefined as T // "No Content"
  const text = await response.text()
  const data = text ? JSON.parse(text) : {}
  if (!response.ok) throw new ApiError(response.status, data)
  return data as T
}

async function send(path: string, options: Options): Promise<Response> {
  const headers: Record<string, string> = { ...options.headers }
  if (options.body !== undefined) headers['Content-Type'] = 'application/json'

  const token = options.auth === 'owner' ? useOwnerAuth.getState().token : options.auth === 'admin' ? useAdminAuth.getState().token : null
  if (token) headers.Authorization = `Bearer ${token}`

  try {
    return await fetch(`/api/v1${path}`, {
      method: options.method ?? 'GET',
      headers,
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
    })
  } catch {
    throw new ApiError(0, { code: 'NETWORK', detail: "Can't reach the server." })
  }
}

/**
 * Gets a new admin access token using the refresh cookie the server set at login.
 * The cookie is HttpOnly, so JavaScript can't read it; the browser sends it automatically.
 * Returns true if the admin is still signed in.
 */
let refreshInProgress: Promise<boolean> | null = null

export function refreshAdminToken(): Promise<boolean> {
  // If several requests fail at once, they all wait for the same refresh.
  if (!refreshInProgress) {
    refreshInProgress = fetch('/api/v1/admin/auth/refresh', { method: 'POST' })
      .then(async (response) => {
        if (!response.ok) {
          useAdminAuth.getState().signOut()
          return false
        }
        useAdminAuth.getState().signIn((await response.json()) as AdminSession)
        return true
      })
      .catch(() => false)
      .finally(() => (refreshInProgress = null))
  }
  return refreshInProgress
}

/** Builds "?a=1&b=2", leaving out empty values. */
export function queryString(params: Record<string, string | number | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== '') search.set(key, String(value))
  }
  const text = search.toString()
  return text ? `?${text}` : ''
}
