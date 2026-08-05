/**
 * MCP TypeScript Client — 连接 AgentPrimordia Go MCP Server。
 *
 * 支持两种传输：
 *   - HTTP/SSE：连接 Go MCP Server 的 ServeSSE 端点（推荐，浏览器/Node 通用）
 *   - stdio：通过 child_process 启动 Go MCP Server 并通信（Node.js 专用）
 *
 * 协议：MCP 2024-11-05 + JSON-RPC 2.0
 *
 * 使用方式（HTTP/SSE）：
 *   const client = new MCPClient({ transport: 'sse', url: 'http://localhost:3000/mcp' })
 *   await client.initialize()
 *   const tools = await client.listTools()
 *   const result = await client.callTool('get_weather', { city: 'Beijing' })
 *
 * 使用方式（stdio）：
 *   const client = new MCPClient({ transport: 'stdio', command: './mcp-server' })
 *   await client.initialize()
 *   const tools = await client.listTools()
 *   client.close()
 */

import { spawn } from 'node:child_process';

// ===== 类型定义 =====

export interface MCPTool {
  name: string
  description?: string
  inputSchema?: Record<string, unknown>
}

export interface MCPToolResult {
  content: Array<{ type: string; text: string }>
  isError?: boolean
}

export interface MCPServerInfo {
  name: string
  version: string
}

export interface MCPCapabilities {
  tools?: { listChanged?: boolean }
}

export interface JSONRPCRequest {
  jsonrpc: '2.0'
  id?: string | number
  method: string
  params?: Record<string, unknown>
}

export interface JSONRPCResponse {
  jsonrpc: '2.0'
  id?: string | number
  result?: unknown
  error?: { code: number; message: string }
}

export type MCPTransport = 'sse' | 'stdio'

export interface MCPClientOptions {
  transport: MCPTransport
  /** SSE 传输时的 URL */
  url?: string
  /** stdio 传输时的命令路径 */
  command?: string
  /** stdio 传输时的命令行参数 */
  args?: string[]
  /** 初始化超时（毫秒） */
  timeout?: number
  /** 日志函数 */
  logger?: (...args: unknown[]) => void
  /** v3.9-4：工具名命名空间前缀，隔离多 MCP server 同名工具 */
  toolPrefix?: string
}

// ===== HTTP/SSE 传输层 =====

class SSETransport {
  private url: string
  private fetch: typeof globalThis.fetch

  constructor(url: string) {
    this.url = url
    this.fetch = globalThis.fetch
  }

  async request(body: JSONRPCRequest): Promise<JSONRPCResponse> {
    const res = await this.fetch(this.url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })

    if (!res.ok) {
      throw new MCPError(`HTTP ${res.status}: ${res.statusText}`, -32000)
    }

    return res.json() as Promise<JSONRPCResponse>
  }
}

// ===== stdio 传输层 =====

class StdioTransport {
  private child: ReturnType<typeof spawn> | null = null
  private url?: string

  constructor(command: string, args: string[] = []) {
    this.child = spawn(command, args, { stdio: ['pipe', 'pipe', 'inherit'] })
  }

  async request(body: JSONRPCRequest): Promise<JSONRPCResponse> {
    if (!this.child) {
      throw new MCPError('stdio transport not initialized', -32603)
    }
    return new Promise((resolve, reject) => {
      const line = JSON.stringify(body) + '\n'
      const stdout = this.child!.stdout
      if (!stdout) {
        reject(new MCPError('stdout not available', -32603))
        return
      }

      let buf = ''
      const onData = (chunk: Buffer) => {
        buf += chunk.toString()
        const idx = buf.indexOf('\n')
        if (idx !== -1) {
          const responseLine = buf.slice(0, idx)
          stdout.off('data', onData)
          try {
            resolve(JSON.parse(responseLine) as JSONRPCResponse)
          } catch (e) {
            reject(new MCPError(`parse error: ${(e as Error).message}`, -32700))
          }
        }
      }
      stdout.on('data', onData)
      this.child!.stdin!.write(line, (err) => {
        if (err) reject(new MCPError(`stdin write error: ${err.message}`, -32603))
      })
    })
  }

  close() {
    this.child?.kill()
    this.child = null
  }
}

// ===== MCP 错误 =====

export class MCPError extends Error {
  constructor(message: string, public code: number) {
    super(message)
    this.name = 'MCPError'
  }
}

// ===== MCP 客户端 =====

export class MCPClient {
  private transport: SSETransport | StdioTransport
  private options: MCPClientOptions
  private initialized = false
  private serverInfo?: MCPServerInfo
  private capabilities?: MCPCapabilities
  private requestID = 0
  private toolPrefix = ''

  constructor(options: MCPClientOptions) {
    this.options = { timeout: 30000, ...options }
    if (options.toolPrefix) {
      this.toolPrefix = options.toolPrefix.replace(/^_+|_+$/g, '')
    }
    if (options.transport === 'sse') {
      if (!options.url) throw new MCPError('url required for SSE transport', -32602)
      this.transport = new SSETransport(options.url)
    } else {
      if (!options.command) throw new MCPError('command required for stdio transport', -32602)
      this.transport = new StdioTransport(options.command, options.args)
    }
  }

  /** v3.9-4：设置工具名命名空间前缀（多 server 同名工具隔离） */
  setToolPrefix(prefix: string): void {
    this.toolPrefix = prefix.replace(/^_+|_+$/g, '')
  }

  /** v3.9-4：工具对外名（带前缀）→ MCP 原始工具名 */
  private toRawName(prefixedName: string): string {
    if (!this.toolPrefix) return prefixedName
    const raw = prefixedName.startsWith(this.toolPrefix + '_')
      ? prefixedName.slice(this.toolPrefix.length + 1)
      : prefixedName
    return raw
  }

  /** v3.9-4：MCP 原始工具名 → 工具对外名（带前缀） */
  private toPublicName(rawName: string): string {
    if (!this.toolPrefix) return rawName
    return `${this.toolPrefix}_${rawName}`
  }

  get isInitialized(): boolean {
    return this.initialized
  }

  get info(): MCPServerInfo | undefined {
    return this.serverInfo
  }

  get caps(): MCPCapabilities | undefined {
    return this.capabilities
  }

  /** 初始化 MCP 连接 */
  async initialize(): Promise<MCPServerInfo> {
    const resp = await this.sendRequest('initialize', {
      protocolVersion: '2024-11-05',
      capabilities: {},
      clientInfo: { name: '@agentprimordia/sdk', version: '1.0.0' },
    })
    if (resp.error) {
      throw new MCPError(resp.error.message, resp.error.code)
    }
    const result = resp.result as { protocolVersion: string; serverInfo: MCPServerInfo; capabilities: MCPCapabilities }
    this.initialized = true
    this.serverInfo = result.serverInfo
    this.capabilities = result.capabilities
    this.options.logger?.('[MCP] connected to', result.serverInfo.name, result.serverInfo.version)
    return this.serverInfo
  }

  /** 列出所有可用工具（v3.9-4：返回带命名空间前缀的工具名） */
  async listTools(): Promise<MCPTool[]> {
    const resp = await this.sendRequest('tools/list', {})
    if (resp.error) {
      throw new MCPError(resp.error.message, resp.error.code)
    }
    const tools = (resp.result as { tools: MCPTool[] }).tools ?? []
    return tools.map((t) => ({ ...t, name: this.toPublicName(t.name) }))
  }

  /** 调用指定工具（v3.9-4：自动剥离前缀，以原始名调用 MCP server） */
  async callTool(name: string, arguments_?: Record<string, unknown>): Promise<MCPToolResult> {
    const rawName = this.toRawName(name)
    const resp = await this.sendRequest('tools/call', { name: rawName, arguments: arguments_ })
    if (resp.error) {
      throw new MCPError(resp.error.message, resp.error.code)
    }
    return resp.result as MCPToolResult
  }

  /** 发送 JSON-RPC 请求 */
  private async sendRequest(
    method: string,
    params: Record<string, unknown>,
  ): Promise<JSONRPCResponse> {
    const id = ++this.requestID
    const req: JSONRPCRequest = { jsonrpc: '2.0', id, method, params }
    this.options.logger?.('[MCP] →', method, params)
    return this.transport.request(req)
  }

  /** 关闭连接 */
  close(): void {
    if (this.transport instanceof StdioTransport) {
      this.transport.close()
    }
    this.initialized = false
  }
}

// ===== 工具调用助手 =====

/**
 * 创建类型安全的工具调用函数。
 *
 * 使用方式：
 *   const call = createToolCaller(client)
 *   const weather = await call<{ city: string }>({ name: 'get_weather', arguments: { city: 'Beijing' } })
 */
export function createToolCaller(client: MCPClient) {
  return async function callTool<TArgs extends Record<string, unknown> = Record<string, unknown>>(
    tool: { name: string; arguments?: TArgs },
  ): Promise<MCPToolResult> {
    return client.callTool(tool.name, tool.arguments)
  }
}

export default MCPClient
