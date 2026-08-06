/**
 * 状态枚举中文化映射
 *
 * 后端返回英文枚举值，展示前统一映射为中文，
 * 避免在中英文混杂的界面里出现裸英文状态词。
 */
export type ExperimentStatus = 'pending' | 'running' | 'completed' | 'aborted' | 'failed';
export type NodeStatus = 'online' | 'offline' | 'leaving';
export type NodeRole = 'leader' | 'follower' | 'candidate';

const EXPERIMENT_STATUS_LABELS: Record<ExperimentStatus, string> = {
  pending: '待处理',
  running: '运行中',
  completed: '已完成',
  aborted: '已中止',
  failed: '已失败',
};

const NODE_STATUS_LABELS: Record<NodeStatus, string> = {
  online: '在线',
  offline: '离线',
  leaving: '离开中',
};

const NODE_ROLE_LABELS: Record<NodeRole, string> = {
  leader: '领导者',
  follower: '跟随者',
  candidate: '候选者',
};

/** 实验状态 → 中文（未知值回退原文） */
export function experimentStatusLabel(status: string): string {
  return EXPERIMENT_STATUS_LABELS[status as ExperimentStatus] ?? status;
}

/** 节点状态 → 中文（未知值回退原文） */
export function nodeStatusLabel(status: string): string {
  return NODE_STATUS_LABELS[status as NodeStatus] ?? status;
}

/** 节点角色 → 中文（未知值回退原文） */
export function nodeRoleLabel(role: string): string {
  return NODE_ROLE_LABELS[role as NodeRole] ?? role;
}

/** 节点状态字形符号（非纯色编码） */
export function nodeStatusGlyph(status: string): string {
  switch (status) {
    case 'online': return '●';
    case 'offline': return '○';
    case 'leaving': return '◐';
    default: return '？';
  }
}

/** 实验状态字形符号（非纯色编码） */
export function experimentStatusGlyph(status: string): string {
  switch (status) {
    case 'completed': return '✓';
    case 'failed': return '✕';
    case 'aborted': return '◐';
    case 'running': return '◉';
    case 'pending': return '○';
    default: return '？';
  }
}
