import { ActionIcon, Tooltip } from '@mantine/core'
import { IconEye, IconEyeOff, IconPlus } from '@tabler/icons-react'
import { SortableList } from './SortableList'
import { PAGE_SECTIONS } from '@/lib/api/types'
import type { PageKind, PageSection } from '@/lib/api/types'

/* The order of the blocks on the page, and which of them are on it.
 *
 * `sections` is a list of names the server iterates (pagecfg), so what is
 * dragged here is exactly what is rendered there — this is not a canvas that
 * approximates a layout. That constraint is the point: the config can never
 * become markup, because it can only ever be five names in an order.
 *
 * Hidden blocks sit apart rather than greyed out in place. A block that is not
 * on the page has no position ON the page, and pretending it does — letting
 * somebody drag a hidden row and watching it snap back — is a lie the list
 * tells about itself.
 */

const LABEL: Record<PageSection, { name: string; hint: string }> = {
  logo: { name: 'Logo', hint: 'The image, or the initial when no logo URL is set.' },
  heading: { name: 'Headline', hint: 'The large line. Sign-out pages use a different one per step.' },
  subheading: { name: 'Description', hint: 'Only renders when the description has text.' },
  form: { name: 'Form', hint: 'The fields and the button.' },
  links: { name: 'Links', hint: 'Your links, plus the registration link when it is switched on.' },
}

export function SectionOrder({ sections, kind, onChange }: {
  sections: PageSection[] | undefined
  kind: PageKind
  onChange: (next: PageSection[]) => void
}) {
  const visible: PageSection[] =
    sections?.length ? sections.filter((s) => PAGE_SECTIONS.includes(s)) : [...PAGE_SECTIONS]
  const hidden = PAGE_SECTIONS.filter((s) => !visible.includes(s))

  const reorder = (from: number, to: number) => {
    const next = [...visible]
    const [moved] = next.splice(from, 1)
    if (!moved) return
    next.splice(to, 0, moved)
    onChange(next)
  }

  /* Restoring puts a block back where it belongs rather than at the end: a
     logo that reappears under the button is not what "show" meant. */
  const show = (s: PageSection) => {
    const home = PAGE_SECTIONS.indexOf(s)
    const at = visible.findIndex((v) => PAGE_SECTIONS.indexOf(v) > home)
    const next = [...visible]
    next.splice(at === -1 ? next.length : at, 0, s)
    onChange(next)
  }

  return (
    <div className="flex flex-col gap-2">
      <SortableList
        items={visible}
        keyOf={(s) => s}
        onReorder={reorder}
      >
        {(s, _i, handle) => (
          <>
            {handle}
            <div className="min-w-0 flex-1">
              <div className="t-body truncate" style={{ fontWeight: 550 }}>
                {s === 'form' && kind === 'signout' ? 'Sign-out form' : LABEL[s].name}
              </div>
              <div className="t-xs truncate" style={{ opacity: 0.62 }}>{LABEL[s].hint}</div>
            </div>
            {s === 'form' ? (
              <Tooltip label="The form is the page — it cannot be removed." withArrow>
                <span><ActionIcon variant="subtle" size="sm" disabled><IconEye size={14} /></ActionIcon></span>
              </Tooltip>
            ) : (
              <Tooltip label="Hide this block" withArrow>
                <ActionIcon
                  variant="subtle" size="sm" color="gray"
                  aria-label={`Hide ${LABEL[s].name}`}
                  onClick={() => onChange(visible.filter((v) => v !== s))}
                >
                  <IconEyeOff size={14} />
                </ActionIcon>
              </Tooltip>
            )}
          </>
        )}
      </SortableList>

      {hidden.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="t-xs" style={{ opacity: 0.6 }}>Not on the page:</span>
          {hidden.map((s) => (
            <button
              key={s}
              type="button"
              className="chip"
              style={{ cursor: 'pointer', display: 'inline-flex', alignItems: 'center', gap: 4 }}
              onClick={() => show(s)}
            >
              <IconPlus size={11} /> {LABEL[s].name}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
