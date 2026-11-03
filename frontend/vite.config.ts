import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// macOS 12 ships WebKit/Safari 15: keep the output compatible with it.
export default defineConfig({
  plugins: [svelte()],
  base: './',
  build: {
    target: ['es2020', 'safari15', 'chrome100'],
    outDir: 'dist',
    emptyOutDir: false, // keep dist/.gitkeep (needed by go:embed before the first build)
    cssTarget: ['safari15', 'chrome100'],
  },
  server: { port: 9245, strictPort: true },
})
