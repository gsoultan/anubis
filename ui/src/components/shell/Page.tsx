import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { IconArrowLeft } from '@tabler/icons-react'

/* One page frame so heading weight, description measure and gutters cannot
   drift between screens. max-w keeps prose readable on wide displays while
   letting tables use the full width.

   `back`, `lead` and `badge` exist for detail pages — a screen about one
   record rather than a list of them. They are the frame's job for the same
   reason the heading is: a person's page and a role's page have to open the
   same way, or the console grows a second dialect. */
export function Page({
  title, description, actions, children, wide = false, back, lead, badge,
}: {
  title: string
  description?: ReactNode
  actions?: ReactNode
  children: ReactNode
  wide?: boolean
  /** Where this record lives. A detail page is always somewhere you arrived at. */
  back?: { to: string; label: string }
  /** Rendered left of the heading block — an avatar, an icon. */
  lead?: ReactNode
  /** Rendered on the heading's baseline — status, assurance. Facts, not actions. */
  badge?: ReactNode
}) {
  return (
    <div className="fade">
      <div className="flex items-start justify-between gap-6 px-6 pb-5 pt-6">
        <div className="flex min-w-0 items-start gap-3.5">
          {lead}
          <div className="min-w-0">
            {back && (
              <Link
                to={back.to}
                className="t-xs mb-1 inline-flex items-center gap-1 no-underline"
                style={{ color: 'var(--ink-3)' }}
              >
                <IconArrowLeft size={12} />
                {back.label}
              </Link>
            )}
            <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
              <h1 className="t-display">{title}</h1>
              {badge}
            </div>
            {description && (
              <p className="t-sm mt-1.5" style={{ maxWidth: 640 }}>{description}</p>
            )}
          </div>
        </div>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </div>
      <div className={`px-6 pb-10 ${wide ? '' : 'max-w-[1400px]'}`}>{children}</div>
    </div>
  )
}
