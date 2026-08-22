import {
  IconBuilding, IconTruck, IconBox, IconUsers, IconWallet,
  IconMapPin, IconTag, IconCircleDashed,
} from '@tabler/icons-react'
import type { ComponentType } from 'react'

/* Maps ui_schema.icon (a server-supplied string) to a component.
   Note what this is NOT: a switch on axis code. An unknown icon degrades to a
   neutral glyph so a newly registered axis renders correctly on day one,
   before anyone has chosen an icon for it. */
const REGISTRY: Record<string, ComponentType<{ size?: number; stroke?: number }>> = {
  building: IconBuilding,
  truck: IconTruck,
  box: IconBox,
  users: IconUsers,
  wallet: IconWallet,
  location: IconMapPin,
  tag: IconTag,
}

export function AxisIcon({ name, size = 16 }: { name?: string | undefined; size?: number }) {
  const Cmp = (name && REGISTRY[name]) || IconCircleDashed
  return <Cmp size={size} stroke={1.6} />
}
