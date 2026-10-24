/* A flat list with parent links, in tree order: each item followed by what
   sits inside it, with its depth. An item whose parent is not in the list
   starts a tree of its own, which is how an operator who runs only a
   subsidiary sees it at the top. Name order among siblings. */
export function treeOrder<T>(items: T[], id: (t: T) => string, parent: (t: T) => string, name: (t: T) => string): { item: T; depth: number }[] {
  const ids = new Set(items.map(id))
  const kids = new Map<string, T[]>()
  for (const t of items) {
    const p = ids.has(parent(t)) ? parent(t) : ''
    kids.set(p, [...(kids.get(p) ?? []), t])
  }
  const out: { item: T; depth: number }[] = []
  const seen = new Set<string>()
  const walk = (p: string, depth: number) => {
    for (const t of [...(kids.get(p) ?? [])].sort((a, b) => name(a).localeCompare(name(b)))) {
      if (seen.has(id(t))) continue
      seen.add(id(t))
      out.push({ item: t, depth })
      walk(id(t), depth + 1)
    }
  }
  walk('', 0)
  return out
}

/** Everything inside an item, at any depth: where it may not be moved to. */
export function descendants<T>(items: T[], id: (t: T) => string, parent: (t: T) => string, of: string): Set<string> {
  const out = new Set<string>()
  let frontier = [of]
  while (frontier.length) {
    const next = items.filter((t) => frontier.includes(parent(t)) && !out.has(id(t))).map(id)
    next.forEach((n) => out.add(n))
    frontier = next
  }
  return out
}
