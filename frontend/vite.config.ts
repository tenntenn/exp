import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/parser.v1.ParserService': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
