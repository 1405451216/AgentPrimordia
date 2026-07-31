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

/** 稳态条件接口（对齐 Go SteadyState） */
export interface SteadyState {
  /** 执行稳态检查 */
  check(): Promise<SteadyStateResult>;
  /** 稳态条件名称 */
  name(): string;
}

/** SLO 稳态验证（对齐 Go SLOSteadyState） */
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
        ? `${this.metricName} = ${(value * 100).toFixed(2)}% >= ${(this.threshold * 100).toFixed(2)}%`
        : `${this.metricName} = ${(value * 100).toFixed(2)}% < ${(this.threshold * 100).toFixed(2)}% (SLO 违反)`,
      details: { metric: this.metricName, value, threshold: this.threshold },
    };
  }

  name(): string { return this.metricName; }
}

/** 可用性稳态验证（对齐 Go AvailabilitySteadyState） */
export class AvailabilitySteadyState implements SteadyState {
  private readonly nameStr: string;
  private readonly target: number;
  private readonly checkFn: () => { total: number; failures: number };

  constructor(name: string, target: number, checkFn: () => { total: number; failures: number }) {
    this.nameStr = name;
    this.target = target;
    this.checkFn = checkFn;
  }

  async check(): Promise<SteadyStateResult> {
    const { total, failures } = this.checkFn();
    const avail = total === 0 ? 1.0 : (total - failures) / total;
    const met = avail >= this.target;
    return {
      met,
      message: `可用性 ${avail.toFixed(4)} (目标 ${this.target.toFixed(4)})`,
      details: { total, failures, availability: avail, target: this.target },
    };
  }

  name(): string { return this.nameStr; }
}

/** 延迟稳态验证（对齐 Go LatencySteadyState） */
export class LatencySteadyState implements SteadyState {
  private readonly nameStr: string;
  private readonly p99TargetMs: number;
  private samples: number[] = [];

  constructor(name: string, p99TargetMs: number) {
    this.nameStr = name;
    this.p99TargetMs = p99TargetMs;
  }

  /** 记录一个延迟样本 */
  record(latencyMs: number): void {
    this.samples.push(latencyMs);
  }

  async check(): Promise<SteadyStateResult> {
    if (this.samples.length === 0) {
      return { met: true, message: '无延迟样本' };
    }
    const sorted = [...this.samples].sort((a, b) => a - b);
    const p99Index = Math.ceil(sorted.length * 0.99) - 1;
    const p99 = sorted[Math.max(0, p99Index)];
    const met = p99 <= this.p99TargetMs;
    return {
      met,
      message: `P99 延迟 ${p99.toFixed(1)}ms (目标 ${this.p99TargetMs}ms)`,
      details: { p99: p99.toFixed(1), target: this.p99TargetMs, samples: this.samples.length },
    };
  }

  name(): string { return this.nameStr; }
}

/** 组合稳态条件 — 所有条件都必须满足（对齐 Go CompositeSteadyState） */
export class CompositeSteadyState implements SteadyState {
  private readonly nameStr: string;
  private readonly states: SteadyState[];

  constructor(name: string, states: SteadyState[]) {
    this.nameStr = name;
    this.states = states;
  }

  async check(): Promise<SteadyStateResult> {
    let allMet = true;
    const details: Record<string, unknown> = {};
    const messages: string[] = [];

    for (const ss of this.states) {
      const result = await ss.check();
      if (!result.met) allMet = false;
      details[ss.name()] = result;
      messages.push(`${ss.name()}: ${result.message}`);
    }

    return {
      met: allMet,
      message: allMet ? '所有稳态条件满足' : '稳态条件不满足',
      details,
    };
  }

  name(): string { return this.nameStr; }
}

/** 自定义稳态检查（对齐 Go 的 AlwaysMetSteadyState / NeverMetSteadyState / ToggleSteadyState 统一为 CustomSteadyState） */
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
