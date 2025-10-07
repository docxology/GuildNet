import { defineConfig } from 'vite'
import solid from 'vite-plugin-solid'
import fs from 'node:fs'
import path from 'node:path'

export default defineConfig({
  plugins: [solid()],
  server: {
    port: 5173,
    https: (() => {
      const repoCrt = path.resolve(process.cwd(), '../certs/server.crt')
      const repoKey = path.resolve(process.cwd(), '../certs/server.key')
      if (fs.existsSync(repoCrt) && fs.existsSync(repoKey)) {
        return {
          cert: fs.readFileSync(repoCrt),
          key: fs.readFileSync(repoKey)
        } as any
      }
      return true // Last resort: Vite self-signed
    })()
  },
  build: {
    target: 'esnext'
  }
})
