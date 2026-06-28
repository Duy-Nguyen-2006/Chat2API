import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'fs'
import { join } from 'path'

export class JsonFileStore<T extends Record<string, unknown>> {
  private data: T
  private readonly filePath: string

  constructor(options: { name: string; cwd: string; defaults: T }) {
    if (!existsSync(options.cwd)) {
      mkdirSync(options.cwd, { recursive: true })
    }
    this.filePath = join(options.cwd, `${options.name}.json`)
    this.data = { ...options.defaults }
    this.load()
  }

  private load(): void {
    if (!existsSync(this.filePath)) return
    try {
      const raw = readFileSync(this.filePath, 'utf-8')
      this.data = { ...this.data, ...JSON.parse(raw) }
    } catch (error) {
      console.error('[JsonStore] Failed to load, using defaults:', error)
    }
  }

  private save(): void {
    writeFileSync(this.filePath, JSON.stringify(this.data, null, 2), 'utf-8')
  }

  get<K extends keyof T>(key: K): T[K] {
    return this.data[key]
  }

  set<K extends keyof T>(key: K, value: T[K]): void {
    this.data[key] = value
    this.save()
  }

  clear(): void {
    const defaults = { ...this.data }
    for (const key of Object.keys(defaults)) {
      delete this.data[key as keyof T]
    }
    this.save()
  }

  get store(): T {
    return this.data
  }
}