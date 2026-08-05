import { describe, it, expect } from 'vitest'
import { MCPClient } from '../../src/mcp/client.js'

/** 启动一个最小 JSON-RPC MCP mock server */
function startMockMCPServer(handlers: Record<string, (params: any) => any>) {
  const { createServer } = require('node:http') as typeof import('node:http')
  const received: Array<{ method: string; params: any }> = []
  const server = createServer((req, res) => {
    let body = ''
    req.on('data', (c) => (body += c))
    req.on('end', () => {
      const msg = JSON.parse(body)
      received.push({ method: msg.method, params: msg.params })
      const handler = handlers[msg.method]
      res.setHeader('Content-Type', 'application/json')
      if (handler) {
        res.end(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: handler(msg.params) }))
      } else {
        res.end(JSON.stringify({ jsonrpc: '2.0', id: msg.id, error: { code: -32601, message: 'method not found' } }))
      }
    })
  })
  return new Promise<{ url: string; close: () => Promise<void>; received: typeof received }>((resolve) => {
    server.listen(0, () => {
      const addr = server.address() as { port: number }
      resolve({
        url: `http://127.0.0.1:${addr.port}`,
        close: () => new Promise((r) => server.close(() => r(undefined))),
        received,
      })
    })
  })
}

// ===== v3.9-4 MCP 深度集成：工具名命名空间前缀 =====
describe('MCPClient toolPrefix (v3.9-4)', () => {
  it('listTools returns prefixed names when toolPrefix set', async () => {
    const mock = await startMockMCPServer({
      'tools/list': () => ({
        tools: [
          { name: 'get_weather', description: 'weather', inputSchema: { type: 'object', properties: {} } },
        ],
      }),
    })
    try {
      const client = new MCPClient({ transport: 'sse', url: mock.url, toolPrefix: 'github' })
      const tools = await client.listTools()
      expect(tools[0].name).toBe('github_get_weather')
    } finally {
      await mock.close()
    }
  })

  it('callTool strips prefix and calls raw MCP tool name', async () => {
    const mock = await startMockMCPServer({
      'tools/call': (params) => ({
        content: [{ type: 'text', text: `called:${params.name}:${JSON.stringify(params.arguments)}` }],
      }),
    })
    try {
      const client = new MCPClient({ transport: 'sse', url: mock.url, toolPrefix: 'github' })
      const result = await client.callTool('github_get_weather', { city: 'Beijing' })
      expect(result.content[0].text).toBe('called:get_weather:{"city":"Beijing"}')
      expect(mock.received).toHaveLength(1)
      expect(mock.received[0].params.name).toBe('get_weather')
    } finally {
      await mock.close()
    }
  })

  it('setToolPrefix works after construction', async () => {
    const mock = await startMockMCPServer({
      'tools/list': () => ({ tools: [{ name: 'read_file', description: 'read', inputSchema: {} }] }),
      'tools/call': (params) => ({ content: [{ type: 'text', text: params.name }] }),
    })
    try {
      const client = new MCPClient({ transport: 'sse', url: mock.url })
      client.setToolPrefix('filesystem')
      const tools = await client.listTools()
      expect(tools[0].name).toBe('filesystem_read_file')
      const result = await client.callTool('filesystem_read_file', { path: '/tmp/a' })
      expect(result.content[0].text).toBe('read_file')
    } finally {
      await mock.close()
    }
  })

  it('without toolPrefix, names pass through unchanged', async () => {
    const mock = await startMockMCPServer({
      'tools/list': () => ({ tools: [{ name: 'echo', description: 'echo', inputSchema: {} }] }),
    })
    try {
      const client = new MCPClient({ transport: 'sse', url: mock.url })
      const tools = await client.listTools()
      expect(tools[0].name).toBe('echo')
    } finally {
      await mock.close()
    }
  })

  it('edge prefix underscores are trimmed', async () => {
    const mock = await startMockMCPServer({
      'tools/list': () => ({ tools: [{ name: 'ping', description: 'p', inputSchema: {} }] }),
    })
    try {
      const client = new MCPClient({ transport: 'sse', url: mock.url, toolPrefix: '__gh__' })
      const tools = await client.listTools()
      expect(tools[0].name).toBe('gh_ping')
    } finally {
      await mock.close()
    }
  })
})
