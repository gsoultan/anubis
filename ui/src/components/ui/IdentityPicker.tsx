import { useState } from 'react'
import { Select } from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import * as live from '@/lib/api/live'

/* A person picker that SEARCHES rather than loads.

   Every one of these used to call api.identities(), which walks pages until
   it hits a 2,000-row cap — over a directory of 57,000 people that is both
   slow and wrong: the person you are looking for is frequently not in the
   list at all. The server already searches by username and email, so this
   asks it, debounced, and shows what comes back. */
export function IdentityPicker({
  value, onChange, realmId, placeholder = 'Search for a person…', label, error, size = 'sm',
}: {
  value: string | null
  onChange: (id: string | null, username: string) => void
  /** Restrict to one population. Omit to search them all. */
  realmId?: string
  placeholder?: string
  label?: string
  error?: string
  size?: string
}) {
  const [search, setSearch] = useState('')
  const [debounced] = useDebouncedValue(search, 250)
  const { data } = useQuery({
    queryKey: ['identity-search', realmId ?? '', debounced],
    queryFn: () => live.identitiesPage(realmId, debounced || undefined, '', 20),
    placeholderData: (prev) => prev,
  })
  const rows = data?.rows ?? []

  /* A value chosen elsewhere ("grant a role to THIS person") is usually not
     in the current search results, and a Select whose value has no matching
     option renders blank — which reads as "nothing selected". Fetch that one
     person so the field shows who it is holding. */
  const missing = !!value && !rows.some((i) => i.id === value)
  const { data: selected } = useQuery({
    queryKey: ['identity-selected', value],
    queryFn: () => live.identity(value as string),
    enabled: missing,
  })
  const options = rows.map((i) => ({ value: i.id, label: i.username }))
  if (missing && selected) options.unshift({ value: selected.id, label: selected.username })

  return (
    <Select
      size={size} label={label} error={error} placeholder={placeholder}
      searchable value={value}
      data={options}
      onChange={(id) => onChange(id, options.find((o) => o.value === id)?.label ?? '')}
      searchValue={search} onSearchChange={setSearch}
      /* The server already filtered; filtering again would hide matches it
         deliberately returned. */
      filter={({ options }) => options}
      nothingFoundMessage={debounced ? 'Nobody matches' : 'Type to search'}
    />
  )
}
