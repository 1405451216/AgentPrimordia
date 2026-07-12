/**
 * TraceContext — W3C Trace Context 兼容（v2.0 #16 统一 Trace 跨端关联）
 * 与 Go 端 protocol/trace.go 对齐。
 */
export class TraceContext {
  constructor(
    public traceID: string = generateHex(16),
    public spanID: string = generateHex(8),
    public sampled: boolean = true,
  ) {}

  /** 从 W3C traceparent header 解析 */
  static fromHeaders(headers: Record<string, string>): TraceContext | null {
    const traceparent = headers['traceparent'];
    if (!traceparent) return null;
    const parts = traceparent.split('-');
    if (parts.length < 4) return null;
    return new TraceContext(parts[1], parts[2], parts[3].endsWith('1'));
  }

  /** 注入到 headers */
  toHeaders(): Record<string, string> {
    const flags = this.sampled ? '01' : '00';
    return {
      traceparent: `00-${this.traceID}-${this.spanID}-${flags}`,
    };
  }

  /** 转为 W3C traceparent 字符串 */
  toTraceparent(): string {
    const flags = this.sampled ? '01' : '00';
    return `00-${this.traceID}-${this.spanID}-${flags}`;
  }
}

function generateHex(length: number): string {
  const array = new Uint8Array(length / 2 + 1);
  crypto.getRandomValues(array);
  let hex = '';
  for (let i = 0; i < array.length; i++) {
    hex += array[i].toString(16).padStart(2, '0');
  }
  return hex.substring(0, length);
}

