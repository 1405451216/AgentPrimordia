import type { Provider } from '../llm/provider.js';

// ===== Planning Types =====

export type TaskStatus = 'pending' | 'running' | 'completed' | 'failed';

export interface SubTask {
  id: string;
  description: string;
  dependsOn: string[];
  status: TaskStatus;
  result?: string;
}

export interface Plan {
  goal: string;
  subTasks: SubTask[];
  createdAt: string;
}

export interface Planner {
  decompose(task: string): Promise<SubTask[]>;
  generatePlan(task: string): Promise<Plan>;
}

// ===== LLM Planner =====

export class LLMPlanner implements Planner {
  private provider: Provider;
  private model?: string;

  constructor(provider: Provider, model?: string) {
    this.provider = provider;
    this.model = model;
  }

  async decompose(task: string): Promise<SubTask[]> {
    const prompt = `Decompose the following task into executable subtasks.

Task: ${task}

Return a JSON array. Each subtask has:
- id: task identifier
- description: task description
- depends_on: array of dependency IDs (can be empty)

Example:
[{"id":"1","description":"First step","depends_on":[]},{"id":"2","description":"Second step","depends_on":["1"]}]

Return ONLY the JSON array.`;

    const resp = await this.provider.complete({
      messages: [{ role: 'user', content: prompt }],
      model: this.model,
      temperature: 0,
    });

    const subtasks = this.parseSubtasks(resp.content);
    return subtasks.map(s => ({ ...s, status: 'pending' as TaskStatus }));
  }

  async generatePlan(task: string): Promise<Plan> {
    const subTasks = await this.decompose(task);
    return {
      goal: task,
      subTasks,
      createdAt: new Date().toISOString(),
    };
  }

  private parseSubtasks(text: string): SubTask[] {
    try {
      const arr = JSON.parse(text);
      if (Array.isArray(arr)) return arr.map(this.normalizeSubTask);
    } catch {}
    const match = text.match(/\[[\s\S]*\]/);
    if (match) {
      try {
        const arr = JSON.parse(match[0]);
        if (Array.isArray(arr)) return arr.map(this.normalizeSubTask);
      } catch {}
    }
    return [];
  }

  private normalizeSubTask(raw: Record<string, unknown>): SubTask {
    return {
      id: String(raw.id ?? ''),
      description: String(raw.description ?? ''),
      dependsOn: Array.isArray(raw.depends_on) ? raw.depends_on.map(String) :
                 Array.isArray(raw.dependsOn) ? raw.dependsOn.map(String) : [],
      status: 'pending',
    };
  }
}

// ===== Plan Executor =====

export interface PlanExecutorConfig {
  provider: Provider;
  model?: string;
  maxRetries?: number;
}

export class PlanExecutor {
  private config: PlanExecutorConfig;

  constructor(config: PlanExecutorConfig) {
    this.config = config;
  }

  async execute(plan: Plan): Promise<Plan> {
    const completed = new Set<string>();
    const failed = new Set<string>();
    const updated = [...plan.subTasks];

    while (completed.size + failed.size < updated.length) {
      // Find ready tasks (all dependencies completed)
      const ready = updated.filter(t =>
        t.status === 'pending' &&
        t.dependsOn.every(dep => completed.has(dep))
      );

      if (ready.length === 0) {
        // Check if any pending tasks have failed dependencies
        const stuck = updated.filter(t =>
          t.status === 'pending' &&
          t.dependsOn.some(dep => failed.has(dep))
        );
        for (const task of stuck) {
          task.status = 'failed';
          task.result = 'Dependency failed';
          failed.add(task.id);
        }
        if (stuck.length === 0) break; // No more progress possible
        continue;
      }

      // Execute ready tasks in parallel
      await Promise.all(ready.map(async (task) => {
        task.status = 'running';
        try {
          const context = task.dependsOn
            .map(dep => updated.find(t => t.id === dep)?.result)
            .filter(Boolean)
            .join('\n');

          const prompt = context
            ? `Previous results:\n${context}\n\nExecute: ${task.description}`
            : `Execute: ${task.description}`;

          const resp = await this.config.provider.complete({
            messages: [{ role: 'user', content: prompt }],
            model: this.config.model,
            temperature: 0,
          });

          task.result = resp.content;
          task.status = 'completed';
          completed.add(task.id);
        } catch (err) {
          task.result = (err as Error).message;
          task.status = 'failed';
          failed.add(task.id);
        }
      }));
    }

    return { ...plan, subTasks: updated };
  }
}
