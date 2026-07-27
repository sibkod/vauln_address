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
    proxy: {
      '/api': {
        target: 'http://api.vuln.test',
        changeOrigin: true
      }
    }
  },
  define: {
    'process.env': {},
    global: 'globalThis'
  }
})
