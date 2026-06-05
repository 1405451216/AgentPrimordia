export class MetricsCollector {
  llmTotalCalls = 0;
  llmTotalErrors = 0;
  toolTotalCalls = 0;
  toolTotalErrors = 0;
  totalTurns = 0;
  activeAgents = 0;

  private static readonly MAX_HISTORY = 10000;
  private llmLatencies: number[] = [];
  private toolLatencies: number[] = [];

  recordLLMCall(durationMs: number, error?: Error): void {
    this.llmTotalCalls++;
    if (error) this.llmTotalErrors++;
    this.llmLatencies.push(durationMs);
    if (this.llmLatencies.length > MetricsCollector.MAX_HISTORY) {
      this.llmLatencies = this.llmLatencies.slice(-MetricsCollector.MAX_HISTORY);
    }
  }

  recordToolCall(durationMs: number, error?: Error): void {
    this.toolTotalCalls++;
    if (error) this.toolTotalErrors++;
    this.toolLatencies.push(durationMs);
    if (this.toolLatencies.length > MetricsCollector.MAX_HISTORY) {
      this.toolLatencies = this.toolLatencies.slice(-MetricsCollector.MAX_HISTORY);
    }
  }

  recordTurn(): void {
    this.totalTurns++;
  }

  incActiveAgents(): void { this.activeAgents++; }
  decActiveAgents(): void { this.activeAgents = Math.max(0, this.activeAgents - 1); }

  avgLLMLatency(): number {
    return this.llmLatencies.length > 0
      ? this.llmLatencies.reduce((a, b) => a + b, 0) / this.llmLatencies.length
      : 0;
  }

  avgToolLatency(): number {
    return this.toolLatencies.length > 0
      ? this.toolLatencies.reduce((a, b) => a + b, 0) / this.toolLatencies.length
      : 0;
  }

  reset(): void {
    this.llmTotalCalls = 0;
    this.llmTotalErrors = 0;
    this.toolTotalCalls = 0;
    this.toolTotalErrors = 0;
    this.totalTurns = 0;
    this.activeAgents = 0;
    this.llmLatencies = [];
    this.toolLatencies = [];
  }

  snapshot(): Record<string, number> {
    return {
      llm_total_calls: this.llmTotalCalls,
      llm_total_errors: this.llmTotalErrors,
      tool_total_calls: this.toolTotalCalls,
      tool_total_errors: this.toolTotalErrors,
      total_turns: this.totalTurns,
      active_agents: this.activeAgents,
      avg_llm_latency_ms: this.avgLLMLatency(),
      avg_tool_latency_ms: this.avgToolLatency(),
    };
  }
}
