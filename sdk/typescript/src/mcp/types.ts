export interface MCPToolDefinition {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
}

export interface MCPToolCall {
  name: string;
  arguments: Record<string, unknown>;
}

export interface MCPToolResult {
  content: Array<{ type: 'text' | 'image'; text?: string; data?: string; mimeType?: string }>;
  isError?: boolean;
}

export interface MCPServerConfig {
  name: string;
  version: string;
  transport: 'stdio' | 'http';
  command?: string;
  args?: string[];
  url?: string;
}

export interface MCPListToolsResponse {
  tools: MCPToolDefinition[];
}

export class MCPClient {
  private tools: MCPToolDefinition[] = [];

  constructor(private config: MCPServerConfig) {}

  async connect(): Promise<void> {
    // Placeholder: real implementation would use stdio/http transport
  }

  async listTools(): Promise<MCPToolDefinition[]> {
    return this.tools;
  }

  async callTool(call: MCPToolCall): Promise<MCPToolResult> {
    return {
      content: [{ type: 'text', text: `MCP tool ${call.name} not yet connected` }],
      isError: true,
    };
  }

  async disconnect(): Promise<void> {
    this.tools = [];
  }
}
