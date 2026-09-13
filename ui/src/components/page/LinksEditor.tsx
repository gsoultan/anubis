import { ActionIcon, Button, TextInput, Tooltip } from '@mantine/core'
import { IconPlus, IconTrash } from '@tabler/icons-react'
import { SortableList } from './SortableList'
import type { PageLink } from '@/lib/api/types'

/* The links under the form — the terms, the help desk, the status page.
 *
 * The config has carried these since the page builder shipped and the console
 * had no input for them at all: they could be set through the API and were
 * invisible to the person whose job it is to set them. So this is less a new
 * feature than a missing one.
 *
 * Order matters and is stored, which is why the rows drag. The server caps the
 * list at five and refuses anything that is not http(s) — the same check runs
 * here so the refusal arrives while somebody is typing rather than on save.
 */

const MAX = 5

function urlProblem(url: string): string | null {
  if (url.trim() === '') return 'Required'
  if (!/^https?:\/\//i.test(url)) return 'Must start with http:// or https://'
  return null
}

export function LinksEditor({ links, onChange }: {
  links: PageLink[]
  onChange: (next: PageLink[]) => void
}) {
  const set = (i: number, patch: Partial<PageLink>) =>
    onChange(links.map((l, j) => (i === j ? { ...l, ...patch } : l)))

  const reorder = (from: number, to: number) => {
    const next = [...links]
    const [moved] = next.splice(from, 1)
    if (!moved) return
    next.splice(to, 0, moved)
    onChange(next)
  }

  return (
    <div className="flex flex-col gap-2">
      {links.length === 0 ? (
        <div className="t-xs" style={{ opacity: 0.65 }}>
          No links. Most pages carry one or two — terms, or where to get help
          when the password does not work.
        </div>
      ) : (
        <SortableList items={links} keyOf={(_l, i) => `link-${i}`} onReorder={reorder}>
          {(l, i, handle) => {
            const problem = urlProblem(l.url)
            return (
              <>
                {handle}
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <TextInput
                    size="xs" placeholder="Label" aria-label={`Link ${i + 1} label`}
                    value={l.label}
                    onChange={(e) => set(i, { label: e.currentTarget.value })}
                  />
                  <TextInput
                    size="xs" placeholder="https://example.com/help"
                    aria-label={`Link ${i + 1} URL`}
                    value={l.url}
                    error={l.url !== '' && problem ? problem : undefined}
                    onChange={(e) => set(i, { url: e.currentTarget.value })}
                  />
                </div>
                <Tooltip label="Remove" withArrow>
                  <ActionIcon
                    variant="subtle" size="sm" color="red"
                    aria-label={`Remove link ${i + 1}`}
                    onClick={() => onChange(links.filter((_, j) => j !== i))}
                  >
                    <IconTrash size={14} />
                  </ActionIcon>
                </Tooltip>
              </>
            )
          }}
        </SortableList>
      )}

      <Button
        variant="default" size="compact-xs" leftSection={<IconPlus size={12} />}
        disabled={links.length >= MAX}
        onClick={() => onChange([...links, { label: '', url: '' }])}
      >
        {links.length >= MAX ? 'Five links is the limit' : 'Add link'}
      </Button>
    </div>
  )
}
