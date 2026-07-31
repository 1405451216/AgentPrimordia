/**
 * MCP 协议客户端示例 — Model Context Protocol 集成
 *
 * 运行: npx tsx examples/mcp-client.ts
 */
import { MCPClient } from '../src/mcp/client.js';

async function main() {
  console.log('=== AgentPrimordia TS SDK: MCP Client ===\n');

  // 创建 MCP 客户端（连接到本地 MCP 服务器）
  const client = new MCPClient({
    transport: 'stdio',
    command: 'npx',
    args: ['-y', '@modelcontextprotocol/server-filesystem', '/tmp'],
  });

  console.log('MCP Client configured:');
  console.log(`  Transport: stdio`);
  console.log(`  Command: npx -y @modelcontextprotocol/server-filesystem /tmp`);

  // 注意：实际连接需要 MCP 服务器运行
  // 以下演示 API 用法（不实际连接）
  console.log('\n--- MCP Protocol API ---');
  console.log('client.connect()     — 建立连接');
  console.log('client.listTools()   — 发现可用工具');
  console.log('client.callTool()    — 调用远程工具');
  console.log('client.listResources() — 列出资源');
  console.log('client.readResource()  — 读取资源');
  console.log('client.disconnect()  — 断开连接');

  // 工具注册到 Agent 的 ToolRegistry
  console.log('\n--- Integration with Agent ---');
  console.log('MCP 工具自动注册到 ToolRegistry，Agent 可直接调用远程 MCP 工具。');
  console.log('支持 stdio / SSE / WebSocket 三种传输方式。');

  console.log('\n--- Done ---');
}

main().catch(console.error);
