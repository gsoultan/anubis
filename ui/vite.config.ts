import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import { fileURLToPath, URL } from 'node:url'

/* 7447 is deliberate. Avoided: 3000/3001 (Node, CRA), 4200 (Angular),
   5000 (also macOS AirPlay Receiver), 5173 (Vite default), 8000/8080/8888.
   Also avoided everything already listening on this machine -- 5432 Postgres,
   5672/15672 RabbitMQ, 6379 Redis, 7000, 9000/9001 MinIO.
   7448 is reserved for the Go API so the pair stays memorable. */
const PORT = Number(process.env['ANUBIS_UI_PORT'] ?? 7447)

export default defineConfig({
  plugins: [
    // Router plugin must precede the React plugin so generated route types
    // exist before Fast Refresh wraps the modules.
    tanstackRouter({ target: 'react', autoCodeSplitting: true }),
    react(),
  ],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  server: {
    port: PORT,
    // Fail loudly rather than silently hopping to 7448 (reserved for the API).
    strictPort: true,
    proxy: {
      '/v1': { target: `http://localhost:${process.env['ANUBIS_API_PORT'] ?? 7448}`, changeOrigin: true },
      // Connect RPC surface (same-origin in dev beats CORS entirely).
      '/anubis.v1.': { target: `http://localhost:${process.env['ANUBIS_API_PORT'] ?? 7448}`, changeOrigin: true },
      '/.well-known': { target: `http://localhost:${process.env['ANUBIS_API_PORT'] ?? 7448}`, changeOrigin: true },
    },
  },
  preview: { port: PORT + 1, strictPort: true },
})
