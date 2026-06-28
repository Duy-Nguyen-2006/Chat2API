/**
 * ChatGPT Web API Adapter
 * Converts OpenAI-format requests to ChatGPT backend-api protocol
 */

import axios, { type AxiosResponse } from 'axios'
import type { Account, Provider } from '../../store/types'
import type { ChatCompletionRequest, ChatMessage } from '../types'

const CHATGPT_BASE = 'https://chatgpt.com'

const BROWSER_HEADERS = {
  'Accept': '*/*',
  'Accept-Encoding': 'gzip, deflate, br',
  'Accept-Language': 'en-US,en;q=0.9',
  'Content-Type': 'application/json',
  'Origin': CHATGPT_BASE,
  'Referer': `${CHATGPT_BASE}/`,
  'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36',
}

function uuid(): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}

function mapModel(model: string): string {
  const lower = model.toLowerCase()
  if (lower.includes('o3-mini-high')) return 'o3-mini-high'
  if (lower.includes('o3-mini')) return 'o3-mini'
  if (lower.includes('o1-preview')) return 'o1-preview'
  if (lower.includes('o1-pro')) return 'o1-pro'
  if (lower.includes('o1-mini')) return 'o1-mini'
  if (lower.includes('o1')) return 'o1'
  if (lower.includes('gpt-4o-mini')) return 'gpt-4o-mini'
  if (lower.includes('gpt-4o')) return 'gpt-4o'
  if (lower.includes('gpt-4')) return 'gpt-4'
  if (lower.includes('gpt-3.5') || lower.includes('3.5')) return 'text-davinci-002-render-sha'
  if (lower.includes('auto')) return 'auto'
  return 'gpt-4o'
}

function extractTextContent(content: ChatMessage['content']): string {
  if (!content) return ''
  if (typeof content === 'string') return content
  return content
    .filter((part) => part.type === 'text' && part.text)
    .map((part) => part.text || '')
    .join('\n')
}

function toChatGptMessages(messages: ChatMessage[]) {
  return messages.map((msg) => ({
    id: uuid(),
    author: { role: msg.role === 'tool' ? 'tool' : msg.role },
    content: {
      content_type: 'text',
      parts: [extractTextContent(msg.content)],
    },
    metadata: {},
  }))
}

export class ChatGPTAdapter {
  private provider: Provider
  private account: Account
  private accessToken: string
  private accountId?: string

  constructor(provider: Provider, account: Account) {
    this.provider = provider
    this.account = account
    this.accessToken =
      account.credentials.accessToken ||
      account.credentials.token ||
      account.credentials.apiKey ||
      ''
    this.accountId = account.credentials.accountId
  }

  static isChatGPTProvider(provider: Provider): boolean {
    return provider.id === 'chatgpt' ||
      provider.apiEndpoint.includes('chatgpt.com') ||
      provider.apiEndpoint.includes('chat.openai.com')
  }

  private buildHeaders(extra?: Record<string, string>): Record<string, string> {
    const headers: Record<string, string> = {
      ...BROWSER_HEADERS,
      Authorization: `Bearer ${this.accessToken}`,
      ...this.provider.headers,
      ...extra,
    }
    if (this.accountId) {
      headers['ChatGPT-Account-ID'] = this.accountId
    }
    return headers
  }

  async getChatRequirements(): Promise<string | null> {
    try {
      const response = await axios.post(
        `${CHATGPT_BASE}/backend-api/sentinel/chat-requirements`,
        { p: '' },
        {
          headers: this.buildHeaders(),
          timeout: 15000,
          validateStatus: () => true,
        }
      )
      if (response.status === 200) {
        return response.data?.token || null
      }
      return null
    } catch {
      return null
    }
  }

  buildConversationBody(request: ChatCompletionRequest, actualModel: string) {
    const reqModel = mapModel(actualModel)
    const chatMessages = toChatGptMessages(request.messages)

    return {
      action: 'next',
      messages: chatMessages,
      model: reqModel,
      parent_message_id: uuid(),
      history_and_training_disabled: true,
      conversation_mode: { kind: 'primary_assistant' },
      force_paragen: false,
      force_rate_limit: false,
      force_use_sse: true,
      timezone_offset_min: -480,
      timezone: 'America/Los_Angeles',
      websocket_request_id: uuid(),
    }
  }

  async chatCompletion(
    request: ChatCompletionRequest,
    actualModel: string
  ): Promise<AxiosResponse> {
    const chatToken = await this.getChatRequirements()
    const headers = this.buildHeaders({
      Accept: 'text/event-stream',
      ...(chatToken ? { 'openai-sentinel-chat-requirements-token': chatToken } : {}),
    })

    const body = this.buildConversationBody(request, actualModel)

    return axios.post(`${CHATGPT_BASE}/backend-api/conversation`, body, {
      headers,
      responseType: request.stream ? 'stream' : 'stream',
      timeout: 120000,
      validateStatus: () => true,
    })
  }
}

export const chatgptAdapter = ChatGPTAdapter
export default ChatGPTAdapter