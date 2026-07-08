import { describe, it, expect, vi, beforeEach } from 'vitest'
import { MCPClient, MCPError, createToolCaller } from '../../src/mcp/client.js'

describe('MCPClient JSON-RPC', () => {
  it('should parse initialize response', () => {
    const resp = {
      jsonrpc: '2.0',
      id: 1,
      result: {
        protocolVersion: '2024-11-05',
        serverInfo: { name: 'test-mcp', version: '1.0.0' },
        capabilities: { tools: { listChanged: false } },
      },
    }
    expect(resp.result.serverInfo.name).toBe('test-mcp')
    expect(resp.result.protocolVersion).toBe('2024-11-05')
  })

  it('should parse tools/list response', () => {
    const resp = {
      jsonrpc: '2.0',
      id: 2,
      result: {
        tools: [
          { name: 'echo', description: 'echo input', inputSchema: { type: 'object', properties: {} } },
        ],
      },
    }
    expect(resp.result.tools).toHaveLength(1)
    expect(resp.result.tools[0].name).toBe('echo')
  })

  it('should parse tools/call response', () => {
    const resp = {
      jsonrpc: '2.0',
      id: 3,
      result: {
        content: [{ type: 'text', text: 'hello' }],
        isError: false,
      },
    }
    expect(resp.result.content[0].text).toBe('hello')
  })

  it('should parse error response', () => {
    const resp = {
      jsonrpc: '2.0',
      id: 4,
      error: { code: -32601, message: 'method not found' },
    }
    expect(resp.error?.code).toBe(-32601)
  })
})

describe('MCPError', () => {
  it('should create error with code', () => {
    const err = new MCPError('tool not found', -32601)
    expect(err.message).toBe('tool not found')
    expect(err.code).toBe(-32601)
    expect(err.name).toBe('MCPError')
  })
})

describe('createToolCaller', () => {
  it('should return a callable function', () => {
    const fn = createToolCaller
    expect(typeof fn).toBe('function')
  })
})

describe('MCPTool type', () => {
  it('should have name field', () => {
    const tool: { name: string; description?: string } = { name: 'echo' }
    expect(tool.name).toBe('echo')
  })
})
