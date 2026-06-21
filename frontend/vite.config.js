import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// Go バックエンド（既定 :8080）。VITE_API_TARGET で上書き可。
const apiTarget = process.env.VITE_API_TARGET || 'http://localhost:8080'

// https://vite.dev/config/
export default defineConfig({
  plugins: [svelte()],
  server: {
    proxy: {
      // REST と WebSocket（/api/panes/{id}/io）の両方を同一オリジンで中継し、
      // CORS を回避する。ws:true で WebSocket アップグレードも転送する。
      '/api': {
        target: apiTarget,
        changeOrigin: true,
        ws: true,
      },
    },
  },
})
