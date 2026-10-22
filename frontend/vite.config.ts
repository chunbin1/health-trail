import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

// In development (`make dev`) the UI runs on Vite (5173) and the API on
// `health-trail serve -port 8722` (HEALTH_TRAIL_DEV_PORT), away from an
// installed app on 8721. The proxy drops the browser's Origin header: the
// server refuses requests from any origin but its own, and to it the dev
// page's origin is a foreign one.
export default defineConfig(({ mode }) => {
  const port = loadEnv(mode, '.', 'HEALTH_TRAIL_').HEALTH_TRAIL_DEV_PORT || '8722'
  const backend = {
    target: `http://127.0.0.1:${port}`,
    configure: (proxy: any) => proxy.on('proxyReq', (req: any) => req.removeHeader('origin')),
  }
  return {
    plugins: [react()],
    server: { proxy: { '/api': backend, '/mcp': backend } },
    build: { outDir: 'dist', emptyOutDir: true },
  }
})
