import { useId } from 'react';

/**
 * 迷你 SVG 趋势线
 *
 * 在能力卡片内渲染历史分数折线，让"进化趋势"名副其实
 * （此前只有当前分数快照，无趋势）。
 */
interface TrendPoint {
  score: number;
}

interface SparklineProps {
  data: TrendPoint[];
  width?: number;
  height?: number;
}

export function Sparkline({ data, width = 140, height = 36 }: SparklineProps) {
  const gradientId = useId();
  if (!data || data.length < 2) return null;

  const pad = 2;
  const min = Math.min(...data.map((d) => d.score));
  const max = Math.max(...data.map((d) => d.score));
  const range = max - min || 1;
  const step = (width - pad * 2) / (data.length - 1);

  const points = data.map((d, i) => {
    const x = pad + i * step;
    const y = height - pad - ((d.score - min) / range) * (height - pad * 2);
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });

  const line = points.join(' ');
  const area = `M${pad},${height - pad} L${line} L${width - pad},${height - pad} Z`;

  // 趋势方向决定颜色
  const up = data[data.length - 1].score >= data[0].score;
  const color = up ? '#27ae60' : '#e74c3c';

  return (
    <svg
      className="sparkline"
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      role="img"
      aria-label={up ? '能力趋势上升' : '能力趋势下降'}
    >
      <defs>
        <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity="0.25" />
          <stop offset="100%" stopColor={color} stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={area} fill={`url(#${gradientId})`} />
      <polyline
        points={line}
        fill="none"
        stroke={color}
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
