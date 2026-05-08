import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../internal/webui/static/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://localhost:48879',
        changeOrigin: true,
      },
      '/login': 'http://localhost:48879',
      '/logout': 'http://localhost:48879',
    },
  },
})
