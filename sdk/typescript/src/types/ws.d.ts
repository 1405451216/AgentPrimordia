declare module 'ws' {
  export class WebSocketServer {
    constructor(options?: Record<string, any>);
    on(event: string, listener: (...args: any[]) => void): this;
    close(cb?: (err?: Error) => void): void;
  }

  export class WebSocket {
    static readonly CONNECTING: 0;
    static readonly OPEN: 1;
    static readonly CLOSING: 2;
    static readonly CLOSED: 3;
    readyState: number;
    send(data: any, cb?: (err?: Error) => void): void;
    close(code?: number, reason?: string): void;
    on(event: string, listener: (...args: any[]) => void): this;
  }

  export { WebSocketServer as Server };
}
