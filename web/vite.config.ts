import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { VitePWA } from 'vite-plugin-pwa'

export default defineConfig({
  plugins: [react(), VitePWA({
    registerType: 'prompt',
    injectRegister: false,
    strategies: 'injectManifest', srcDir: 'src', filename: 'sw.ts',
    manifest: {
      id: '/', name: 'Pinelink', short_name: 'Pinelink', start_url: '/', scope: '/',
      display: 'standalone', background_color: '#ffffff', theme_color: '#0866ff',
      prefer_related_applications: false,
      icons: [
        { src: '/pwa-192.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
        { src: '/pwa-512.png', sizes: '512x512', type: 'image/png', purpose: 'any' },
        { src: '/pwa-512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
      ],
    },
    injectManifest: {
      rollupFormat: 'iife',
      globPatterns: ['**/*.{js,css,html,png,ico,svg,woff2,wasm,webmanifest}'],
      maximumFileSizeToCacheInBytes: 4 * 1024 * 1024,
    },
  })],
  server: { host: '127.0.0.1', port: 5173, fs: { allow: ['..'] } },
  test: { environment: 'jsdom', setupFiles: ['./src/test/setup.ts'] },
})
