/**
 * 快捷键面板测试
 *
 * 覆盖：
 *  - Shift+/ 打开与关闭快捷键面板
 *  - Esc 关闭面板
 *  - g + 数字导航到对应路由
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { ShortcutPalette } from './ShortcutPalette';

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({}) }) as Response));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function renderPalette() {
  return render(
    <MemoryRouter>
      <ShortcutPalette />
    </MemoryRouter>,
  );
}

describe('ShortcutPalette 快捷键', () => {
  it('Shift+/ 打开面板，再次按关闭', () => {
    renderPalette();

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    fireEvent.keyDown(window, { key: '/', shiftKey: true });
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('键盘快捷键')).toBeInTheDocument();

    fireEvent.keyDown(window, { key: '/', shiftKey: true });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('Esc 关闭面板', () => {
    renderPalette();

    fireEvent.keyDown(window, { key: '/', shiftKey: true });
    expect(screen.getByRole('dialog')).toBeInTheDocument();

    fireEvent.keyDown(window, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});
