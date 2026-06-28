// K8s Operator CRD Types — Declarative Agent Deployment

// ===== CRD Types =====

export interface AgentDeployment {
  apiVersion: string;
  kind: 'AgentDeployment';
  metadata: {
    name: string;
    namespace?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
  spec: AgentDeploymentSpec;
  status?: AgentDeploymentStatus;
}

export interface AgentDeploymentSpec {
  replicas: number;
  template: AgentTemplateSpec;
  autoscaling?: AutoscalingSpec;
  healthCheck?: HealthCheckSpec;
}

export interface AgentTemplateSpec {
  provider: string;
  model: string;
  systemPrompt: string;
  maxTurns?: number;
  apiSecretRef?: string;
  image?: string;
  tools?: ToolSpec[];
  memory?: MemorySpec;
  resources?: ResourceSpec;
  metrics?: MetricsSpec;
  tracing?: TracingSpec;
  environment?: Record<string, string>;
}

export interface ToolSpec {
  name: string;
  type: 'builtin' | 'mcp' | 'custom';
  config?: Record<string, unknown>;
}

export interface MemorySpec {
  enabled: boolean;
  backend: 'sqlite' | 'memory' | 'redis' | 'postgres';
  config?: Record<string, unknown>;
}

export interface ResourceSpec {
  cpu?: string;
  memory?: string;
  gpu?: string;
}

export interface AutoscalingSpec {
  enabled: boolean;
  minReplicas: number;
  maxReplicas: number;
  targetConcurrentTasks: number;
  scaleUpThreshold?: number;
  scaleDownThreshold?: number;
}

export interface HealthCheckSpec {
  enabled: boolean;
  interval?: number;
  timeout?: number;
  unhealthyThreshold?: number;
}

export interface MetricsSpec {
  enabled: boolean;
  port?: number;
  path?: string;
}

export interface TracingSpec {
  enabled: boolean;
  endpoint?: string;
  serviceName?: string;
  samplingRate?: number;
}

export interface AgentDeploymentStatus {
  ready: boolean;
  readyReplicas: number;
  phase: 'Pending' | 'Running' | 'Scaling' | 'Failed' | 'Terminated';
  message?: string;
  conditions?: Condition[];
  observedGeneration?: number;
}

export interface Condition {
  type: string;
  status: 'True' | 'False' | 'Unknown';
  reason?: string;
  message?: string;
  lastTransitionTime?: string;
}

// ===== YAML Manifest Helpers =====

export function basicAgentDeployment(name: string, opts: {
  provider: string;
  model: string;
  systemPrompt: string;
  replicas?: number;
  apiSecretRef?: string;
}): AgentDeployment {
  return {
    apiVersion: 'agentprimordia.io/v1',
    kind: 'AgentDeployment',
    metadata: { name },
    spec: {
      replicas: opts.replicas ?? 1,
      template: {
        provider: opts.provider,
        model: opts.model,
        systemPrompt: opts.systemPrompt,
        apiSecretRef: opts.apiSecretRef,
        maxTurns: 10,
      },
    },
  };
}

export function multiAgentDeployment(name: string, agents: Array<{
  name: string;
  provider: string;
  model: string;
  systemPrompt: string;
}>): AgentDeployment[] {
  return agents.map(a => basicAgentDeployment(a.name, a));
}

export function withAutoscaling(
  deployment: AgentDeployment,
  config: Partial<AutoscalingSpec>
): AgentDeployment {
  deployment.spec.autoscaling = {
    enabled: true,
    minReplicas: config.minReplicas ?? 1,
    maxReplicas: config.maxReplicas ?? 10,
    targetConcurrentTasks: config.targetConcurrentTasks ?? 5,
    scaleUpThreshold: config.scaleUpThreshold ?? 0.8,
    scaleDownThreshold: config.scaleDownThreshold ?? 0.2,
  };
  return deployment;
}

export function withHealthCheck(
  deployment: AgentDeployment,
  config?: Partial<HealthCheckSpec>
): AgentDeployment {
  deployment.spec.healthCheck = {
    enabled: true,
    interval: config?.interval ?? 30,
    timeout: config?.timeout ?? 5,
    unhealthyThreshold: config?.unhealthyThreshold ?? 3,
  };
  return deployment;
}

export function withMetrics(
  deployment: AgentDeployment,
  config?: Partial<MetricsSpec>
): AgentDeployment {
  deployment.spec.template.metrics = {
    enabled: true,
    port: config?.port ?? 9090,
    path: config?.path ?? '/metrics',
  };
  return deployment;
}

export function withTracing(
  deployment: AgentDeployment,
  config?: Partial<TracingSpec>
): AgentDeployment {
  deployment.spec.template.tracing = {
    enabled: true,
    endpoint: config?.endpoint ?? 'http://otel-collector:4317',
    serviceName: config?.serviceName ?? deployment.metadata.name,
    samplingRate: config?.samplingRate ?? 1.0,
  };
  return deployment;
}

// ===== YAML Serialization (simplified) =====

export function toYAML(deployment: AgentDeployment): string {
  return objectToYAML(deployment, 0);
}

function objectToYAML(obj: unknown, indent: number): string {
  if (obj === null || obj === undefined) return 'null\n';
  if (typeof obj === 'string') return obj.includes(':') || obj.includes('#') ? `"${obj}"\n` : `${obj}\n`;
  if (typeof obj === 'number' || typeof obj === 'boolean') return `${obj}\n`;

  const pad = '  '.repeat(indent);
  const lines: string[] = [];

  if (Array.isArray(obj)) {
    for (const item of obj) {
      if (typeof item === 'object' && item !== null) {
        const sub = objectToYAML(item, indent + 1).trimStart();
        lines.push(`${pad}- ${sub}`);
      } else {
        lines.push(`${pad}- ${item}`);
      }
    }
  } else if (typeof obj === 'object') {
    for (const [key, value] of Object.entries(obj as Record<string, unknown>)) {
      if (value === undefined || value === null) continue;
      if (typeof value === 'object') {
        lines.push(`${pad}${key}:`);
        lines.push(objectToYAML(value, indent + 1));
      } else {
        const valStr = typeof value === 'string' && (value.includes(':') || value.includes('#'))
          ? `"${value}"` : String(value);
        lines.push(`${pad}${key}: ${valStr}`);
      }
    }
  }

  return lines.join('\n') + (indent === 0 ? '' : '\n');
}
