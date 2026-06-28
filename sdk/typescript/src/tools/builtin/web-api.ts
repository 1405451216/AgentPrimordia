import type { Tool } from '../../types.js';

export interface WebConfig {
  timeoutMs?: number;
  maxResponseLength?: number;
  allowedDomains?: string[];
  blockedDomains?: string[];
}

/**
 * Web tool — HTTP GET/POST requests.
 */
export class WebTool implements Tool {
  name = 'web';
  description = 'Make HTTP GET and POST requests to fetch web content';
  parameters = {
    type: 'object',
    properties: {
      method: { type: 'string', enum: ['GET', 'POST', 'PUT', 'DELETE'], description: 'HTTP method' },
      url: { type: 'string', description: 'The URL to request' },
      headers: { type: 'object', description: 'HTTP headers (optional)' },
      body: { type: 'string', description: 'Request body (for POST/PUT)' },
    },
    required: ['url'],
  };

  private config: Required<WebConfig>;

  constructor(config?: WebConfig) {
    this.config = {
      timeoutMs: config?.timeoutMs ?? 30_000,
      maxResponseLength: config?.maxResponseLength ?? 50_000,
      allowedDomains: config?.allowedDomains ?? [],
      blockedDomains: config?.blockedDomains ?? [],
    };
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const url = args.url as string;
    if (!url?.trim()) return 'Error: URL is required';

    // Validate URL
    let parsedUrl: URL;
    try {
      parsedUrl = new URL(url);
    } catch {
      return `Error: invalid URL: ${url}`;
    }

    // Check domain restrictions
    const domain = parsedUrl.hostname;
    if (this.config.blockedDomains.includes(domain)) {
      return `Error: domain "${domain}" is blocked`;
    }
    if (this.config.allowedDomains.length > 0 && !this.config.allowedDomains.includes(domain)) {
      return `Error: domain "${domain}" is not in allowed list`;
    }

    const method = (args.method as string) ?? 'GET';
    const headers = (args.headers as Record<string, string>) ?? {};
    const body = args.body as string | undefined;

    try {
      const resp = await fetch(url, {
        method,
        headers,
        body: body ?? undefined,
        signal: AbortSignal.timeout(this.config.timeoutMs),
      });

      const text = await resp.text();
      let result = `Status: ${resp.status} ${resp.statusText}\n`;
      result += `Content-Type: ${resp.headers.get('content-type') ?? 'unknown'}\n\n`;

      if (text.length > this.config.maxResponseLength) {
        result += text.slice(0, this.config.maxResponseLength) + '\n... (truncated)';
      } else {
        result += text;
      }

      return result;
    } catch (err) {
      return `Error: ${(err as Error).message}`;
    }
  }
}

/**
 * API tool — REST API calls with whitelist and timeout.
 */
export class APITool implements Tool {
  name = 'api';
  description = 'Make REST API calls with endpoint whitelist and authentication';
  parameters = {
    type: 'object',
    properties: {
      endpoint: { type: 'string', description: 'API endpoint path' },
      method: { type: 'string', enum: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'] },
      params: { type: 'object', description: 'Query parameters (for GET)' },
      body: { type: 'object', description: 'Request body (for POST/PUT/PATCH)' },
      headers: { type: 'object', description: 'Additional headers' },
    },
    required: ['endpoint'],
  };

  private baseURL: string;
  private defaultHeaders: Record<string, string>;
  private allowedEndpoints: string[];

  constructor(config: { baseURL: string; headers?: Record<string, string>; allowedEndpoints?: string[] }) {
    this.baseURL = config.baseURL.replace(/\/+$/, '');
    this.defaultHeaders = config.headers ?? {};
    this.allowedEndpoints = config.allowedEndpoints ?? [];
  }

  async execute(args: Record<string, unknown>): Promise<string> {
    const endpoint = args.endpoint as string;
    const method = (args.method as string) ?? 'GET';

    // Check endpoint whitelist
    if (this.allowedEndpoints.length > 0) {
      const matched = this.allowedEndpoints.some((ep) => endpoint.startsWith(ep));
      if (!matched) return `Error: endpoint "${endpoint}" is not in allowed list`;
    }

    const params = args.params as Record<string, string> | undefined;
    const body = args.body;
    const headers = { ...this.defaultHeaders, ...(args.headers as Record<string, string>) };

    let url = this.baseURL + '/' + endpoint.replace(/^\/+/, '');
    if (params) {
      const searchParams = new URLSearchParams(params);
      url += '?' + searchParams.toString();
    }

    try {
      const resp = await fetch(url, {
        method,
        headers,
        body: body ? JSON.stringify(body) : undefined,
        signal: AbortSignal.timeout(30_000),
      });

      const text = await resp.text();
      let result = `Status: ${resp.status} ${resp.statusText}\n\n`;

      // Try to parse as JSON for pretty-printing
      try {
        const json = JSON.parse(text);
        result += JSON.stringify(json, null, 2);
      } catch {
        result += text;
      }

      if (result.length > 50_000) {
        result = result.slice(0, 50_000) + '\n... (truncated)';
      }

      return result;
    } catch (err) {
      return `Error: ${(err as Error).message}`;
    }
  }
}
