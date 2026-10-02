import { createFileRoute, redirect } from '@tanstack/react-router'

/* The Access screen is gone. It listed grants — 150,000 of them here — when
   the question operators bring is about a person: what can they do, and how
   do I change it. That lives on the person's own page now, and "who holds
   this role" lives on the role (Roles & permissions → People with this role).

   The address stays, as a redirect: it shipped, so it is in bookmarks,
   runbooks and tickets, and a 404 is a worse answer than the new home. */
export const Route = createFileRoute('/grants')({
  beforeLoad: () => {
    throw redirect({ to: '/identities', replace: true })
  },
})
