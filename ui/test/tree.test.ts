import { describe, expect, test } from 'bun:test'
import { descendants, treeOrder } from '@/lib/tree'

type T = { id: string; p: string; n: string }
const items: T[] = [
  { id: 'b', p: 'a', n: 'Beta' }, { id: 'a', p: '', n: 'Alpha' },
  { id: 'c', p: 'b', n: 'Gamma' }, { id: 'x', p: 'missing', n: 'Orphan' },
]
const order = (l: T[]) => treeOrder(l, (t) => t.id, (t) => t.p, (t) => t.n)

describe('tree order', () => {
  test('each item is followed by what is inside it', () => {
    expect(order(items).map((r) => `${r.item.id}${r.depth}`)).toEqual(['a0', 'b1', 'c2', 'x0'])
  })
  test('an item whose parent is not listed starts its own tree', () => {
    expect(order(items.filter((t) => t.id !== 'a')).map((r) => `${r.item.id}${r.depth}`)).toEqual(['b0', 'c1', 'x0'])
  })
  test('descendants are everything inside, at any depth', () => {
    expect([...descendants(items, (t) => t.id, (t) => t.p, 'a')].sort()).toEqual(['b', 'c'])
  })
})
