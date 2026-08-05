/**
 * Primordium — Overview 主视觉
 *
 * 回应批判思考问题 Q1：把品牌「原初涌现」母题（dot + orbit）
 * 升级为活体可视化。环上的节点即在线集群节点（分片比例着色），
 * 中心的"脉搏"为能力平均分——让首页第一眼传达
 * "从混沌中涌现的智能体集群"。
 */
interface PrimordiumNode {
  id: string;
  status: string;
  shards?: number;
}

interface PrimordiumProps {
  nodes: PrimordiumNode[];
  /** 0-1 能力脉搏（能力平均分） */
  pulse?: number;
  size?: number;
}

export function Primordium({ nodes, pulse = 0.5, size = 220 }: PrimordiumProps) {
  const online = nodes.filter((n) => n.status === 'online');
  const count = Math.max(online.length, 1);
  const cx = size / 2;
  const cy = size / 2;
  const r = size / 2 - 14;
  const ringR = r * 0.92;
  // 分片比例段宽
  const totalOnlineShards = Math.max(
    online.reduce((s, n) => s + (n.shards ?? 0), 0),
    1,
  );
  const segDeg = 360 / count;
  // 脉搏半径（能力平均分 → 内圈大小）
  const pulseR = Math.max(6, Math.min(16, 8 + pulse * 10));

  return (
    <svg
      className="primordium"
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
      role="img"
      aria-label={`集群原初体：${online.length} 个在线节点，能力脉搏 ${Math.round(pulse * 100)}%`}
    >
      <defs>
        <radialGradient id="primordium-glow" cx="50%" cy="50%" r="50%">
          <stop offset="0%" stopColor="#4f8cff" stopOpacity="0.25" />
          <stop offset="100%" stopColor="#4f8cff" stopOpacity="0" />
        </radialGradient>
      </defs>

      {/* 光晕 */}
      <circle cx={cx} cy={cy} r={r} fill="url(#primordium-glow)" />

      {/* 轨道 */}
      <circle cx={cx} cy={cy} r={ringR} fill="none" stroke="#2a3550" strokeWidth="1" />

      {/* 节点段（分片比例决定弧长） */}
      {online.map((node, i) => {
        const total = totalOnlineShards;
        const frac = Math.max((node.shards ?? 1) / total, 0.08);
        const arc = segDeg * frac;
        const angle = i * segDeg - 90;
        const rad = (angle * Math.PI) / 180;
        const x = cx + ringR * Math.cos(rad);
        const y = cy + ringR * Math.sin(rad);
        const len = Math.max(4, arc * 1.2);
        return (
          <g key={node.id}>
            <circle
              cx={x}
              cy={y}
              r={6}
              fill={`var(--shard-${i % 6})`}
              stroke="#10162a"
              strokeWidth="2"
            />
            <path
              d={`M ${cx} ${cy} L ${x + 14 * Math.cos(rad)} ${y + 14 * Math.sin(rad)}`}
              stroke="#2a3550"
              strokeWidth="0.75"
              strokeDasharray={len > 0 ? `${len} ${8}` : undefined}
            />
          </g>
        );
      })}

      {/* 中心脉搏：能力平均分（CSS 动画，受 reduced-motion 控制） */}
      <g className="primordium-pulse">
        <circle cx={cx} cy={cy} r={pulseR} fill="#4f8cff" fillOpacity="0.85" className="pulse-core" />
        <circle cx={cx} cy={cy} r={pulseR + 6} fill="none" stroke="#4f8cff" strokeOpacity="0.4" strokeWidth="1" className="pulse-ring" />
      </g>
    </svg>
  );
}
