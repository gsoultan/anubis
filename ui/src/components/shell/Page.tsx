import type { ReactNode } from 'react'

/* One page frame so heading weight, description measure and gutters cannot
   drift between screens. max-w keeps prose readable on wide displays while
   letting tables use the full width. */
export function Page({
  title, description, actions, children, wide = false,
}: {
  title: string
  description?: ReactNode
  actions?: ReactNode
  children: ReactNode
  wide?: boolean
}) {
  return (
    <div className="fade">
      <div className="flex items-start justify-between gap-6 px-6 pb-5 pt-6">
        <div className="min-w-0">
          <h1 className="t-display">{title}</h1>
          {description && (
            <p className="t-sm mt-1.5" style={{ maxWidth: 640 }}>{description}</p>
          )}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </div>
      <div className={`px-6 pb-10 ${wide ? '' : 'max-w-[1400px]'}`}>{children}</div>
    </div>
  )
}
