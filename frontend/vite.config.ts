import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { NodeGlobalsPolyfillPlugin } from '@esbuild-plugins/node-globals-polyfill'

export default defineConfig({
  plugins: [
    vue(),
    NodeGlobalsPolyfillPlugin({
      buffer: true,
      process: true
    })
  ],
  build: {
    outDir: 'dist'
  },
  server: {
    allowedHosts: true,
    proxy: {
      '/api': {
        target: process.env.VITE_PROXY_TARGET || 'http://api.vuln.test',
        changeOrigin: true
      }
    }
  },
  preview: {
    allowedHosts: true
  },
  define: {
    'process.env': {},
    global: 'globalThis'
  }
})
