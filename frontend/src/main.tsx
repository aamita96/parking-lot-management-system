// The app starts here. index.html loads this file, and it draws the app into <div id="root">.
//
// Which page shows for which URL is decided by the files in src/routes/:
//   routes/index.tsx          → /
//   routes/register.tsx       → /register
//   routes/admin/tickets.tsx  → /admin/tickets
// The Vite plugin reads that folder and writes routeTree.gen.ts (never edit that file by hand).

import { QueryClientProvider } from '@tanstack/react-query'
import { createRouter, RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { queryClient } from './lib/queryClient'
import { routeTree } from './routeTree.gen'
import './styles.css'

const router = createRouter({ routeTree, scrollRestoration: true })

// Lets TypeScript check every <Link to="..."> against the real list of pages.
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
)
