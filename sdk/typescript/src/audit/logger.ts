// Audit Logger — Compliance audit trail for Agent operations
// Mirrors Go internal/audit/logger.go

// ===== Audit Event =====

export interface AuditEvent {
  timestamp: string;
  actor: string;
  action: string;
  resource: string;
  details?: Record<string, unknown>;
  result: string;
}

// ===== Query Filter =====

export interface QueryFilter {
  actor?: string;
  action?: string;
  resource?: string;
  start?: string;
  end?: string;
  limit?: number;
}

// ===== Actor Stats =====

export interface ActorStats {
  totalActions: number;
  actions: Map<string, number>;
}

// ===== Compliance Report =====

export interface PeriodStats {
  start: string;
  end: string;
}

export interface ComplianceReport {
  period: PeriodStats;
  totalEvents: number;
  actorStats: Map<string, ActorStats>;
  actionStats: Map<string, number>;
}

// ===== Output Interface =====

export interface AuditOutput {
  write(event: AuditEvent): Promise<void>;
  query(filter: QueryFilter): Promise<AuditEvent[]>;
}

// ===== In-Memory Output (default) =====

export class InMemoryAuditOutput implements AuditOutput {
  private events: AuditEvent[] = [];

  async write(event: AuditEvent): Promise<void> {
    this.events.push(event);
  }

  async query(filter: QueryFilter): Promise<AuditEvent[]> {
    let result = [...this.events];

    if (filter.actor) result = result.filter((e) => e.actor === filter.actor);
    if (filter.action) result = result.filter((e) => e.action === filter.action);
    if (filter.resource) result = result.filter((e) => e.resource === filter.resource);
    if (filter.start) result = result.filter((e) => e.timestamp >= filter.start!);
    if (filter.end) result = result.filter((e) => e.timestamp <= filter.end!);

    if (filter.limit && filter.limit > 0) {
      result = result.slice(0, filter.limit);
    }

    return result;
  }

  clear(): void {
    this.events = [];
  }

  count(): number {
    return this.events.length;
  }
}

// ===== Audit Logger =====

export interface AuditLoggerConfig {
  output: AuditOutput;
}

export class AuditLogger {
  private config: AuditLoggerConfig;

  constructor(config: AuditLoggerConfig) {
    if (!config.output) {
      throw new Error('audit: LoggerConfig.output cannot be null');
    }
    this.config = config;
  }

  /** Log an audit event. Auto-fills timestamp if empty. */
  async log(event: Partial<AuditEvent> & { actor: string; action: string }): Promise<void> {
    const fullEvent: AuditEvent = {
      timestamp: event.timestamp ?? new Date().toISOString(),
      actor: event.actor,
      action: event.action,
      resource: event.resource ?? '',
      details: event.details,
      result: event.result ?? 'success',
    };
    await this.config.output.write(fullEvent);
  }

  /** Query audit events by filter. */
  async query(filter: QueryFilter): Promise<AuditEvent[]> {
    return this.config.output.query(filter);
  }

  /** Generate a compliance report for a time range. */
  async generateReport(start: string, end: string): Promise<ComplianceReport> {
    const events = await this.config.output.query({ start, end });

    const report: ComplianceReport = {
      period: { start, end },
      totalEvents: events.length,
      actorStats: new Map(),
      actionStats: new Map(),
    };

    for (const e of events) {
      // Actor stats
      let as = report.actorStats.get(e.actor);
      if (!as) {
        as = { totalActions: 0, actions: new Map() };
        report.actorStats.set(e.actor, as);
      }
      as.totalActions++;
      as.actions.set(e.action, (as.actions.get(e.action) ?? 0) + 1);

      // Action stats
      report.actionStats.set(e.action, (report.actionStats.get(e.action) ?? 0) + 1);
    }

    return report;
  }

  /** Export a compliance report as formatted JSON. */
  exportReportJSON(report: ComplianceReport): string {
    return JSON.stringify({
      period: report.period,
      totalEvents: report.totalEvents,
      actorStats: Object.fromEntries(
        Array.from(report.actorStats.entries()).map(([k, v]) => [
          k,
          { totalActions: v.totalActions, actions: Object.fromEntries(v.actions) },
        ])
      ),
      actionStats: Object.fromEntries(report.actionStats),
    }, null, 2);
  }
}
