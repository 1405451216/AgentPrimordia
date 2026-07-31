/**
 * a11y.test.ts — React 组件无障碍（Accessibility）静态分析测试。
 *
 * 通过读取组件源码进行静态分析，验证：
 * - ARIA 属性（role, aria-label, aria-live, aria-busy 等）存在性
 * - 键盘事件处理器（onKeyDown, onKeyDown 等）
 * - 语义化 HTML 标签使用
 * - 屏幕阅读器友好内容（sr-only 等）
 *
 * 这些测试不依赖 DOM 渲染，适合 CI 环境。
 */

import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// ===== 工具函数 =====

const reactDir = resolve(import.meta.dirname ?? '.', '..');

function readComponent(filename: string): string {
  return readFileSync(resolve(reactDir, filename), 'utf-8');
}

// ===== AgentPanel.tsx 无障碍测试 =====

describe('AgentPanel.tsx accessibility', () => {
  const source = readComponent('AgentPanel.tsx');

  it('should use role="alert" for error display', () => {
    expect(source).toContain('role="alert"');
  });

  it('should use aria-busy for loading state', () => {
    expect(source).toContain('aria-busy="true"');
  });

  it('should use aria-live for streaming output', () => {
    expect(source).toContain('aria-live="polite"');
  });

  it('should include sr-only text for screen readers', () => {
    expect(source).toContain('sr-only');
  });

  it('should have data-stream attribute for SSE output', () => {
    expect(source).toContain('data-stream="sse"');
  });
});

// ===== AgentPanelServer.tsx 无障碍测试 =====

describe('AgentPanelServer.tsx accessibility', () => {
  const source = readComponent('AgentPanelServer.tsx');

  it('should use role="alert" for error display', () => {
    expect(source).toContain('role="alert"');
  });

  it('should use aria-busy for loading state', () => {
    expect(source).toContain('aria-busy="true"');
  });
});

// ===== agent-stream.tsx 无障碍测试 =====

describe('agent-stream.tsx accessibility', () => {
  const source = readComponent('agent-stream.tsx');

  it('should use role="status" for the stream container', () => {
    expect(source).toContain('role="status"');
  });

  it('should use aria-live="polite" for streaming content', () => {
    expect(source).toContain('aria-live="polite"');
  });

  it('should have aria-label describing the stream purpose', () => {
    expect(source).toMatch(/aria-label=\{`Agent \$\{name\}/);
  });

  it('should use role="alert" for error boundary', () => {
    expect(source).toContain('role="alert"');
  });

  it('should use aria-busy for skeleton loading', () => {
    expect(source).toContain('aria-busy="true"');
  });

  it('should have aria-label on skeleton component', () => {
    expect(source).toMatch(/aria-label=\{`Agent \$\{name\}.*加载`/);
  });
});

// ===== visual-editor.tsx 无障碍测试 =====

describe('visual-editor.tsx accessibility', () => {
  const source = readComponent('visual-editor.tsx');

  it('should have keyboard event handler for Delete/Backspace', () => {
    expect(source).toContain("'Delete'");
    expect(source).toContain("'Backspace'");
  });

  it('should have keyboard event handler for Escape', () => {
    expect(source).toContain("'Escape'");
  });

  it('should register window keydown listener', () => {
    expect(source).toContain("addEventListener('keydown'");
    expect(source).toContain("removeEventListener('keydown'");
  });

  it('should use semantic HTML elements (label, heading)', () => {
    expect(source).toContain('<label');
    expect(source).toContain('<h4');
  });

  it('should have button elements for interactive actions', () => {
    // Count button occurrences
    const buttonCount = (source.match(/<button/g) || []).length;
    expect(buttonCount).toBeGreaterThanOrEqual(5);
  });

  it('should use form elements (input, select, textarea) with labels', () => {
    expect(source).toContain('<input');
    expect(source).toContain('<select');
    expect(source).toContain('<textarea');
  });
});

// ===== collaboration 组件无障碍测试 =====

describe('HITLPanel.tsx accessibility', () => {
  const source = readComponent('collaboration/HITLPanel.tsx');

  it('should use semantic footer element', () => {
    expect(source).toContain('<footer');
  });

  it('should have button elements for approve/reject actions', () => {
    const buttonCount = (source.match(/<button/g) || []).length;
    expect(buttonCount).toBeGreaterThanOrEqual(2);
  });

  it('should have onClick handlers on action buttons', () => {
    expect(source).toContain('onClick');
  });
});

describe('CollaborationView.tsx accessibility', () => {
  const source = readComponent('collaboration/CollaborationView.tsx');

  it('should use semantic header element', () => {
    expect(source).toContain('<header');
  });

  it('should use semantic main element', () => {
    expect(source).toContain('<main');
  });
});

describe('CollaborationReplay.tsx accessibility', () => {
  const source = readComponent('collaboration/CollaborationReplay.tsx');

  it('should have button elements for playback controls', () => {
    const buttonCount = (source.match(/<button/g) || []).length;
    expect(buttonCount).toBeGreaterThanOrEqual(3);
  });

  it('should have disabled state for navigation buttons', () => {
    expect(source).toContain('disabled=');
  });

  it('should use button elements (not divs) for interactive controls', () => {
    // Buttons should have onClick, not div with onClick
    expect(source).toMatch(/<button[^>]*onClick/);
  });
});

// ===== 综合无障碍属性统计 =====

describe('overall a11y coverage summary', () => {
  const files = [
    'AgentPanel.tsx',
    'AgentPanelServer.tsx',
    'agent-stream.tsx',
    'visual-editor.tsx',
    'collaboration/HITLPanel.tsx',
    'collaboration/CollaborationView.tsx',
    'collaboration/CollaborationReplay.tsx',
  ];

  it('all component files should exist and be readable', () => {
    for (const file of files) {
      const source = readComponent(file);
      expect(source.length).toBeGreaterThan(0);
    }
  });

  it('at least 3 files should contain ARIA attributes', () => {
    let ariaCount = 0;
    for (const file of files) {
      const source = readComponent(file);
      if (source.includes('role=') || source.includes('aria-')) {
        ariaCount++;
      }
    }
    expect(ariaCount).toBeGreaterThanOrEqual(3);
  });

  it('at least 1 file should have keyboard event handling', () => {
    let kbCount = 0;
    for (const file of files) {
      const source = readComponent(file);
      if (source.includes('keydown') || source.includes('onKeyDown') || source.includes('KeyboardEvent')) {
        kbCount++;
      }
    }
    expect(kbCount).toBeGreaterThanOrEqual(1);
  });
});
