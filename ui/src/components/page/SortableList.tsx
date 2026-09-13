import { useState } from 'react'
import { IconGripVertical } from '@tabler/icons-react'

/* Drag-to-reorder, with the keyboard path built in rather than bolted on.
 *
 * A handle you can only drag is a control half the people who administer this
 * cannot use: a mouse drag is unavailable to anyone on a keyboard, and it is
 * the first thing to fail on a trackpad in a hurry. So the handle is a real
 * <button> — focus it and Arrow Up / Arrow Down move the row, which is also
 * the faster way to do it once you know.
 *
 * No drag-and-drop library. What this needs is a list of five things and a
 * list of five more; the HTML5 drag events cover it in fewer lines than the
 * dependency's own setup, and they bring their own touch handling.
 */
export function SortableList<T>({
  items, keyOf, onReorder, children, disabled,
}: {
  items: T[]
  keyOf: (item: T, index: number) => string
  onReorder: (from: number, to: number) => void
  children: (item: T, index: number, handle: React.ReactNode) => React.ReactNode
  disabled?: boolean
}) {
  const [dragging, setDragging] = useState<number | null>(null)
  const [over, setOver] = useState<number | null>(null)

  const move = (from: number, to: number) => {
    if (to < 0 || to >= items.length || from === to) return
    onReorder(from, to)
  }

  return (
    <div className="flex flex-col gap-1">
      {items.map((item, i) => {
        const handle = (
          <button
            type="button"
            aria-label="Reorder — drag, or use the arrow keys"
            disabled={disabled}
            className="sort-handle"
            /* The handle is draggable, not the row: a row you can drag from
               anywhere is a row you cannot select text in. */
            draggable={!disabled}
            onDragStart={(e) => {
              setDragging(i)
              e.dataTransfer.effectAllowed = 'move'
              // Firefox ignores a drag that carries no data.
              e.dataTransfer.setData('text/plain', String(i))
            }}
            onDragEnd={() => { setDragging(null); setOver(null) }}
            onKeyDown={(e) => {
              if (e.key === 'ArrowUp') { e.preventDefault(); move(i, i - 1) }
              if (e.key === 'ArrowDown') { e.preventDefault(); move(i, i + 1) }
            }}
          >
            <IconGripVertical size={14} />
          </button>
        )
        return (
          <div
            key={keyOf(item, i)}
            data-dragging={dragging === i ? '' : undefined}
            data-over={over === i && dragging !== null && dragging !== i ? '' : undefined}
            className="sort-row"
            onDragOver={(e) => {
              if (dragging === null) return
              e.preventDefault()
              e.dataTransfer.dropEffect = 'move'
              setOver(i)
            }}
            onDrop={(e) => {
              e.preventDefault()
              if (dragging !== null) move(dragging, i)
              setDragging(null)
              setOver(null)
            }}
          >
            {children(item, i, handle)}
          </div>
        )
      })}
    </div>
  )
}
