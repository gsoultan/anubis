import { useEffect, useState } from 'react'
import { useMediaQuery } from '@mantine/hooks'

const KEY = 'anubis.nav.folded'

function readChoice(): boolean | null {
  try {
    const v = localStorage.getItem(KEY)
    return v === '1' ? true : v === '0' ? false : null
  } catch {
    return null
  }
}

/* Whether the sidebar is folded to its icon rail.

   Between 768 and 1280px the full column costs a sixth of the screen, so it
   folds by itself there — until somebody chooses. A choice is remembered and
   wins from then on, at every width: someone who unfolded it on a laptop
   meant it. Storage can be missing (private windows, blocked site data), and
   then the width alone decides, which is still a working sidebar. */
export function useFold(): [boolean, (folded: boolean) => void] {
  const narrow = useMediaQuery('(max-width: 80em)') ?? false
  const [choice, setChoice] = useState<boolean | null>(readChoice)
  useEffect(() => {
    const onStorage = (e: StorageEvent) => { if (e.key === KEY) setChoice(readChoice()) }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])
  const set = (folded: boolean) => {
    setChoice(folded)
    try { localStorage.setItem(KEY, folded ? '1' : '0') } catch { /* width decides */ }
  }
  return [choice ?? narrow, set]
}
