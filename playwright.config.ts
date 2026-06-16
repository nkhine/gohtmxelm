import { defineConfig } from '@playwright/test'

const baseURL = process.env.PLAYWRIGHT_BASE_URL || 'https://localhost:8091'
const url = new URL(baseURL)
// Derive the server settings from the base URL so the dev server is started on
// the same host/port the tests hit. HTTPS base URLs need GOHTMXELM_TLS=1 so the
// server negotiates HTTP/2 over TLS (see Makefile).
const port = url.port || (url.protocol === 'https:' ? '443' : '80')
const tls = url.protocol === 'https:' ? '1' : '0'

export default defineConfig({
  testDir: './tests/browser',
  timeout: 30_000,
  use: {
    baseURL,
    ignoreHTTPSErrors: true,
  },
  webServer: {
    command: `make build && GOHTMXELM_TLS=${tls} PORT=${port} ./gohtmxelm-demo`,
    url: baseURL,
    ignoreHTTPSErrors: true,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
})
