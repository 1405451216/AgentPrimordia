/**
 * worldmodel.test.ts — 世界模型 TS 端测试（矩阵 #1 对等）。
 *
 * 两层验证：
 *  1. 跨语言夹具对账门：Go 为权威生成方（agentprimordia/internal/agent/
 *     worldmodel/testdata/worldmodel_fixtures.json），本文件逐项断言 TS 实现
 *     与 Go 确定性语义一致（NodeID 位级 / ComparePaths 判定 / Rehearse 文案 /
 *     Snapshot JSON 形态可直接互换）；
 *  2. 本端单元语义：事件流增量维护 / 覆盖式修订 / 裁剪落图 / 快照校验。
 */
import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  WorldModelTracker,
  comparePaths,
  rehearse,
  toolCallNodeId,
  nodeId,
  KindToolCall,
  KindObservation,
  EdgeCause,
  type PlanStep,
  type TrackerSnapshot,
} from '../worldmodel/tracker.js';

const __dirname = dirname(fileURLToPath(import.meta.url));
const FIXTURES_PATH = resolve(
  __dirname,
  '../../../../../agentprimordia/internal/agent/worldmodel/testdata/worldmodel_fixtures.json',
);

interface WmFixtures {
  nodeID: { kind: string; summary: string; id: string }[];
  comparePaths: { planned: string[]; actual: string[]; diff: { DivergedAt: string; PlannedButSkipped: string[] | null; ExecutedButUnplanned: string[] | null } }[];
  rehearse: { plan: { Goal: string; Steps: PlanStep[] | null }; graph_nodes: unknown[]; pass: boolean; missing: string[] }[];
  snapshot: TrackerSnapshot;
}

const FX = JSON.parse(readFileSync(FIXTURES_PATH, 'utf-8')) as WmFixtures;

// ===== 1. 跨语言夹具对账 =====

describe('世界模型跨语言夹具对账（Go 权威）', () => {
  it('NodeID：FNV-1a64 派生与 Go 逐位一致（含空白规范化/中文/Kind 区分）', () => {
    for (const f of FX.nodeID) {
      expect(nodeId(f.kind as never, f.summary)).toBe(f.id);
    }
    // 空白规范化等价对（同一 ID）
    expect(FX.nodeID[0].id).toBe(FX.nodeID[1].id);
    // Kind 区分
    expect(FX.nodeID[0].id).not.toBe(FX.nodeID[3].id);
  });

  it('comparePaths：差异判定与 Go 逐字段一致（一致/乱序/计划外/跳步）', () => {
    for (const f of FX.comparePaths) {
      expect(comparePaths(f.planned, f.actual)).toEqual(f.diff);
    }
  });

  it('rehearse：缺陷文案（中文）与 Go 逐字一致（通过/缺失形态）', () => {
    for (const f of FX.rehearse) {
      const tr = new WorldModelTracker();
      tr.restore({
        nodes: f.graph_nodes as never,
        plan: { Goal: '', Steps: null },
        has_plan: false,
      });
      const report = rehearse(f.plan, tr.getGraph());
      expect(report.Pass).toBe(f.pass);
      expect(report.MissingPreconditions).toEqual(f.missing ?? []);
    }
  });

  it('snapshot：Go 产出的快照 JSON 可被 TS 直接 restore，且再导出逐键等价', () => {
    const tr = new WorldModelTracker();
    expect(() => tr.restore(JSON.parse(JSON.stringify(FX.snapshot)))).not.toThrow();
    const roundTrip = JSON.parse(JSON.stringify(tr.snapshot()));
    expect(roundTrip).toEqual(FX.snapshot);
    // 恢复后继续增量可用：重复观察去重不重复建节点
    tr.apply({ type: 'tool_observed', turn: 3, toolName: 'read', toolInput: 'a', observation: '内容A' });
    const before = tr.getGraph().nodes().length;
    tr.apply({ type: 'tool_observed', turn: 4, toolName: 'read', toolInput: 'a', observation: '内容A' });
    expect(tr.getGraph().nodes().length).toBe(before);
  });
});

// ===== 2. 本端单元语义 =====

describe('WorldModelTracker 事件流', () => {
  it('计划修订 → 调用收敛 → 轨迹随修订重置', () => {
    const tr = new WorldModelTracker();
    const stepA: PlanStep = { ID: toolCallNodeId('read', 'a.txt'), Summary: 'read a.txt', DependsOn: null };
    tr.apply({ type: 'plan_revised', turn: 1, task: '任务', goal: 'g', steps: [stepA] });
    expect(tr.getPlanTrajectory()).toBeNull();

    tr.apply({ type: 'tool_observed', turn: 1, toolName: 'read', toolInput: 'a.txt', observation: '内容' });
    const traj = tr.getPlanTrajectory();
    expect(traj).toEqual([stepA.ID]);

    // 覆盖式修订重置轨迹
    tr.apply({ type: 'plan_revised', turn: 2, task: '', goal: 'g2', steps: [] });
    expect(tr.getPlanTrajectory()).toBeNull();
    const plan = tr.currentPlan();
    expect(plan?.Steps).toHaveLength(0);
  });

  it('因果边与假设/上下文分型', () => {
    const tr = new WorldModelTracker();
    tr.apply({ type: 'plan_revised', turn: 1, task: '任务T', goal: 'g', steps: [] });
    tr.apply({ type: 'tool_observed', turn: 1, toolName: 't', toolInput: 'x', observation: 'o' });
    tr.apply({ type: 'hypothesis_formed', turn: 1, text: '推理文本' });
    tr.trimNotification([{ role: 'user', content: '被裁历史', turn: -1 }], 2);

    const callNode = tr.getGraph().node(toolCallNodeId('t', 'x'))!;
    expect(callNode.Edges).toHaveLength(1);
    expect(callNode.Edges![0].Kind).toBe(EdgeCause);
    const obsNode = tr.getGraph().node(callNode.Edges![0].To)!;
    expect(obsNode.Kind).toBe(KindObservation);
    expect(obsNode.Summary).toBe('o');

    // 假设节点与被裁事实节点均在图上（分型正确）
    const kinds = tr.getGraph().nodes().map((n) => n.Kind);
    expect(kinds).toContain('hypothesis');
    expect(tr.getGraph().nodes().some((n) => n.Kind === KindObservation && n.Summary === '被裁历史')).toBe(true);
  });

  it('comparePaths 基本形态（TS 本端独立于夹具的健全性断言）', () => {
    expect(comparePaths(['a', 'b'], ['a', 'b'])).toEqual({
      DivergedAt: '',
      PlannedButSkipped: null,
      ExecutedButUnplanned: null,
    });
    expect(comparePaths(['a', 'b', 'c'], ['a', 'x']).DivergedAt).toBe('b');
  });

  it('restore 校验：重复节点 ID / 悬挂边 / 悬挂轨迹引用抛错且不改动现有状态', () => {
    const tr = new WorldModelTracker();
    tr.apply({ type: 'tool_observed', turn: 1, toolName: 't', toolInput: 'x', observation: 'o' });
    const before = tr.getGraph().nodes().length;

    const dup: TrackerSnapshot = {
      nodes: [
        { ID: 'a', Kind: KindToolCall, Summary: 'x', CreatedAtTurn: 1, Edges: null },
        { ID: 'a', Kind: KindToolCall, Summary: 'y', CreatedAtTurn: 1, Edges: null },
      ],
      plan: { Goal: '', Steps: null },
      has_plan: false,
    };
    expect(() => tr.restore(dup)).toThrow(/重复节点 ID/);

    const dangling: TrackerSnapshot = {
      nodes: [{ ID: 'a', Kind: KindObservation, Summary: 'o', CreatedAtTurn: 1, Edges: [{ To: 'ghost', Kind: EdgeCause }] }],
      plan: { Goal: '', Steps: null },
      has_plan: false,
      plan_traj: ['a'],
    };
    expect(() => tr.restore(dangling)).toThrow(/不存在/);

    // 失败的 restore 不改动现有状态
    expect(tr.getGraph().nodes().length).toBe(before);
  });
});
