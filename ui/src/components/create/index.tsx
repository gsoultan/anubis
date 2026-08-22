import { useEffect } from 'react'
import { useCreate, type CreateKind } from '@/stores/create'
import { CreateIdentity } from './CreateIdentity'
import { CreatePermission } from './CreatePermission'
import { CreateRole } from './CreateRole'
import { CreateGrant } from './CreateGrant'
import { CreateNode } from './CreateNode'
import { CreateAxis } from './CreateAxis'
import { CreateMembership } from './CreateMembership'
import { EditRole } from './EditRole'
import { CreateSyncSource } from './CreateSyncSource'

const KINDS: CreateKind[] = ['identity', 'permission', 'role', 'grant', 'node', 'axis', 'membership']  // editRole opens only with ctx, not via ?new=

/* Mounted once in the root layout. Also honours ?new=<kind> so create flows
   are deep-linkable — a runbook can say "click here to add an identity". */
export function CreateDrawers() {
  const { open, openCreate } = useCreate()

  useEffect(() => {
    const wanted = new URLSearchParams(window.location.search).get('new')
    if (wanted && (KINDS as string[]).includes(wanted)) openCreate(wanted as CreateKind)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <>
      <CreateIdentity opened={open === 'identity'} />
      <CreatePermission opened={open === 'permission'} />
      <CreateRole opened={open === 'role'} />
      <CreateGrant opened={open === 'grant'} />
      <CreateNode opened={open === 'node'} />
      <CreateAxis opened={open === 'axis'} />
      <CreateMembership opened={open === 'membership'} />
      <EditRole opened={open === 'editRole'} />
      <CreateSyncSource opened={open === 'syncSource'} />
    </>
  )
}
