/**
 * useConfirmDialog — 统一确认对话框焦点管理
 *
 * 为所有确认/信息模态提供一致的键盘行为：
 *  - 打开时聚焦指定目标（默认第一个按钮）
 *  - Tab 陷阱（循环聚焦）
 *  - Esc 关闭（受 busy 状态门控）
 *  - 关闭后焦点恢复到触发元素
 *
 * 回应批判发现：4 个手写确认对话框存在 3 种焦点管理级别，
 * 现统一为单一 hook。
 */
import { useEffect, useRef } from 'react';

interface ConfirmDialogOptions {
  /** 是否打开 */
  open: boolean;
  /** 忙碌状态：true 时禁止 Esc 关闭 */
  busy?: boolean;
  /** 打开时聚焦的按钮选择器（默认第一个按钮） */
  focusTarget?: string;
  /** 关闭回调（供 Esc 使用） */
  onClose: () => void;
}

export function useConfirmDialog({ open, busy = false, focusTarget, onClose }: ConfirmDialogOptions) {
  const dialogRef = useRef<HTMLDivElement>(null);
  // 记住触发元素，关闭后恢复焦点
  const triggerRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;

    // 记录触发元素
    if (document.activeElement && document.activeElement !== document.body) {
      triggerRef.current = document.activeElement as HTMLElement;
    }

    // 初始聚焦
    const target = focusTarget
      ? dialogRef.current?.querySelector<HTMLElement>(focusTarget)
      : dialogRef.current?.querySelector<HTMLElement>('button');
    target?.focus();

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !busy) {
        onClose();
        return;
      }
      if (e.key !== 'Tab' || !dialogRef.current) return;
      const focusables = dialogRef.current.querySelectorAll<HTMLElement>(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
      );
      if (focusables.length === 0) return;
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };

    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      // 关闭后恢复焦点到触发元素
      if (!open && triggerRef.current) {
        triggerRef.current.focus();
      }
    };
  }, [open, busy, focusTarget, onClose]);

  /** 关闭并恢复焦点 */
  const closeAndRestore = () => {
    onClose();
  };

  return { dialogRef, closeAndRestore };
}
