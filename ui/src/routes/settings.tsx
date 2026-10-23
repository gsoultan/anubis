import { createFileRoute, redirect } from '@tanstack/react-router'

/* Settings is a section, not a page: its first tab is Populations. The
   address exists so the sidebar has somewhere to point that will still be
   right if the tabs are ever reordered. */
export const Route = createFileRoute('/settings')({
  beforeLoad: () => { throw redirect({ to: '/realms', replace: true }) },
})
