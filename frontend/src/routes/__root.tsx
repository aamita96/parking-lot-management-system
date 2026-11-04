import { createRootRoute, Outlet } from '@tanstack/react-router'
import { Toaster } from 'sonner'
import { LinkButton } from '@/components/ui'

// The outermost component, shown around every page.
// <Outlet /> is where the current page is drawn. <Toaster /> shows pop-up messages (toast.success(...)).
export const Route = createRootRoute({
  component: () => (
    <>
      <Outlet />
      <Toaster richColors position="top-right" closeButton />
    </>
  ),
  notFoundComponent: () => (
    <div className="grid min-h-screen place-items-center px-6 text-center">
      <div>
        <p className="text-6xl font-extrabold text-brand-800">404</p>
        <p className="mt-2 text-slate-600">This page doesn't exist.</p>
        <LinkButton to="/" className="mt-6">
          Back to availability
        </LinkButton>
      </div>
    </div>
  ),
})
