import { defineConfig } from 'vite'
import solid from 'vite-plugin-solid'
import fs from 'node:fs'
import path from 'node:path'

export default defineConfig({
  plugins: [solid()],
  server: {
    port: 5173,
    https: (() => {
      // Prefer repo certs if present
      const repoCrt = path.resolve(process.cwd(), '../certs/dev.crt')
      const repoKey = path.resolve(process.cwd(), '../certs/dev.key')
      if (fs.existsSync(repoCrt) && fs.existsSync(repoKey)) {
        return {
          cert: fs.readFileSync(repoCrt),
          key: fs.readFileSync(repoKey)
        } as any
      }
      return true // Vite will create a self-signed cert
    })()
  },
  build: {
    target: 'esnext'
  }
})
