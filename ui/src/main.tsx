import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { MantineProvider } from '@mantine/core'
import { Notifications } from '@mantine/notifications'
import { QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider, createRouter } from '@tanstack/react-router'
import { Button, Stack, Text, Title } from '@mantine/core'
import { routeTree } from './routeTree.gen'
import { queryClient } from './lib/query/client'
import { theme } from './styles/theme'
import './styles/index.css'

const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
  scrollRestoration: true,
  context: { queryClient },
  // Without this, TanStack Router falls back to a bare "<p>Not Found</p>" and
  // logs a warning on every unmatched request, including asset probes.
  defaultNotFoundComponent: () => (
    <Stack align="center" justify="center" h="100vh" gap="xs">
      <Title order={2}>404</Title>
      <Text size="sm" c="dimmed">No such page in the console.</Text>
      <Button size="xs" variant="light" onClick={() => { window.location.href = '/' }}>
        Back to overview
      </Button>
    </Stack>
  ),
})

declare module '@tanstack/react-router' {
  interface Register { router: typeof router }
}

/* ?scheme=dark|light overrides the stored preference for this load — used by
   docs, demos and the screenshot harness. */
const forced = new URLSearchParams(window.location.search).get('scheme')
if (forced === 'dark' || forced === 'light') {
  window.localStorage.setItem('mantine-color-scheme-value', forced)
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    {/* Light is the default: it is the more legible scheme for daily
        administration. Dark stays one click away for the on-call at 3am — the
        verdict palette is tuned for both. */}
    <MantineProvider theme={theme} defaultColorScheme="light">
      <QueryClientProvider client={queryClient}>
        <Notifications position="top-right" />
        <RouterProvider router={router} />
      </QueryClientProvider>
    </MantineProvider>
  </StrictMode>,
)
