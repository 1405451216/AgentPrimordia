// versioned-registry.ts — Phase 2 T2-2 版本化 Prompt Registry
// 版本化的 prompt 仓库：支持多版本并存、按 tag 查询、回滚到任意历史版本。
// 不依赖外部存储，纯内存实现；可通过 export/import JSON 持久化。
//
// 与 prompt/registry.ts（TemplateRegistry）的区别：
//   - TemplateRegistry：管理 PromptTemplate 渲染规则
//   - versioned-registry.ts：管理 prompt 内容的多版本历史与 A/B 推广

export interface PromptVersion {
  /** 版本号（单调递增整数，1-based） */
  version: number;
  /** 提示词内容 */
  content: string;
  /** 创建时间（ISO 8601） */
  createdAt: string;
  /** 作者 */
  author?: string;
  /** 标签（用于查询） */
  tags?: string[];
  /** 关联的实验 ID（可选，来自 prompt-ab-test） */
  experimentId?: string;
  /** 元数据（自定义） */
  metadata?: Record<string, unknown>;
  /** 弃用标记 */
  deprecated?: boolean;
}

export interface PromptEntry {
  /** 提示词名称（唯一标识） */
  name: string;
  /** 描述 */
  description?: string;
  /** 版本列表（按 version 升序） */
  versions: PromptVersion[];
  /** 当前活跃版本号 */
  activeVersion: number;
}

export interface PromptRegistryOptions {
  /** 最大版本历史数（超出自动淘汰最旧的非弃用版本），0 = 无限制 */
  maxVersions?: number;
  /** 时间提供器（便于测试） */
  now?: () => Date;
}

export class VersionedPromptRegistry {
  private entries = new Map<string, PromptEntry>();
  private nextVersions = new Map<string, number>();
  private options: Required<PromptRegistryOptions>;

  constructor(options: PromptRegistryOptions = {}) {
    this.options = {
      maxVersions: options.maxVersions ?? 100,
      now: options.now ?? (() => new Date()),
    };
  }

  /**
   * 注册一个全新的 prompt（v1）。
   * 如果 name 已存在，抛出错误；改用 addVersion(name, content) 增加新版本。
   */
  register(
    name: string,
    content: string,
    meta?: Omit<PromptVersion, 'version' | 'content' | 'createdAt'>,
  ): PromptVersion {
    if (this.entries.has(name)) {
      throw new Error(`VersionedPromptRegistry.register: "${name}" already exists. Use addVersion().`);
    }
    const v: PromptVersion = {
      version: 1,
      content,
      createdAt: this.options.now().toISOString(),
      ...meta,
    };
    this.entries.set(name, {
      name,
      versions: [v],
      activeVersion: 1,
    });
    this.nextVersions.set(name, 2);
    return v;
  }

  /**
   * 向已存在的 prompt 添加新版本。
   */
  addVersion(
    name: string,
    content: string,
    meta?: Omit<PromptVersion, 'version' | 'content' | 'createdAt'>,
  ): PromptVersion {
    const entry = this.entries.get(name);
    if (!entry) {
      throw new Error(`VersionedPromptRegistry.addVersion: "${name}" not registered.`);
    }
    const nextVer = this.nextVersions.get(name) ?? entry.versions.length + 1;
    const v: PromptVersion = {
      version: nextVer,
      content,
      createdAt: this.options.now().toISOString(),
      ...meta,
    };
    entry.versions.push(v);
    this.nextVersions.set(name, nextVer + 1);

    // 自动淘汰最旧的非弃用版本
    if (this.options.maxVersions > 0) {
      const active = entry.versions.filter((v) => !v.deprecated);
      while (active.length > this.options.maxVersions) {
        const oldest = active.shift()!;
        const idx = entry.versions.indexOf(oldest);
        if (idx >= 0) entry.versions.splice(idx, 1);
      }
    }

    // 新版本自动成为活跃版本（除非显式指定 deprecated）
    if (!meta?.deprecated) {
      entry.activeVersion = v.version;
    }
    return v;
  }

  /** 获取当前活跃版本 */
  getActive(name: string): PromptVersion | undefined {
    const entry = this.entries.get(name);
    if (!entry) return undefined;
    return entry.versions.find((v) => v.version === entry.activeVersion);
  }

  /** 获取指定版本 */
  getVersion(name: string, version: number): PromptVersion | undefined {
    const entry = this.entries.get(name);
    if (!entry) return undefined;
    return entry.versions.find((v) => v.version === version);
  }

  /** 列出所有版本（默认按 version 升序） */
  listVersions(name: string): PromptVersion[] {
    const entry = this.entries.get(name);
    if (!entry) return [];
    return [...entry.versions].sort((a, b) => a.version - b.version);
  }

  /**
   * 切换活跃版本。
   */
  activate(name: string, targetVersion: number): PromptVersion {
    const entry = this.entries.get(name);
    if (!entry) throw new Error(`activate: "${name}" not found`);
    const v = entry.versions.find((vv) => vv.version === targetVersion);
    if (!v) throw new Error(`activate: version ${targetVersion} not found for "${name}"`);
    if (v.deprecated) {
      throw new Error(`activate: cannot activate deprecated version ${targetVersion} of "${name}"`);
    }
    entry.activeVersion = targetVersion;
    return v;
  }

  /**
   * 回滚到上一个版本（active - 1）。
   * 如果当前已是 v1，则抛错。
   */
  rollback(name: string): PromptVersion {
    const entry = this.entries.get(name);
    if (!entry) throw new Error(`rollback: "${name}" not found`);
    if (entry.activeVersion <= 1) {
      throw new Error(`rollback: "${name}" is already at v1`);
    }
    const target = entry.activeVersion - 1;
    return this.activate(name, target);
  }

  /**
   * 按 tag 查询版本（返回第一个匹配的版本）。
   */
  findByTag(name: string, tag: string): PromptVersion | undefined {
    const entry = this.entries.get(name);
    if (!entry) return undefined;
    return entry.versions.find((v) => v.tags?.includes(tag));
  }

  /** 列出所有已注册的 prompt 名称 */
  listNames(): string[] {
    return Array.from(this.entries.keys()).sort();
  }

  /** 删除某个 prompt（含其所有版本） */
  delete(name: string): boolean {
    const had = this.entries.delete(name);
    this.nextVersions.delete(name);
    return had;
  }

  /** 列出 entry 的元信息 */
  describe(name: string): PromptEntry | undefined {
    const entry = this.entries.get(name);
    if (!entry) return undefined;
    return {
      ...entry,
      versions: entry.versions.map((v) => ({ ...v })),
    };
  }

  /**
   * 导出全部为 JSON（用于持久化/快照）。
   */
  toJSON(): string {
    return JSON.stringify({
      entries: Array.from(this.entries.values()).map((e) => ({
        ...e,
        versions: e.versions.map((v) => ({ ...v })),
      })),
      nextVersions: Array.from(this.nextVersions.entries()),
    }, null, 2);
  }

  /**
   * 从 JSON 恢复（合并模式：保留现有未冲突的 entry）。
   * 返回新增的 entry 数。
   */
  static fromJSON(json: string, options?: PromptRegistryOptions): VersionedPromptRegistry {
    const data = JSON.parse(json) as {
      entries: PromptEntry[];
      nextVersions: Array<[string, number]>;
    };
    const reg = new VersionedPromptRegistry(options);
    for (const e of data.entries) {
      reg.entries.set(e.name, {
        name: e.name,
        description: e.description,
        activeVersion: e.activeVersion,
        versions: e.versions.map((v) => ({ ...v })),
      });
    }
    for (const [name, ver] of data.nextVersions) {
      reg.nextVersions.set(name, ver);
    }
    return reg;
  }
}