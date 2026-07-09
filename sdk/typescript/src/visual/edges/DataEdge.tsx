/**
 * DataEdge — 数据流边（SVG 路径，T2-1 可视化构建器）。
 *
 * 在 AgentDesigner 的 <svg> 覆盖层内渲染，使用贝塞尔曲线连接源/目标端口。
 */

import type { ReactElement } from 'react';

export interface DataEdgeProps {
  /** 起点 x（源节点右侧端口） */
  x1: number;
  /** 起点 y */
  y1: number;
  /** 终点 x（目标节点左侧端口） */
  x2: number;
  /** 终点 y */
  y2: number;
}

export function DataEdge({ x1, y1, x2, y2 }: DataEdgeProps): ReactElement {
  const midX = (x1 + x2) / 2;
  const d = `M ${x1} ${y1} C ${midX} ${y1}, ${midX} ${y2}, ${x2} ${y2}`;
  return (
    <path
      d={d}
      className="ap-edge"
      fill="none"
      stroke="#94a3b8"
      strokeWidth={2}
      markerEnd="url(#ap-arrow)"
    />
  );
}
