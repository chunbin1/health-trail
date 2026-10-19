import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// In development the UI runs on Vite (5173) and the API on `health-vault
// serve` (8721). The proxy drops the browser's Origin header: the server
// refuses requests from any origin but its own, and to it the dev page's
// origin is a foreign one.
const backend = {
  target: 'http://127.0.0.1:8721',
  configure: (proxy: any) => proxy.on('proxyReq', (req: any) => req.removeHeader('origin')),
}

export default defineConfig({
  plugins: [react()],
  server: { proxy: { '/api': backend, '/mcp': backend } },
  build: { outDir: 'dist', emptyOutDir: true },
})
