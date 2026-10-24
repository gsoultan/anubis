import { useState, type ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { Collapse, UnstyledButton } from '@mantine/core'
import { IconArrowLeft, IconChevronDown, IconHelpCircle } from '@tabler/icons-react'
import { SectionTabs } from './SectionTabs'
import type { SectionKey } from './nav'

/* One page frame so heading weight, description measure and gutters cannot
   drift between screens. max-w keeps prose readable on wide displays while
   letting tables use the full width.

   `back`, `lead` and `badge` exist for detail pages — a screen about one
   record rather than a list of them. They are the frame's job for the same
   reason the heading is: a person's page and a role's page have to open the
   same way, or the console grows a second dialect. */
export function Page({
  title, description, actions, children, wide = false, back, lead, badge, section, help,
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
  /** The tabs of the section this page belongs to (Roles, Apps, Settings). */
  section?: SectionKey
  /** The long explanation. The description is one line; this sits behind
      "How this works" so the heading costs one line until somebody asks. */
  help?: ReactNode
}) {
  const [helpOpen, setHelpOpen] = useState(false)
  /* The heading block used to run title → description → content on three
     separate rows, which cost about 190px before a table's first row appeared
     — a fifth of a laptop screen spent on chrome. The description now sits on
     the title's own column at a reading measure, and the gutter is a single
     scale (px-5 / pt-5) shared with the header above it, so the page and the
     shell line up on one vertical — px-3 below md, where the shell header
     tightens too, and px-5 from md up.

     Below md the heading and the actions STACK. Side by side, the actions
     kept their full width and the title got the remainder: on Platform users
     at 375px that was about 60px, one word per line. */
  return (
    <div className="fade">
      {section && <div className="px-3 pt-3 md:px-5"><SectionTabs section={section} /></div>}
      <div className="flex flex-col gap-3 px-3 pb-4 pt-4 md:flex-row md:items-start md:justify-between md:gap-6 md:px-5 md:pt-5">
        <div className="flex min-w-0 items-start gap-3">
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
              <p className="t-sm mt-1" style={{ maxWidth: 680 }}>
                {description}
                {help && (
                  <>
                    {' '}
                    <UnstyledButton onClick={() => setHelpOpen((o) => !o)} aria-expanded={helpOpen}
                      className="t-sm inline-flex items-center gap-1 align-baseline"
                      style={{ color: 'var(--accent)' }}>
                      <IconHelpCircle size={13} style={{ alignSelf: 'center' }} />
                      How this works
                      <IconChevronDown size={12} style={{ alignSelf: 'center',
                        transform: helpOpen ? 'rotate(180deg)' : undefined, transition: 'transform var(--t-fast)' }} />
                    </UnstyledButton>
                  </>
                )}
              </p>
            )}
            {help && (
              <Collapse expanded={helpOpen}>
                <div className="t-sm panel-inset mt-2 px-3 py-2.5" style={{ maxWidth: 680 }}>{help}</div>
              </Collapse>
            )}
          </div>
        </div>
        {actions && <div className="flex flex-wrap items-center gap-2 md:shrink-0">{actions}</div>}
      </div>
      <div className={`px-3 pb-10 md:px-5 ${wide ? '' : 'max-w-[1440px]'}`}>{children}</div>
    </div>
  )
}
