/**
 * Proxy Service Module - Request Forwarder (ChatGPT only)
 */

import axios, { type AxiosRequestConfig, type AxiosResponse, AxiosError } from 'axios'
import type { Account, Provider } from '../store/types'
import { type ForwardResult, type ChatCompletionRequest, type ProxyContext } from './types'
import { proxyStatusManager } from './status'
import { storeManager } from '../store/store'
import { ChatGPTAdapter } from './adapters/chatgpt'
import { ChatGPTStreamHandler } from './adapters/chatgpt-stream'

export class RequestForwarder {
  private axiosInstance = axios.create({
    timeout: 120000,
    maxBodyLength: Infinity,
    maxContentLength: Infinity,
  })

  async forwardChatCompletion(
    request: ChatCompletionRequest,
    account: Account,
    provider: Provider,
    actualModel: string,
    _context: ProxyContext
  ): Promise<ForwardResult> {
    const startTime = Date.now()
    const config = storeManager.getConfig()
    const maxRetries = config.retryCount
    let lastError: string | undefined

    for (let attempt = 0; attempt <= maxRetries; attempt++) {
      if (attempt > 0) {
        await this.delay(2000)
      }

      try {
        const result = await this.forwardChatGPT(request, account, provider, actualModel, startTime)
        if (result.success) return result
        lastError = result.error
        if (result.status && result.status < 500 && result.status !== 429) break
      } catch (error) {
        lastError = error instanceof Error ? error.message : 'Unknown error'
      }
    }

    return {
      success: false,
      error: lastError || 'Request failed after retries',
      latency: Date.now() - startTime,
    }
  }

  private async forwardChatGPT(
    request: ChatCompletionRequest,
    account: Account,
    provider: Provider,
    actualModel: string,
    startTime: number
  ): Promise<ForwardResult> {
    try {
      const adapter = new ChatGPTAdapter(provider, account)
      const response = await adapter.chatCompletion(request, actualModel)
      const latency = Date.now() - startTime

      if (response.status >= 400) {
        return {
          success: false,
          status: response.status,
          error: this.extractErrorMessage(response),
          latency,
        }
      }

      const handler = new ChatGPTStreamHandler(actualModel)

      if (request.stream) {
        const transformedStream = handler.handleStream(response.data)
        return {
          success: true,
          status: response.status,
          headers: this.extractHeaders(response.headers),
          stream: transformedStream,
          skipTransform: true,
          latency,
        }
      }

      const body = await handler.handleNonStream(response.data)
      return {
        success: true,
        status: response.status,
        headers: this.extractHeaders(response.headers),
        body,
        latency,
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
        latency: Date.now() - startTime,
      }
    }
  }

  private extractHeaders(headers: Record<string, unknown>): Record<string, string> {
    const result: Record<string, string> = {}
    for (const [key, value] of Object.entries(headers)) {
      if (typeof value === 'string') {
        result[key] = value
      } else if (Array.isArray(value)) {
        result[key] = value.join(', ')
      }
    }
    return result
  }

  private extractErrorMessage(response: AxiosResponse): string {
    if (response.data) {
      if (typeof response.data === 'string') return response.data
      if (response.data.error?.message) return response.data.error.message
      if (response.data.message) return response.data.message
      if (response.data.detail) {
        return typeof response.data.detail === 'string'
          ? response.data.detail
          : JSON.stringify(response.data.detail)
      }
      try {
        return JSON.stringify(response.data)
      } catch {
        return 'Unknown error'
      }
    }
    return `HTTP ${response.status}`
  }

  private delay(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms))
  }

  async forwardToUrl(
    url: string,
    method: string,
    headers: Record<string, string>,
    body: unknown,
    isStream: boolean = false
  ): Promise<ForwardResult> {
    const startTime = Date.now()

    try {
      const config: AxiosRequestConfig = {
        method,
        url,
        headers,
        data: body,
        timeout: proxyStatusManager.getConfig().timeout,
        responseType: isStream ? 'stream' : 'json',
        validateStatus: () => true,
      }

      const response: AxiosResponse = await this.axiosInstance.request(config)
      const latency = Date.now() - startTime

      if (response.status >= 400) {
        return {
          success: false,
          status: response.status,
          error: this.extractErrorMessage(response),
          latency,
        }
      }

      if (isStream) {
        return {
          success: true,
          status: response.status,
          headers: this.extractHeaders(response.headers),
          stream: response.data,
          latency,
        }
      }

      return {
        success: true,
        status: response.status,
        headers: this.extractHeaders(response.headers),
        body: response.data,
        latency,
      }
    } catch (error) {
      return {
        success: false,
        error: error instanceof Error ? error.message : 'Unknown error',
        latency: Date.now() - startTime,
      }
    }
  }
}

export const requestForwarder = new RequestForwarder()
export default requestForwarder