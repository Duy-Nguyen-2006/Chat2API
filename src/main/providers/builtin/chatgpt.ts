import type { BuiltinProviderConfig } from '../../store/types'

export const chatgptConfig: BuiltinProviderConfig = {
  id: 'chatgpt',
  name: 'ChatGPT',
  type: 'builtin',
  authType: 'token',
  apiEndpoint: 'https://chatgpt.com',
  chatPath: '/backend-api/conversation',
  headers: {
    'Content-Type': 'application/json',
    'Accept': 'text/event-stream',
    'Origin': 'https://chatgpt.com',
    'Referer': 'https://chatgpt.com/',
    'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36',
  },
  enabled: true,
  description: 'ChatGPT Web API — OpenAI-compatible proxy',
  supportedModels: [
    'gpt-4o',
    'gpt-4o-mini',
    'gpt-4',
    'gpt-3.5-turbo',
    'o1',
    'o1-mini',
    'o1-preview',
    'o3-mini',
    'auto',
  ],
  modelMappings: {
    'gpt-4o': 'gpt-4o',
    'gpt-4o-mini': 'gpt-4o-mini',
    'gpt-4': 'gpt-4',
    'gpt-3.5-turbo': 'text-davinci-002-render-sha',
    'o1': 'o1',
    'o1-mini': 'o1-mini',
    'o1-preview': 'o1-preview',
    'o3-mini': 'o3-mini',
    'auto': 'auto',
  },
  credentialFields: [
    {
      name: 'accessToken',
      label: 'Access Token',
      type: 'password',
      required: true,
      placeholder: 'Enter ChatGPT access token',
      helpText: 'Get from https://chatgpt.com/api/auth/session after logging in',
    },
    {
      name: 'accountId',
      label: 'Account ID (optional)',
      type: 'text',
      required: false,
      placeholder: 'Team/Plus account ID',
      helpText: 'Required for Team workspace accounts',
    },
  ],
  tokenCheckEndpoint: '/api/auth/session',
  tokenCheckMethod: 'GET',
}

export default chatgptConfig