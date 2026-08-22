import { create } from 'zustand'
import { persist } from 'zustand/middleware'

/* Zustand holds CLIENT state only. Anything the server owns -- identities,
   grants, axes -- lives in TanStack Query. Duplicating server data here is how
   consoles end up showing two different answers to the same question. */

interface SessionState {
  tenantSlug: string
  /** null = Platform view (super admin); otherwise the tenant being managed. */
  currentTenantId: string | null
  /** Filters the identity list; null = all populations. */
  realmFilter: string | null
  /** Which axis the scope explorer is showing. */
  activeAxis: string | null
  /** Operator-facing density preference; the default is deliberately dense. */
  density: 'compact' | 'comfortable'
  /** First-run guidance on the overview; dismissable, persisted. */
  gettingStartedDismissed: boolean
  setCurrentTenant: (id: string | null) => void
  setRealmFilter: (r: string | null) => void
  setActiveAxis: (a: string | null) => void
  setDensity: (d: 'compact' | 'comfortable') => void
  dismissGettingStarted: () => void
}

export const useSession = create<SessionState>()(
  persist(
    (set) => ({
      tenantSlug: 'impack',
      currentTenantId: 'tnt_impack',
      realmFilter: null,
      activeAxis: null,
      density: 'compact',
      gettingStartedDismissed: false,
      setCurrentTenant: (currentTenantId) => set({ currentTenantId }),
      setRealmFilter: (realmFilter) => set({ realmFilter }),
      setActiveAxis: (activeAxis) => set({ activeAxis }),
      setDensity: (density) => set({ density }),
      dismissGettingStarted: () => set({ gettingStartedDismissed: true }),
    }),
    { name: 'anubis.session' },
  ),
)

/* ---------------------------------------------------------------------------
   Authorization playground draft.

   Not persisted and not in Query: it is a scratchpad an operator builds up
   across several screens (pick an identity here, a scope node there) before
   asking for a verdict. Query would evict it on refetch; the URL would make it
   unwieldy at four-plus axes.
   --------------------------------------------------------------------------- */
interface PlaygroundState {
  subject: string | null
  permission: string | null
  /** axis code -> node id. '_owner' is reserved for self-scoped grants. */
  targets: Record<string, string>
  setSubject: (s: string | null) => void
  setPermission: (p: string | null) => void
  setTarget: (axis: string, nodeId: string | null) => void
  clearTargets: () => void
  reset: () => void
}

export const usePlayground = create<PlaygroundState>((set) => ({
  subject: null,
  permission: null,
  targets: {},
  setSubject: (subject) => set({ subject }),
  setPermission: (permission) => set({ permission }),
  setTarget: (axis, nodeId) =>
    set((s) => {
      const next = { ...s.targets }
      // Removing a target is meaningful, not merely empty: it exercises the
      // fail-closed path, so the UI must be able to express "unset".
      if (nodeId === null) delete next[axis]
      else next[axis] = nodeId
      return { targets: next }
    }),
  clearTargets: () => set({ targets: {} }),
  reset: () => set({ subject: null, permission: null, targets: {} }),
}))
