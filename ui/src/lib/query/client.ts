import { QueryClient } from '@tanstack/react-query'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      /* The scope forest and axis registry change rarely and are read on
         nearly every screen. In production these are invalidated by the
         backend's catalog_version signal (see docs/architecture.md), so a long
         staleTime here is safe rather than a guess. */
      staleTime: 60_000,
      gcTime: 5 * 60_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})
