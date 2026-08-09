import { describe, it, expect } from 'vitest';
import {
  SkillAcquisition, SkillStore, createSkill,
  type SkillDistiller, type Trajectory, type SkillGuardrail,
} from '../../src/skills/index.js';
import {
  AutonomyRuntime, GoalState, createPlan,
  type CheckpointStore, type Checkpoint, type PlanStep,
} from '../../src/autonomy/index.js';

// ===== v4.7-1 TS 技能习得流水线（对齐 Go skills.Acquisition） =====

class MockDistiller implements SkillDistiller {
  async distill(t: Trajectory): Promise<ReturnType<typeof createSkill>> {
    const skill = createSkill('数据修复', '从轨迹提炼', t.records.map((r, i) => ({
      id: `s${i + 1}`,
      description: `${r.toolName} 操作`,
      toolName: r.toolName,
      dependsOn: i > 0 ? [`s${i}`] : undefined,
    })));
    skill.description = '从轨迹提炼';
    return skill;
  }
}

class SanitizingGuardrail implements SkillGuardrail {
  async sanitizeSkillDescription(d: string): Promise<string> {
    return `${d}（已脱敏）`;
  }
}

function trajectory(): Trajectory {
  return {
    taskDescription: '数据修复',
    success: true,
    timestamp: new Date(),
    records: [
      { toolName: 'query_anomaly', success: true, durationMs: 100 },
      { toolName: 'fix_data', success: true, durationMs: 200 },
    ],
  };
}

describe('SkillAcquisition (v4.7-1)', () => {
  it('习得：提炼 → 护栏 → 校验 → draft 入库', async () => {
    const acq = new SkillAcquisition(new MockDistiller());
    acq.setGuardrail(new SanitizingGuardrail());
    const skill = await acq.acquire(trajectory());
    expect(skill.description).toBe('从轨迹提炼（已脱敏）');
    expect(skill.status).toBe('draft');

    const store = new SkillStore();
    store.save(skill);
    expect(store.count).toBe(1);
  });

  it('轨迹记录计数', () => {
    const acq = new SkillAcquisition(new MockDistiller());
    acq.recordTrajectory(trajectory());
    expect(acq.trajectoryCount).toBe(1);
  });

  it('护栏报错 → 习得失败', async () => {
    const acq = new SkillAcquisition(new MockDistiller());
    acq.setGuardrail({ sanitizeSkillDescription: async () => { throw new Error('护栏拒绝'); } });
    await expect(acq.acquire(trajectory())).rejects.toThrow('护栏拒绝');
  });
});

// ===== v4.7-1 TS 自治运行时（对齐 Go AutonomyRuntime） =====

class SeqExecutor {
  async executeStep(step: PlanStep): Promise<string> {
    return `ok:${step.id}`;
  }
}

class MemCheckpointStore implements CheckpointStore {
  private items = new Map<string, Checkpoint>();
  async saveCheckpoint(cp: Checkpoint): Promise<void> { this.items.set(cp.goalId, cp); }
  async loadCheckpoint(goalId: string): Promise<Checkpoint | undefined> { return this.items.get(goalId); }
  async listIncomplete(): Promise<Checkpoint[]> {
    return [...this.items.values()].filter(c => !c.completed);
  }
}

describe('AutonomyRuntime (v4.7-1)', () => {
  it('目标生命周期：submit → plan → execute → complete', async () => {
    const rt = new AutonomyRuntime({ stepExecutor: new SeqExecutor() });
    const goal = rt.submitGoal('监控数据并修复');
    const plan = createPlan(goal.id, [
      { id: 'collect', description: '采集', strategy: 'sequential' },
      { id: 'fix', description: '修复', dependsOn: ['collect'], strategy: 'sequential' },
      { id: 'verify', description: '验证', dependsOn: ['fix'], strategy: 'sequential' },
    ]);
    rt.setPlan(goal.id, plan);
    await rt.executeGoal(goal.id);
    rt.completeGoal(goal.id);
    expect(rt.getGoal(goal.id)!.state).toBe(GoalState.Done);
  });

  it('崩溃恢复：kill 后新运行时续跑（从断点继续）', async () => {
    const store = new MemCheckpointStore();

    // 节点 A：执行 collect 后落 checkpoint（模拟崩溃）
    const rtA = new AutonomyRuntime({ stepExecutor: new SeqExecutor(), checkpointStore: store });
    const goal = rtA.submitGoal('跨节点续跑目标');
    const plan = createPlan(goal.id, [
      { id: 'collect', description: '采集', strategy: 'sequential' },
      { id: 'fix', description: '修复', dependsOn: ['collect'], strategy: 'sequential' },
      { id: 'verify', description: '验证', dependsOn: ['fix'], strategy: 'sequential' },
    ]);
    rtA.setPlan(goal.id, plan);
    // 手动完成 collect
    for (const s of plan.steps) {
      if (s.id !== 'collect') break;
      s.status = 'completed';
    }
    await store.saveCheckpoint({
      goalId: goal.id,
      goalDescription: goal.description,
      state: GoalState.Executing,
      lastCompletedStep: 'collect',
      plan,
      completed: false,
    });

    // 节点 B：自动接管续跑至完成
    const rtB = new AutonomyRuntime({ stepExecutor: new SeqExecutor(), checkpointStore: store });
    const resumed = await rtB.resumeIncomplete();
    expect(resumed).toEqual([goal.id]);
    await rtB.executeGoal(goal.id);
    rtB.completeGoal(goal.id);
    expect(rtB.getGoal(goal.id)!.state).toBe(GoalState.Done);
  });
});
