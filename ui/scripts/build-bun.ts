/* Native Bun bundler — no Vite, no Rollup, no esbuild.
   Routes are generated beforehand by `tsr generate` (the TanStack Router Vite
   plugin is Vite-only; the CLI does the same job standalone). */
import tailwind from 'bun-plugin-tailwind'
import { rm } from 'node:fs/promises'

const outdir = './dist'
await rm(outdir, { recursive: true, force: true })

const t0 = performance.now()
const result = await Bun.build({
  entrypoints: ['./index.html'],
  outdir,
  plugins: [tailwind],
  target: 'browser',
  minify: true,
  splitting: true,
  sourcemap: 'linked',
  // The console is always served from the origin root (consoleHandler is
  // mounted at "/"), so asset URLs can be root-absolute. A <base href> would
  // do the same job and is blocked: the shell's CSP says base-uri 'none'.
  publicPath: '/',
  define: { 'process.env.NODE_ENV': '"production"' },
})

if (!result.success) {
  for (const log of result.logs) console.error(log)
  process.exit(1)
}

/* The shell is served for every console route, and a relative asset URL
   resolves against the route rather than the root. From /identities/<id>,
   ./chunk.js is /identities/chunk.js — which the SPA fallback answers with
   index.html, the browser refuses as a script, and the page is blank. That
   shipped in v0.4.0: a person's page worked until somebody refreshed it. */
const shell = await Bun.file(`${outdir}/index.html`).text()
const relative = [...shell.matchAll(/\b(?:src|href)="([^"]*)"/g)]
  .map((m) => m[1])
  .filter((u) => !u.startsWith('/') && !/^[a-z][a-z0-9+.-]*:/i.test(u))
if (relative.length) {
  console.error(`index.html loads assets by relative URL, which breaks every nested route:\n  ${relative.join('\n  ')}`)
  process.exit(1)
}

const ms = Math.round(performance.now() - t0)
let total = 0
for (const o of result.outputs) total += o.size
console.log(`\n${result.outputs.length} artefacts, ${(total / 1024).toFixed(1)} kB in ${ms} ms`)
for (const o of result.outputs.sort((a, b) => b.size - a.size).slice(0, 6)) {
  console.log(`  ${(o.size / 1024).toFixed(1).padStart(8)} kB  ${o.path.replace(outdir, '')}`)
}
