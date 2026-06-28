import chatgptConfig from './chatgpt'
import type { BuiltinProviderConfig } from '../../store/types'

export const builtinProviders: BuiltinProviderConfig[] = [
  chatgptConfig,
]

export const builtinProviderMap: Record<string, BuiltinProviderConfig> = {
  chatgpt: chatgptConfig,
}

export function getBuiltinProvider(id: string): BuiltinProviderConfig | undefined {
  return builtinProviderMap[id]
}

export function getBuiltinProviders(): BuiltinProviderConfig[] {
  return builtinProviders
}

export { chatgptConfig }

export default builtinProviders