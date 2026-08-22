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
  define: { 'process.env.NODE_ENV': '"production"' },
})

if (!result.success) {
  for (const log of result.logs) console.error(log)
  process.exit(1)
}

const ms = Math.round(performance.now() - t0)
let total = 0
for (const o of result.outputs) total += o.size
console.log(`\n${result.outputs.length} artefacts, ${(total / 1024).toFixed(1)} kB in ${ms} ms`)
for (const o of result.outputs.sort((a, b) => b.size - a.size).slice(0, 6)) {
  console.log(`  ${(o.size / 1024).toFixed(1).padStart(8)} kB  ${o.path.replace(outdir, '')}`)
}
