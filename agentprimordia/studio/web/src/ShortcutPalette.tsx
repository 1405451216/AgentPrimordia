/**
 * 快捷键面板与全局快捷键
 *
 * 在 App 壳层注册全局键盘监听：
 *  - Shift+/     打开/关闭快捷键面板
 *  - /           聚焦搜索框（若当前页存在）
 *  - g 然后 1-5  导航到对应页面（概览/混沌/集群/学习/市场）
 */
import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';

const SHORTCUTS = [
  { keys: 'Shift + /', desc: '打开/关闭快捷键面板' },
  { keys: '/', desc: '聚焦当前页的搜索框' },
  { keys: 'g 1-5', desc: '跳转页面：1 概览 · 2 混沌 · 3 集群 · 4 学习 · 5 市场' },
  { keys: 'Esc', desc: '关闭对话框或面板' },
];

const ROUTES = ['/', '/chaos', '/cluster', '/learning', '/marketplace'];

export function ShortcutPalette() {
  const [open, setOpen] = useState(false);
  const navigate = useNavigate();
  // g 键待组合状态
  const [pendingG, setPendingG] = useState(false);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      // 输入框中不劫持 / 键
      const tag = (document.activeElement?.tagName ?? '').toLowerCase();
      const inField = tag === 'input' || tag === 'textarea' || tag === 'select';

      if (e.shiftKey && e.key === '/') {
        e.preventDefault();
        setOpen((v) => !v);
        return;
      }
      if (e.key === 'Escape') {
        setOpen(false);
        setPendingG(false);
        return;
      }
      // g + 数字导航
      if (!inField) {
        if (e.key.toLowerCase() === 'g' && !e.metaKey && !e.ctrlKey && !e.altKey) {
          setPendingG(true);
          return;
        }
        if (pendingG && /^[1-5]$/.test(e.key)) {
          e.preventDefault();
          navigate(ROUTES[Number(e.key) - 1]);
          setPendingG(false);
          return;
        }
      }
      // 单独 / 聚焦搜索（仅在有搜索框的页面）
      if (e.key === '/' && !e.shiftKey && !e.metaKey && !e.ctrlKey && !e.altKey && !inField) {
        const search = document.querySelector<HTMLInputElement>(
          'input[type="text"], input:not([type])',
        );
        // 只聚焦 placeholder 含「搜索」的输入框
        if (search && search.placeholder?.includes('搜索')) {
          e.preventDefault();
          search.focus();
        }
      }
      // 非 g 键打断组合状态
      if (pendingG && e.key !== 'g') {
        setPendingG(false);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [pendingG, navigate]);

  if (!open) return null;
  return (
    <div className="modal-overlay" onClick={() => { setOpen(false); setPendingG(false); }}>
      <div
        className="modal shortcut-palette"
        role="dialog"
        aria-modal="true"
        aria-labelledby="shortcut-title"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 id="shortcut-title">键盘快捷键</h3>
        <table className="shortcut-table">
          <tbody>
            {SHORTCUTS.map((s) => (
              <tr key={s.keys}>
                <td><kbd>{s.keys}</kbd></td>
                <td>{s.desc}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <div className="confirm-actions">
          <button className="btn-secondary" onClick={() => { setOpen(false); setPendingG(false); }}>
            关闭
          </button>
        </div>
      </div>
    </div>
  );
}
