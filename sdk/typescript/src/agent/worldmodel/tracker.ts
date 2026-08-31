/**
 * tracker.ts — WorldModelTracker：从 agent 事件流增量维护状态图 + 快照恢复
 * （TypeScript 端，矩阵 #1 对等；与 Go tracker.go/snapshot.go 逐语义对齐）。
 *
 * 职责边界：
 *   - 只消费最小事件集（tool_observed / plan_revised / hypothesis_formed），
 *     消息→事件的转换由接入层完成；
 *   - trimNotification 把上下文裁剪的消息转为 observation 事实节点——
 *     被裁事实不因滑动窗口而丢失；
 *   - 事件自带 Turn，按提交序增量应用、不假设时序单调；
 *   - snapshot/restore 为 state-checkpoint 协议载体（JSON 键名与 Go
 *     序列化形态一致，双线快照可直接互换）。
 */
import {
  StateGraph,
  StateNode,
  nodeId,
  KindTask,
  KindPlan,
  KindToolCall,
  KindObservation,
  KindHypothesis,
  EdgeCause,
  EdgePlan,
  EdgeContext,
  EdgeHypothesis,
} from './graph.js';
import { Plan, PlanStep } from './backdiff.js';

export {
  StateGraph,
  nodeId,
  normalizeSummary,
  KindTask,
  KindPlan,
  KindToolCall,
  KindObservation,
  KindHypothesis,
  EdgeCause,
  EdgePlan,
  EdgeContext,
  EdgeHypothesis,
} from './graph.js';
export { comparePaths, planPath } from './backdiff.js';
export type { Plan, PlanStep, BackDiff } from './backdiff.js';
export { rehearse } from './rehearsal.js';
export type { RehearsalReport } from './rehearsal.js';

// ===== 事件（封闭集合：接入层只构造事件、不能自定义事件种类）=====

export interface ToolObserved {
  type: 'tool_observed';
  turn: number;
  toolName: string;
  toolInput: string;
  observation: string;
}

export interface PlanRevised {
  type: 'plan_revised';
  turn: number;
  task: string;
  goal: string;
  steps: PlanStep[];
}

export interface HypothesisFormed {
  type: 'hypothesis_formed';
  turn: number;
  text: string;
}

export type AgentEvent = ToolObserved | PlanRevised | HypothesisFormed;

/** 一条被上下文裁剪丢弃的消息（trimNotification 输入）。 */
export interface TrimmedMessage {
  role: string;
  content: string;
  /** 产生该消息的轮次；未知传 -1（回退为裁剪发生轮次） */
  turn: number;
}

// ===== 快照（JSON 键名与 Go 序列化形态逐字一致，双线互换零转换）=====

/**
 * Go json.Marshal(Snapshot) 形态。注意两点 Go 序列化语义：
 *   - StateNode/Plan/PlanStep 无 json tag → 字段名保持 Go 导出名（ID/Kind/…）；
 *   - Snapshot.Plan 为 struct 类型，omitempty 对 struct 无效 → "plan" 键恒在
 *     （无计划时为 {"Goal":"","Steps":null}），以 has_plan 为权威。
 */
export interface TrackerSnapshot {
  nodes: StateNode[];
  plan: Plan;
  has_plan: boolean;
  last_task_id?: string;
  plan_traj?: string[];
}

/** 事件派生调用节点 ID（与 Go 接线层 NodeID(KindToolCall, "工具名 输入") 同口径）。 */
export function toolCallNodeId(toolName: string, toolInput: string): string {
  return nodeId(KindToolCall, toolName + ' ' + toolInput);
}

/**
 * WorldModelTracker 世界模型跟踪器：增量维护状态图 + 当前计划。
 * 内部状态在 TS 单线程事件循环下天然串行；对外只暴露值快照。
 */
export class WorldModelTracker {
  private graph = new StateGraph();
  private plan: Plan | null = null;
  private lastTaskID = '';
  private planTraj: string[] = [];

  /** 返回内部状态图（调用方可长期持有引用）。 */
  getGraph(): StateGraph {
    return this.graph;
  }

  /** 返回当前计划拷贝；尚未有计划时返回 null。 */
  currentPlan(): Plan | null {
    return this.plan ? clonePlan(this.plan) : null;
  }

  /** 应用单个事件（增量、幂等）；未知事件静默忽略。 */
  apply(ev: AgentEvent | null | undefined): void {
    if (!ev) {
      return;
    }
    switch (ev.type) {
      case 'tool_observed':
        this.onToolObserved(ev);
        break;
      case 'plan_revised':
        this.onPlanRevised(ev);
        break;
      case 'hypothesis_formed':
        this.onHypothesisFormed(ev);
        break;
    }
  }

  private onToolObserved(e: ToolObserved): void {
    const [callID] = this.graph.addNode(KindToolCall, e.toolName + ' ' + e.toolInput, e.turn);
    const [obsID] = this.graph.addNode(KindObservation, e.observation, e.turn);
    this.graph.addEdge(callID, obsID, EdgeCause);
    // 回溯差异轨迹端：自当前计划形成以来的实际调用序列（应用序）
    this.planTraj.push(callID);
  }

  private onPlanRevised(e: PlanRevised): void {
    const steps: PlanStep[] = (e.steps ?? []).map((s, i) => ({
      ...s,
      ID: s.ID || `step-${i + 1}`, // 缺省步骤 ID：确定性派生
    }));
    if (e.task) {
      const [taskID] = this.graph.addNode(KindTask, e.task, e.turn);
      this.lastTaskID = taskID;
    }
    const [planID] = this.graph.addNode(KindPlan, e.goal, e.turn);
    if (this.lastTaskID) {
      this.graph.addEdge(this.lastTaskID, planID, EdgePlan);
    }
    for (const s of steps) {
      const [stepID] = this.graph.addNode(KindToolCall, s.Summary, e.turn);
      this.graph.addEdge(planID, stepID, EdgePlan);
    }
    this.plan = { Goal: e.goal, Steps: steps };
    // 覆盖式修订同时重置轨迹端：回溯差异只对「本计划形成之后」的执行负责
    this.planTraj = [];
  }

  private onHypothesisFormed(e: HypothesisFormed): void {
    const [hypID] = this.graph.addNode(KindHypothesis, e.text, e.turn);
    if (this.lastTaskID) {
      this.graph.addEdge(this.lastTaskID, hypID, EdgeHypothesis);
    }
  }

  /** 上下文裁剪通知：被裁消息转为 observation 事实节点；返回本次新建节点 ID。 */
  trimNotification(msgs: TrimmedMessage[], turn: number): string[] {
    const created: string[] = [];
    for (const m of msgs) {
      const at = m.turn >= 0 ? m.turn : turn;
      const [id, isNew] = this.graph.addNode(KindObservation, m.content, at);
      if (isNew) {
        created.push(id);
      }
      if (this.lastTaskID) {
        this.graph.addEdge(this.lastTaskID, id, EdgeContext);
      }
    }
    return created;
  }

  /** 自当前计划形成以来的实际调用轨迹（应用序）；无则返回 null。 */
  getPlanTrajectory(): string[] | null {
    return this.planTraj.length > 0 ? [...this.planTraj] : null;
  }

  /** 导出世界模型全量快照（节点按 ID 升序——确定性序列化）。 */
  snapshot(): TrackerSnapshot {
    const snap: TrackerSnapshot = {
      nodes: this.graph.nodes(),
      plan: this.plan ? clonePlan(this.plan) : { Goal: '', Steps: null },
      has_plan: this.plan !== null,
    };
    if (this.lastTaskID) {
      snap.last_task_id = this.lastTaskID;
    }
    if (this.planTraj.length > 0) {
      snap.plan_traj = [...this.planTraj];
    }
    return snap;
  }

  /** 以快照覆盖式替换当前世界状态；校验失败抛错且不改动现有状态。 */
  restore(snap: TrackerSnapshot): void {
    const g = graphFromNodes(snap.nodes ?? []);
    if (snap.has_plan && snap.plan) {
      const inPlan = new Set<string>((snap.plan.Steps ?? []).map((s) => s.ID));
      for (const st of snap.plan.Steps ?? []) {
        for (const dep of st.DependsOn ?? []) {
          if (!dep || inPlan.has(dep)) {
            continue;
          }
          if (!g.node(dep)) {
            throw new Error(`计划步骤 ${st.ID} 的依赖 ${dep} 既不在计划内也不在快照图中`);
          }
        }
      }
    }
    for (const id of snap.plan_traj ?? []) {
      if (!g.node(id)) {
        throw new Error(`计划轨迹节点 ${id} 不在快照图中`);
      }
    }
    this.graph = g;
    this.plan = snap.has_plan && snap.plan ? clonePlan(snap.plan) : null;
    this.lastTaskID = snap.last_task_id ?? '';
    this.planTraj = snap.plan_traj ? [...snap.plan_traj] : [];
  }
}

/** 由节点列表重建状态图（校验语义与 Go graphFromNodes 一致）。 */
function graphFromNodes(nodes: StateNode[]): StateGraph {
  const g = new StateGraph();
  const seenIDs = new Set<string>();
  for (const n of nodes) {
    if (!n.ID) {
      throw new Error('快照含空节点 ID');
    }
    if (seenIDs.has(n.ID)) {
      throw new Error(`快照含重复节点 ID: ${n.ID}`);
    }
    seenIDs.add(n.ID);
  }
  // 快照节点 ID 是权威：直接按 ID 插入（与 Go 直接 map 写入语义一致），
  // 再重建出边与入边反向索引
  for (const n of nodes) {
    g.insertNode(n);
  }
  for (const n of nodes) {
    const seenEdges = new Set<string>();
    for (const e of n.Edges ?? []) {
      if (!g.node(e.To)) {
        throw new Error(`节点 ${n.ID} 的边指向不存在的节点 ${e.To}`);
      }
      const key = e.To + '\x00' + e.Kind;
      if (seenEdges.has(key)) {
        throw new Error(`节点 ${n.ID} 含重复边 →${e.To}(${e.Kind})`);
      }
      seenEdges.add(key);
      g.addEdge(n.ID, e.To, e.Kind);
    }
  }
  return g;
}

/** 计划防御性拷贝（Steps 与每个 DependsOn 均深拷贝）。 */
function clonePlan(p: Plan): Plan {
  const steps: PlanStep[] = (p.Steps ?? []).map((s) => ({
    ...s,
    DependsOn: s.DependsOn ? [...s.DependsOn] : null,
  }));
  return { Goal: p.Goal, Steps: steps };
}
