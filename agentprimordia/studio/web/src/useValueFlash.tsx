/**
 * 数值增量提示 hook
 *
 * 记录上一次渲染的数值，当值变化时短暂高亮，
 * 让轮询页面（Cluster / Learning）的数字变化一眼可见。
 */
import { useRef, useState, useEffect } from 'react';

/**
 * useValueFlash — 值变化时返回 true 触发 CSS 高亮。
 * 高亮持续 flashMs 毫秒后自动熄灭。
 */
export function useValueFlash(value: number | string, flashMs = 1200): boolean {
  const prevRef = useRef<number | string>(value);
  const [flashing, setFlashing] = useState(false);

  useEffect(() => {
    if (prevRef.current !== value) {
      prevRef.current = value;
      setFlashing(true);
      const t = window.setTimeout(() => setFlashing(false), flashMs);
      return () => window.clearTimeout(t);
    }
  }, [value, flashMs]);

  return flashing;
}

/** 渲染统计卡片值：变化时加高亮类 */
export function FlashValue({ value, className = 'value' }: { value: number | string; className?: string }) {
  const flashing = useValueFlash(value);
  return <span className={`${className}${flashing ? ' flash' : ''}`}>{value}</span>;
}
