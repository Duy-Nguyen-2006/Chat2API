/**
 * Credential Storage Module - Entry File
 * Export all storage related types and APIs
 */

// Type definitions
export * from './types'

// Core storage
export { storeManager } from './store'

// Account management API
export { AccountManager } from './accounts'

// Provider management API
export { ProviderManager } from './providers'

// Config management API
export { ConfigManager } from './config'

// Credential validation
export {
  validateCredentials,
  validateCredentialsBatch,
  validateOpenAIKey,
  validateClaudeKey,
  validateChatGPTCookie,
} from './validator'

// Convenience function to initialize storage
import { storeManager as _storeManager } from './store'

export async function initializeStore(): Promise<void> {
  await _storeManager.initialize()
}
