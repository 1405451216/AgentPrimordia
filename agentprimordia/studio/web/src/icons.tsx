/**
 * Studio SVG 图标集
 *
 * 以 1.5px 圆角描边、24×24 视口的线性图标替代 emoji，
 * 统一跨平台渲染并建立产品视觉身份。
 */
interface IconProps {
  size?: number;
  className?: string;
}

function base(size = 16, className?: string) {
  return {
    width: size,
    height: size,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 1.5,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
    className,
    'aria-hidden': true,
  };
}

/** 概览：仪表盘 */
export function IconOverview({ size, className }: IconProps) {
  return (
    <svg {...base(size, className)}>
      <rect x="3" y="3" width="7" height="7" rx="1.5" />
      <rect x="14" y="3" width="7" height="7" rx="1.5" />
      <rect x="3" y="14" width="7" height="7" rx="1.5" />
      <rect x="14" y="14" width="7" height="7" rx="1.5" />
    </svg>
  );
}

/** 混沌实验：辐射/爆裂 */
export function IconChaos({ size, className }: IconProps) {
  return (
    <svg {...base(size, className)}>
      <path d="M12 3v4" />
      <path d="M12 17v4" />
      <path d="M3 12h4" />
      <path d="M17 12h4" />
      <path d="M5.6 5.6l2.8 2.8" />
      <path d="M15.6 15.6l2.8 2.8" />
      <path d="M18.4 5.6l-2.8 2.8" />
      <path d="M8.4 15.6l-2.8 2.8" />
      <circle cx="12" cy="12" r="2.5" />
    </svg>
  );
}

/** 集群：节点与连线 */
export function IconCluster({ size, className }: IconProps) {
  return (
    <svg {...base(size, className)}>
      <circle cx="5" cy="12" r="2.5" />
      <circle cx="19" cy="5" r="2.5" />
      <circle cx="19" cy="19" r="2.5" />
      <path d="M7.3 10.8l9.4-4.5" />
      <path d="M7.3 13.2l9.4 4.5" />
    </svg>
  );
}

/** 学习：大脑 / 神经突触 */
export function IconLearning({ size, className }: IconProps) {
  return (
    <svg {...base(size, className)}>
      <path d="M12 4a3.5 3.5 0 0 0-3.5 3.5A3.5 3.5 0 0 0 5 11a3.5 3.5 0 0 0 0 7h14a3.5 3.5 0 0 0 0-7 3.5 3.5 0 0 0-3.5-3.5A3.5 3.5 0 0 0 12 4z" />
      <path d="M12 8v8" />
      <path d="M9 11l6 2" />
      <path d="M15 11l-6 2" />
    </svg>
  );
}

/** 市场：商店 */
export function IconMarket({ size, className }: IconProps) {
  return (
    <svg {...base(size, className)}>
      <path d="M4 9h16l-1.5 10a2 2 0 0 1-2 1.8h-9a2 2 0 0 1-2-1.8L4 9z" />
      <path d="M8 9V7a4 4 0 0 1 8 0v2" />
    </svg>
  );
}

/** 品牌标识：原初/涌现 */
export function IconBrand({ size, className }: IconProps) {
  return (
    <svg {...base(size, className)}>
      <path d="M12 3a7 7 0 0 1 0 14" />
      <path d="M12 17a5 5 0 0 1 0 4" />
      <circle cx="12" cy="10" r="2" />
    </svg>
  );
}

/** 领导者星标 */
export function IconLeader({ size, className }: IconProps) {
  return (
    <svg {...base(size, className)}>
      <path d="M12 3l2.2 5.4 5.8.6-4.3 3.9 1.2 5.7L12 15.9 7.1 18.6l1.2-5.7L4 9l5.8-.6L12 3z" />
    </svg>
  );
}
