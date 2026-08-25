import { useSyncExternalStore } from 'react'
import { redirect } from '@tanstack/react-router'
import { isAuthenticated, onSessionChange, platformLogin, platformLogout, platformVerifyMfa } from '@/lib/anubis'

/* Unlike everything else under stores/, this is NOT a zustand store.
   The tokens live in lib/anubis.ts — module state plus sessionStorage —
   because the refresh interceptor has to read and replace them outside React
   entirely, and it can clear the session on its own when the server reports a
   stolen refresh token. Copying that into a second store is exactly the
   "two different answers to the same question" the session store warns about,
   so this subscribes to the one source instead of mirroring it. */

const WHO_KEY = 'anubis.who'

/** Who is signed in. Kept beside the tokens rather than decoded from them:
    the access token is a PASETO the browser has no business opening. */
export type Who = { tenant: string; username: string; owner: boolean }

const whoListeners = new Set<() => void>()
let who: Who | null = readWho()

function readWho(): Who | null {
  try {
    const raw = sessionStorage.getItem(WHO_KEY)
    return raw ? (JSON.parse(raw) as Who) : null
  } catch {
    return null
  }
}

function setWho(next: Who | null) {
  who = next
  if (next) sessionStorage.setItem(WHO_KEY, JSON.stringify(next))
  else sessionStorage.removeItem(WHO_KEY)
  for (const fn of whoListeners) fn()
}

/** True while a session exists. Re-renders when the refresh path drops it. */
export function useAuthed(): boolean {
  return useSyncExternalStore(onSessionChange, isAuthenticated, () => false)
}

/** True for an installation owner: the tenants that exist and who operates
    the platform are theirs, and nobody else's. */
export function useIsOwner(): boolean {
  return useWho()?.owner === true
}

export function useWho(): Who | null {
  return useSyncExternalStore(
    (fn) => {
      whoListeners.add(fn)
      return () => whoListeners.delete(fn)
    },
    () => who,
    () => null,
  )
}

/** Sign in a platform user. The console administers the installation, so its
    door is the platform one; a tenant's people never sign in here. */
export async function signIn(username: string, password: string) {
  const res = await platformLogin(username, password)
  // A challenge is not a session: nothing is remembered until it is answered.
  if (res.mfa) return res
  setWho({ tenant: '', username: res.username, owner: res.owner })
  return res
}

export async function completeMfa(mfaToken: string, code: string) {
  const who = await platformVerifyMfa(mfaToken, code)
  setWho({ tenant: '', username: who.username, owner: who.owner })
  return who
}

export async function signOut() {
  // Ends the session server-side too: the refresh family dies, so a leaked
  // token pair is worthless the moment the operator signs out.
  await platformLogout()
  setWho(null)
}

/* Route guard.

   Deliberately per-route rather than global. Only the screens that have been
   migrated onto the real Connect client need a session; the rest still read
   from the in-memory mock and work with no server running at all, which is
   how the console is developed today. Gating the whole app would take that
   away to protect screens that hold no real data.

   As each screen moves off lib/api/client onto lib/anubis, add this to its
   route and it starts demanding a session too. */
export function requireSession({ location }: { location: { href: string } }) {
  if (!isAuthenticated()) {
    throw redirect({ to: '/signin', search: { next: location.href } })
  }
}
