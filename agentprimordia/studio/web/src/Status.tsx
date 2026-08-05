/**
 * Studio 共享状态组件
 *
 * 统一错误面板 / 成功横幅 / 陈旧提示，供各页面复用。
 */
import type { ReactNode } from 'react';

interface ErrorPanelProps {
  message: string;
  onRetry?: () => void;
}

/** 内联错误面板：展示错误信息并提供重试入口 */
export function ErrorPanel({ message, onRetry }: ErrorPanelProps) {
  return (
    <div className="error-panel" role="alert">
      <p className="error-msg">{message}</p>
      {onRetry && (
        <button className="btn-secondary" onClick={onRetry}>
          重试
        </button>
      )}
    </div>
  );
}

interface SuccessBannerProps {
  children: ReactNode;
  onDismiss?: () => void;
}

/** 成功横幅：操作成功后展示，可手动关闭 */
export function SuccessBanner({ children, onDismiss }: SuccessBannerProps) {
  return (
    <div className="success-banner" role="status">
      <span>{children}</span>
      {onDismiss && (
        <button className="banner-close" onClick={onDismiss} aria-label="关闭">
          ✕
        </button>
      )}
    </div>
  );
}

interface StalenessProps {
  lastUpdatedAt: number | null;
  staleAfterMs?: number;
}

/** 陈旧提示：显示数据上次刷新时间，超过阈值标记为可能过期 */
export function Staleness({ lastUpdatedAt, staleAfterMs = 30000 }: StalenessProps) {
  if (lastUpdatedAt === null) return null;
  const seconds = Math.max(0, Math.round((Date.now() - lastUpdatedAt) / 1000));
  const stale = seconds * 1000 > staleAfterMs;
  return (
    <span
      className={`staleness${stale ? ' stale' : ''}`}
      title="数据最后刷新时间"
    >
      {stale ? `数据可能已过期（${seconds} 秒前）` : `上次刷新 ${seconds} 秒前`}
    </span>
  );
}
