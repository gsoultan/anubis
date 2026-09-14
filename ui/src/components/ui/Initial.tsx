/* The initial-avatar, once. The People list and a person's own page draw the
   same person, and a second copy is a second colour rule waiting to drift —
   the mistake `realmKind.ts` already exists to stop. Size is a prop because
   the row wants 28px and the page header wants something you can see. */
export function Initial({ name, colour, size = 28 }: {
  name: string
  colour: string
  size?: number
}) {
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
        fontSize: Math.round(size * 0.4),
        borderRadius: size >= 40 ? 'var(--r-lg)' : 'var(--r-sm)',
      }}
    >
      {name.slice(0, 1)}
    </span>
  )
}
