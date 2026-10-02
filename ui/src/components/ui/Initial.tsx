import { initials } from '@/lib/access'

/* The initial-avatar, once. The People list and a person's own page draw the
   same person, and a second copy is a second colour rule waiting to drift —
   the mistake `realmKind.ts` already exists to stop. Size is a prop because
   the row wants 26px and the page header wants something you can see.

   The colour is the caller's. The list passes a neutral one: fifty tinted
   avatars would be fifty copies of the population column beside them. */
export function Initial({ name, colour, size = 28 }: {
  name: string
  colour: string
  size?: number
}) {
  const text = initials(name)
  return (
    <span
      aria-hidden
      className="avatar"
      style={{
        color: colour,
        background: `color-mix(in srgb, ${colour} 13%, transparent)`,
        borderColor: `color-mix(in srgb, ${colour} 26%, transparent)`,
        width: size,
        height: size,
        fontSize: Math.round(size * (text.length > 1 ? 0.36 : 0.4)),
        borderRadius: size >= 40 ? 'var(--r-lg)' : 'var(--r-sm)',
      }}
    >
      {text}
    </span>
  )
}
