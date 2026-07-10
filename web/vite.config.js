import { defineConfig } from 'vite'
import tailwindcss from '@tailwindcss/vite'

// Backend-integration config: Go (templ) serves the HTML, Vite owns the assets.
// https://vite.dev/guide/backend-integration
export default defineConfig({
  plugins: [tailwindcss()],
  build: {
    manifest: true, // emit .vite/manifest.json so Go can map entries -> hashed files
    outDir: '../public/build', // Gin serves this directory at /build
    emptyOutDir: true,
    rollupOptions: {
      input: 'src/app.js', // single entry; it imports app.css
    },
  },
  server: {
    // Allow the Go app (:8080) to load ES modules from the dev server (:5173)
    // during development. Dev-only; the production build is fully self-hosted.
    cors: true,
  },
})
