/**
 * steady-state.ts — 稳态验证器
 *
 * 对齐 Go 端 internal/chaos/steady_state.go
 * Stability: Experimental
 */

/** 稳态检查结果 */
export interface SteadyStateResult {
  met: boolean;
  message: string;
  details?: Record<string, unknown>;
}

/** 稳态检查接口（对齐 Go SteadyState） */
export interface SteadyState {
  /** 执行稳态检查 */
  check(): Promise<SteadyStateResult>;
  /** 稳态条件名称 */
  name(): string;
}

/** SLO 稳态验证（可用性阈值） */
export class SLOSteadyState implements SteadyState {
  private readonly metricName: string;
  private readonly threshold: number;
  private readonly measureFn: () => Promise<number>;

  constructor(metricName: string, threshold: number, measureFn: () => Promise<number>) {
    this.metricName = metricName;
    this.threshold = threshold;
    this.measureFn = measureFn;
  }

  async check(): Promise<SteadyStateResult> {
    const value = await this.measureFn();
    const met = value >= this.threshold;
    return {
      met,
      message: met
        ? `${this.metricName} = ${(value * 100).toFixed(2)}% ≥ ${(this.threshold * 100).toFixed(2)}%`
        : `${this.metricName} = ${(value * 100).toFixed(2)}% < ${(this.threshold * 100).toFixed(2)}% (SLO 违反)`,
      details: { metric: this.metricName, value, threshold: this.threshold },
    };
  }

  name(): string { return `slo:${this.metricName}`; }
}

/** 可用性稳态验证 */
export class AvailabilitySteadyState implements SteadyState {
  private readonly threshold: number;
  private readonly healthCheckFn: () => Promise<boolean>;

  constructor(threshold: number, healthCheckFn: () => Promise<boolean>) {
    this.threshold = threshold;
    this.healthCheckFn = healthCheckFn;
  }

  async check(): Promise<SteadyStateResult> {
    const available = await this.healthCheckFn();
    const value = available ? 1.0 : 0.0;
    const met = value >= this.threshold;
    return {
      met,
      message: met ? '服务可用' : '服务不可用',
      details: { available, threshold: this.threshold },
    };
  }

  name(): string { return 'availability'; }
}

/** 延迟稳态验证 */
export class LatencySteadyState implements SteadyState {
  private readonly maxLatencyMs: number;
  private readonly measureFn: () => Promise<number>;

  constructor(maxLatencyMs: number, measureFn: () => Promise<number>) {
    this.maxLatencyMs = maxLatencyMs;
    this.measureFn = measureFn;
  }

  async check(): Promise<SteadyStateResult> {
    const latency = await this.measureFn();
    const met = latency <= this.maxLatencyMs;
    return {
      met,
      message: met
        ? `延迟 ${latency.toFixed(1)}ms ≤ ${this.maxLatencyMs}ms`
        : `延迟 ${latency.toFixed(1)}ms > ${this.maxLatencyMs}ms (超标)`,
      details: { latencyMs: latency, maxMs: this.maxLatencyMs },
    };
  }

  name(): string { return 'latency'; }
}

/** 自定义稳态检查 */
export class CustomSteadyState implements SteadyState {
  private readonly nameStr: string;
  private readonly checkFn: () => Promise<SteadyStateResult>;

  constructor(name: string, checkFn: () => Promise<SteadyStateResult>) {
    this.nameStr = name;
    this.checkFn = checkFn;
  }

  async check(): Promise<SteadyStateResult> {
    return this.checkFn();
  }

  name(): string { return this.nameStr; }
}
