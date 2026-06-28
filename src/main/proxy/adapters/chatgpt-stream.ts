/**
 * ChatGPT SSE Stream Handler
 * Converts ChatGPT backend-api SSE to OpenAI-compatible format
 */

import { PassThrough } from 'stream'

function generateChatId(): string {
  const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789'
  let id = 'chatcmpl-'
  for (let i = 0; i < 29; i++) {
    id += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  return id
}

interface ChatGPTStreamEvent {
  message?: {
    author?: { role?: string }
    status?: string
    content?: { parts?: string[] }
  }
  error?: string
}

export class ChatGPTStreamHandler {
  private model: string
  private chatId: string
  private created: number
  private sentRole: boolean = false
  private accumulated: string = ''

  constructor(model: string) {
    this.model = model
    this.chatId = generateChatId()
    this.created = Math.floor(Date.now() / 1000)
  }

  private createChunk(content: string, finishReason: string | null = null): string {
    return `data: ${JSON.stringify({
      id: this.chatId,
      object: 'chat.completion.chunk',
      created: this.created,
      model: this.model,
      choices: [{
        index: 0,
        delta: content ? { content } : {},
        finish_reason: finishReason,
      }],
    })}\n\n`
  }

  private createRoleChunk(): string {
    return `data: ${JSON.stringify({
      id: this.chatId,
      object: 'chat.completion.chunk',
      created: this.created,
      model: this.model,
      choices: [{
        index: 0,
        delta: { role: 'assistant', content: '' },
        finish_reason: null,
      }],
    })}\n\n`
  }

  extractDelta(line: string): string | null {
    if (!line.startsWith('data: ')) return null
    const payload = line.slice(6).trim()
    if (payload === '[DONE]') return null

    try {
      const event: ChatGPTStreamEvent = JSON.parse(payload)
      const message = event.message
      if (!message) return null

      const role = message.author?.role
      if (role === 'user' || role === 'system') return null

      const parts = message.content?.parts
      if (!parts || parts.length === 0) return null

      const text = parts.join('')
      if (!text) return null

      return text
    } catch {
      return null
    }
  }

  handleStream(sourceStream: NodeJS.ReadableStream): PassThrough {
    const output = new PassThrough()
    let buffer = ''

    if (!this.sentRole) {
      output.write(this.createRoleChunk())
      this.sentRole = true
    }

    sourceStream.on('data', (chunk: Buffer) => {
      buffer += chunk.toString()
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        const delta = this.extractDelta(line)
        if (delta) {
          const prevLen = this.accumulated.length
          if (delta.length > prevLen) {
            const newContent = delta.slice(prevLen)
            this.accumulated = delta
            output.write(this.createChunk(newContent))
          }
        }
      }
    })

    sourceStream.on('end', () => {
      output.write(this.createChunk('', 'stop'))
      output.write('data: [DONE]\n\n')
      output.end()
    })

    sourceStream.on('error', (err) => {
      output.destroy(err)
    })

    return output
  }

  async handleNonStream(sourceStream: NodeJS.ReadableStream): Promise<Record<string, unknown>> {
    return new Promise((resolve, reject) => {
      let buffer = ''

      sourceStream.on('data', (chunk: Buffer) => {
        buffer += chunk.toString()
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''

        for (const line of lines) {
          const delta = this.extractDelta(line)
          if (delta && delta.length > this.accumulated.length) {
            this.accumulated = delta
          }
        }
      })

      sourceStream.on('end', () => {
        resolve({
          id: this.chatId,
          object: 'chat.completion',
          created: this.created,
          model: this.model,
          choices: [{
            index: 0,
            message: {
              role: 'assistant',
              content: this.accumulated,
            },
            finish_reason: 'stop',
          }],
          usage: {
            prompt_tokens: 0,
            completion_tokens: 0,
            total_tokens: 0,
          },
        })
      })

      sourceStream.on('error', reject)
    })
  }
}

export default ChatGPTStreamHandler