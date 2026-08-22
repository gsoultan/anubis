import { create } from 'zustand'

/* Which create-drawer is open, plus context a caller can preload — "add a node
   under THIS parent", "grant a role to THIS identity". One global store so any
   screen (or the command palette, or a ?new= deep link) can start a create
   flow without prop-drilling. */
export type CreateKind =
  | 'identity' | 'permission' | 'role' | 'editRole' | 'grant' | 'node' | 'axis' | 'membership' | 'syncSource'

export interface CreateCtx {
  identityId?: string
  roleId?: string
  axisCode?: string
  parentId?: string
}

interface CreateState {
  open: CreateKind | null
  ctx: CreateCtx
  openCreate: (kind: CreateKind, ctx?: CreateCtx) => void
  close: () => void
}

export const useCreate = create<CreateState>((set) => ({
  open: null,
  ctx: {},
  openCreate: (kind, ctx) => set({ open: kind, ctx: ctx ?? {} }),
  close: () => set({ open: null, ctx: {} }),
}))
