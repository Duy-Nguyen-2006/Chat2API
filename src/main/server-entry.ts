/**
 * Chat2API Headless Server Entry
 */

import { storeManager } from './store/store'
import { proxyServer } from './proxy/server'

async function main(): Promise<void> {
  process.on('uncaughtException', (error) => {
    console.error('[Server] Uncaught exception:', error)
  })

  process.on('unhandledRejection', (reason) => {
    console.error('[Server] Unhandled rejection:', reason)
  })

  await storeManager.initialize()
  await seedChatGPTFromEnv()

  const config = storeManager.getConfig()
  const port = Number(process.env.PORT || config.proxyPort || 8080)
  let host = process.env.HOST || config.proxyHost || 'localhost'

  // Migrate legacy bind address
  if (host === '127.0.0.1') {
    host = 'localhost'
    storeManager.updateConfig({ proxyHost: host, proxyPort: port })
  }

  const started = await proxyServer.start(port, host)
  if (!started) {
    console.error(`[Server] Failed to start on ${host}:${port}`)
    process.exit(1)
  }

  console.log(`[Server] Chat2API (ChatGPT-only) listening on http://${host}:${port}`)
  console.log('[Server] Endpoints: POST /v1/chat/completions, GET /v1/models')

  const shutdown = async (signal: string) => {
    console.log(`[Server] Received ${signal}, shutting down...`)
    storeManager.flushPendingWrites()
    await proxyServer.stop()
    process.exit(0)
  }

  process.on('SIGINT', () => void shutdown('SIGINT'))
  process.on('SIGTERM', () => void shutdown('SIGTERM'))
}

async function seedChatGPTFromEnv(): Promise<void> {
  const accessToken = process.env.CHATGPT_ACCESS_TOKEN
  if (!accessToken) return

  const accountId = process.env.CHATGPT_ACCOUNT_ID
  let provider = storeManager.getProviderById('chatgpt')

  if (!provider) {
    const { chatgptConfig } = await import('./providers/builtin/chatgpt')
    const now = Date.now()
    provider = {
      ...chatgptConfig,
      createdAt: now,
      updatedAt: now,
    }
    storeManager.addProvider(provider)
    console.log('[Server] Created ChatGPT provider from environment')
  }

  const existingAccounts = storeManager.getAccountsByProviderId('chatgpt', true)
  if (existingAccounts.length === 0) {
    const now = Date.now()
    storeManager.addAccount({
      id: storeManager.generateId(),
      providerId: 'chatgpt',
      name: 'Default',
      credentials: {
        accessToken,
        ...(accountId ? { accountId } : {}),
      },
      status: 'active',
      createdAt: now,
      updatedAt: now,
    })
    console.log('[Server] Added ChatGPT account from CHATGPT_ACCESS_TOKEN')
  }
}

main().catch((error) => {
  console.error('[Server] Startup failed:', error)
  process.exit(1)
})