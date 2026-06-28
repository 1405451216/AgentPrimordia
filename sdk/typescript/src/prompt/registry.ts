// registry.ts 实现 Prompt 模板注册表
// 管理命名模板和消息模板，与 Go 端 prompt/registry.go 对齐

import type { PromptTemplate } from '../agent/prompt-template.js';

// ===== 模板注册表 =====

/** 模板注册表，管理命名模板和消息模板，与 Go 端 Registry 对齐 */
export class TemplateRegistry {
  private templates: Map<string, PromptTemplate> = new Map();

  /** 注册命名模板 */
  register(name: string, tmpl: PromptTemplate): void {
    if (this.templates.has(name)) {
      throw new Error(`模板 "${name}" 已存在`);
    }
    this.templates.set(name, tmpl);
  }

  /** 获取命名模板（返回克隆副本，避免修改原始模板） */
  get(name: string): PromptTemplate | null {
    const tmpl = this.templates.get(name);
    if (!tmpl) return null;
    return tmpl.clone();
  }

  /** 渲染命名模板 */
  render(name: string, vars: Record<string, string>): string {
    const tmpl = this.get(name);
    if (!tmpl) {
      throw new Error(`模板 "${name}" 不存在`);
    }
    return tmpl.withVars(vars).render();
  }

  /** 删除命名模板 */
  unregister(name: string): boolean {
    return this.templates.delete(name);
  }

  /** 获取所有已注册模板名称 */
  list(): string[] {
    return Array.from(this.templates.keys());
  }

  /** 检查模板是否存在 */
  has(name: string): boolean {
    return this.templates.has(name);
  }
}