// Inspector & Debug Server — HTTP API + Web UI for Agent tracing and debugging
// Mirrors Go internal/debugger/http.go + inspector_server.go

import type { IncomingMessage, ServerResponse } from 'node:http';
import type { Debugger, DebugEvent } from '../metrics/otel-prometheus.js';
import type { OTelSpan } from '../metrics/otel-prometheus.js';

// ===== Memory Snapshot =====

export interface MemorySnapshot {
  totalEpisodes: number;
  topSessions: Array<{ sessionID: string; count: number }>;
  recentEvents: DebugEvent[];
  timestamp: string;
}

// ===== Session Trace =====

export interface SessionTrace {
  sessionID: string;
  spans: OTelSpan[];
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  startTime: string;
  endTime: string;
}

// ===== Inspector =====

export interface InspectorConfig {
  maxSpans?: number;
}

export class Inspector {
  private traces: OTelSpan[] = [];
  private sessions: Map<string, SessionTrace> = new Map();
  private maxSpans: number;

  constructor(config?: InspectorConfig) {
    this.maxSpans = config?.maxSpans ?? 10_000;
  }

  /** Record a span. */
  recordSpan(span: OTelSpan): void {
    if (this.traces.length >= this.maxSpans) {
      this.traces.shift();
    }
    this.traces.push(span);
  }

  /** Record a session trace. */
  recordSession(trace: SessionTrace): void {
    this.sessions.set(trace.sessionID, trace);
  }

  getTraces(): OTelSpan[] {
    return [...this.traces];
  }

  getSessionTrace(sessionID: string): SessionTrace | undefined {
    return this.sessions.get(sessionID);
  }

  getAllSessions(): string[] {
    return Array.from(this.sessions.keys());
  }

  getStats(): { totalTraces: number; totalSessions: number } {
    return { totalTraces: this.traces.length, totalSessions: this.sessions.size };
  }

  clear(): void {
    this.traces = [];
    this.sessions.clear();
  }
}

// ===== Inspector Server (HTTP) =====

export class InspectorServer {
  private inspector: Inspector;

  constructor(inspector: Inspector) {
    this.inspector = inspector;
  }

  async handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
    const url = req.url ?? '/';
    const method = req.method ?? 'GET';

    if (method !== 'GET') {
      this.writeJSON(res, 405, { error: 'Method not allowed' });
      return;
    }

    if (url === '/inspector' || url === '/inspector/') {
      this.handleUI(res);
      return;
    }

    if (url === '/api/inspector/traces') {
      this.writeJSON(res, 200, this.inspector.getTraces());
      return;
    }

    if (url === '/api/inspector/sessions') {
      this.writeJSON(res, 200, this.inspector.getAllSessions());
      return;
    }

    if (url.startsWith('/api/inspector/session/')) {
      const sessionID = url.slice('/api/inspector/session/'.length);
      if (!sessionID) {
        this.writeJSON(res, 400, { error: 'Session ID required' });
        return;
      }
      const trace = this.inspector.getSessionTrace(sessionID);
      if (!trace) {
        this.writeJSON(res, 404, { error: 'Session not found' });
        return;
      }
      this.writeJSON(res, 200, trace);
      return;
    }

    if (url === '/api/inspector/stats') {
      this.writeJSON(res, 200, this.inspector.getStats());
      return;
    }

    this.writeJSON(res, 404, { error: 'Not found' });
  }

  private handleUI(res: ServerResponse): void {
    res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    res.end(INSPECTOR_HTML);
  }

  private writeJSON(res: ServerResponse, status: number, data: unknown): void {
    res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' });
    res.end(JSON.stringify(data));
  }
}

// ===== Debug Server (HTTP) =====

export class DebugServer {
  private debugger: Debugger;
  private events: DebugEvent[] = [];
  private snapshots: MemorySnapshot[] = [];
  private maxEvents: number;
  private maxSnapshots: number;

  constructor(debuggerInstance: Debugger, opts?: { maxEvents?: number; maxSnapshots?: number }) {
    this.debugger = debuggerInstance;
    this.maxEvents = opts?.maxEvents ?? 100;
    this.maxSnapshots = opts?.maxSnapshots ?? 10;
  }

  addEvent(type: DebugEvent['type'], message: string): void {
    if (this.events.length >= this.maxEvents) {
      this.events.shift();
    }
    this.events.push({
      type,
      timestamp: new Date(),
      data: { message },
    });
  }

  addSnapshot(snapshot: MemorySnapshot): void {
    if (this.snapshots.length >= this.maxSnapshots) {
      this.snapshots.shift();
    }
    this.snapshots.push(snapshot);
  }

  async handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
    const url = req.url ?? '/';
    const method = req.method ?? 'GET';

    if (method !== 'GET') {
      this.writeJSON(res, 405, { error: 'Method not allowed' });
      return;
    }

    if (url === '/' || url === '/debug') {
      this.handleIndex(res);
      return;
    }

    if (url === '/api/events') {
      // Merge debugger events with local events
      const allEvents = [...this.debugger.getEvents(), ...this.events];
      this.writeJSON(res, 200, allEvents.map((e) => ({
        type: e.type,
        message: e.data?.message ?? JSON.stringify(e.data),
        timestamp: e.timestamp instanceof Date ? e.timestamp.toISOString() : String(e.timestamp),
      })));
      return;
    }

    if (url === '/api/snapshots') {
      this.writeJSON(res, 200, this.snapshots);
      return;
    }

    if (url === '/api/debug/report') {
      res.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' });
      res.end(this.debugger.report());
      return;
    }

    this.writeJSON(res, 404, { error: 'Not found' });
  }

  private handleIndex(res: ServerResponse): void {
    res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    res.end(DEBUG_HTML);
  }

  private writeJSON(res: ServerResponse, status: number, data: unknown): void {
    res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' });
    res.end(JSON.stringify(data));
  }
}

// ===== HTML Pages =====

const INSPECTOR_HTML = `<!DOCTYPE html>
<html>
<head>
  <title>Agent Inspector</title>
  <meta charset="utf-8">
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
    .container { max-width: 1200px; margin: 0 auto; }
    h1 { color: #333; margin-bottom: 30px; }
    .card { background: white; border-radius: 8px; padding: 20px; margin-bottom: 20px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
    table { width: 100%; border-collapse: collapse; }
    th, td { padding: 12px; text-align: left; border-bottom: 1px solid #eee; }
    th { background: #f9f9f9; color: #666; }
    tr:hover { background: #f9f9f9; }
    .badge { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 12px; font-weight: bold; }
    .badge-ok { background: #d4edda; color: #155724; }
    .badge-error { background: #f8d7da; color: #721c24; }
    .badge-unset { background: #e2e3e5; color: #383d41; }
  </style>
</head>
<body>
  <div class="container">
    <h1>🔍 Agent Inspector</h1>
    <div class="card">
      <h2>Stats</h2>
      <div id="stats">Loading...</div>
    </div>
    <div class="card">
      <h2>Sessions</h2>
      <div id="sessions">Loading...</div>
    </div>
    <div class="card">
      <h2>Recent Traces</h2>
      <div id="traces">Loading...</div>
    </div>
  </div>
  <script>
    async function loadStats() {
      const res = await fetch('/api/inspector/stats');
      const stats = await res.json();
      document.getElementById('stats').innerHTML =
        '<p><strong>Total Traces:</strong> ' + stats.totalTraces + '</p>' +
        '<p><strong>Total Sessions:</strong> ' + stats.totalSessions + '</p>';
    }
    async function loadSessions() {
      const res = await fetch('/api/inspector/sessions');
      const sessions = await res.json();
      document.getElementById('sessions').innerHTML =
        '<table><thead><tr><th>Session ID</th></tr></thead><tbody>' +
        sessions.map(s => '<tr><td>' + s + '</td></tr>').join('') +
        '</tbody></table>';
    }
    async function loadTraces() {
      const res = await fetch('/api/inspector/traces');
      const traces = await res.json();
      document.getElementById('traces').innerHTML =
        '<table><thead><tr><th>Name</th><th>Kind</th><th>Status</th><th>Start</th></tr></thead><tbody>' +
        traces.slice(0, 50).map(t =>
          '<tr><td>' + t.name + '</td><td>' + (t.kind || '') + '</td>' +
          '<td><span class="badge badge-' + (t.status || 'unset') + '">' + (t.status || 'unset') + '</span></td>' +
          '<td>' + new Date(t.startTime).toLocaleString() + '</td></tr>'
        ).join('') +
        '</tbody></table>';
    }
    loadStats(); loadSessions(); loadTraces();
    setInterval(() => { loadStats(); loadSessions(); loadTraces(); }, 5000);
  </script>
</body>
</html>`;

const DEBUG_HTML = `<!DOCTYPE html>
<html>
<head>
  <title>Agent Debugger</title>
  <meta charset="utf-8">
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
    .container { max-width: 1200px; margin: 0 auto; }
    h1 { color: #333; margin-bottom: 30px; }
    .card { background: white; border-radius: 8px; padding: 20px; margin-bottom: 20px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
    table { width: 100%; border-collapse: collapse; }
    th, td { padding: 12px; text-align: left; border-bottom: 1px solid #eee; }
    th { background: #f9f9f9; color: #666; }
    tr:hover { background: #f9f9f9; }
  </style>
</head>
<body>
  <div class="container">
    <h1>🔧 Agent Debugger</h1>
    <div class="card">
      <h2>Events</h2>
      <div id="events-container">Loading...</div>
    </div>
    <div class="card">
      <h2>Memory Snapshots</h2>
      <div id="snapshots-container">Loading...</div>
    </div>
    <div class="card">
      <h2>Debug Report</h2>
      <pre id="report-container">Loading...</pre>
    </div>
  </div>
  <script>
    async function loadEvents() {
      const res = await fetch('/api/events');
      const events = await res.json();
      document.getElementById('events-container').innerHTML =
        '<table><thead><tr><th>Timestamp</th><th>Type</th><th>Message</th></tr></thead><tbody>' +
        events.map(e => '<tr><td>' + e.timestamp + '</td><td>' + e.type + '</td><td>' + e.message + '</td></tr>').join('') +
        '</tbody></table>';
    }
    async function loadSnapshots() {
      const res = await fetch('/api/snapshots');
      const snapshots = await res.json();
      document.getElementById('snapshots-container').innerHTML =
        '<table><thead><tr><th>Total Episodes</th><th>Top Sessions</th><th>Recent Events</th></tr></thead><tbody>' +
        snapshots.map(s => '<tr><td>' + s.totalEpisodes + '</td><td>' +
          (s.topSessions ? s.topSessions.map(t => t.sessionID + '(' + t.count + ')').join(', ') : 'None') +
          '</td><td>' + (s.recentEvents ? s.recentEvents.length : 0) + ' events</td></tr>').join('') +
        '</tbody></table>';
    }
    async function loadReport() {
      const res = await fetch('/api/debug/report');
      const text = await res.text();
      document.getElementById('report-container').textContent = text;
    }
    loadEvents(); loadSnapshots(); loadReport();
    setInterval(() => { loadEvents(); loadSnapshots(); }, 3000);
  </script>
</body>
</html>`;
