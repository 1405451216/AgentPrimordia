// Episode 记忆条目
export interface Episode {
  id: string;
  sessionId: string;
  role: string;
  content: string;
  summary?: string;
  topics?: string;
  importance?: number;
  metadata?: Record<string, string>;
  createdAt: string; // RFC3339
}

// SearchOptions 搜索选项
export interface SearchOptions {
  query: string;
  sessionId?: string;
  limit?: number;
  offset?: number;
  roleFilter?: string;
  tags?: string[];
  minScore?: number;
  maxResults?: number;
  useSemantic?: boolean;
  semanticWeight?: number;
}

// SearchResult 搜索结果
export interface SearchResult {
  episode: Episode;
  keywordScore: number;
  semanticScore: number;
  combinedScore: number;
}

// ListOptions 列表选项
export interface ListOptions {
  sessionId?: string;
  limit?: number;
  offset?: number;
  orderBy?: string;
  ascending?: boolean;
}

// MemoryStats 记忆统计
export interface MemoryStats {
  totalEpisodes: number;
  totalSessions: number;
  oldestEpisode?: string;
  newestEpisode?: string;
  avgEpisodesPerSession: number;
  sizeBytes: number;
}

// MemoryTimelineGroup 时间线分组
export interface MemoryTimelineGroup {
  date: string;
  episodes: Episode[];
  count: number;
  summary?: string;
}

// MemoryReader 只读接口
export interface MemoryReader {
  get(id: string): Promise<Episode | null>;
  getBatch(ids: string[]): Promise<Map<string, Episode>>;
  search(query: string, opts?: SearchOptions): Promise<Episode[]>;
  list(opts?: ListOptions): Promise<Episode[]>;
  count(sessionId?: string): Promise<number>;
  stats(): Promise<MemoryStats>;
}

// MemoryWriter 写入接口
export interface MemoryWriter {
  add(episode: Episode): Promise<void>;
  addBatch(episodes: Episode[]): Promise<void>;
  delete(id: string): Promise<void>;
  deleteBatch(ids: string[]): Promise<void>;
  updateSummary(id: string, summary: string, topics: string): Promise<void>;
  setImportance(episodeId: string, importance: number): Promise<void>;
}

// MemorySearcher 高级搜索接口
export interface MemorySearcher {
  searchAdvanced(opts: SearchOptions): Promise<SearchResult[]>;
  searchByTag(tag: string, opts?: SearchOptions): Promise<Episode[]>;
  getImportant(threshold: number, limit: number): Promise<Episode[]>;
  getTimeline(days: number): Promise<MemoryTimelineGroup[]>;
}

// MemoryLifecycle 生命周期接口
export interface MemoryLifecycle {
  close(): Promise<void>;
  cleanupExpired(maxAgeDays: number): Promise<number>;
  clearAll(sessionId?: string): Promise<void>;
}

// MemoryExporter 导入导出接口
export interface MemoryExporter {
  exportMemories(sessionId?: string, format?: string): Promise<Uint8Array>;
  importMemories(data: Uint8Array, format?: string): Promise<number>;
}

// MemoryQuery 辅助查询接口
export interface MemoryQuery {
  getMemoriesByTag(tag: string, limit?: number): Promise<Episode[]>;
  getMemoriesBySession(sessionId: string): Promise<Episode[]>;
  getImportantMemories(threshold: number, limit?: number): Promise<Episode[]>;
  getMemoryTimeline(days: number): Promise<MemoryTimelineGroup[]>;
}

// Memory 组合接口
export interface Memory
  extends MemoryReader,
    MemoryWriter,
    MemorySearcher,
    MemoryLifecycle,
    MemoryExporter,
    MemoryQuery {}

// SummaryExtractor 摘要提取接口
export interface SummaryExtractor {
  extractSummary(content: string): Promise<{ summary: string; topics: string }>;
}

// 默认搜索限制
const DEFAULT_SEARCH_LIMIT = 10;

// tokenizeRe 与 Go 版本 sqlite.go 中的正则保持一致
// 匹配非单词字符作为分隔符
const TOKENIZE_RE = /[^\p{L}\p{N}]+/u;

/**
 * 将文本拆分为 lowercased token 数组
 * 行为与 Go 版本 indexTokens 一致
 */
function indexTokens(...fields: (string | undefined)[]): string[] {
  const combined = fields.filter((f) => f && f.length > 0).join(" ");
  if (!combined) return [];
  return combined
    .toLowerCase()
    .split(TOKENIZE_RE)
    .filter((t) => t.length > 0);
}

/**
 * 将查询拆分为唯一的 lowercased token 数组
 * 行为与 Go 版本 uniqueTokens 一致
 */
function uniqueTokens(query: string): string[] {
  const parts = query.toLowerCase().split(TOKENIZE_RE);
  const seen = new Set<string>();
  const out: string[] = [];
  for (const p of parts) {
    if (p.length === 0) continue;
    if (seen.has(p)) continue;
    seen.add(p);
    out.push(p);
  }
  return out;
}

/**
 * 倒排索引的 AND 交集
 * 返回所有 token 都出现过的 episode ID 列表
 * 行为与 Go 版本 intersectPostings 一致
 */
function intersectPostings(
  idx: Map<string, Set<string>>,
  tokens: string[]
): string[] {
  if (tokens.length === 0) return [];

  // 取最小 postings 集作为起点（性能优化）
  let minPostings: Set<string> | null = null;
  for (const tok of tokens) {
    const postings = idx.get(tok);
    if (!postings) {
      // 任一 token 不存在 → 交集为空
      return [];
    }
    if (!minPostings || postings.size < minPostings.size) {
      minPostings = postings;
    }
  }
  if (!minPostings) return [];

  const out: string[] = [];
  for (const id of minPostings) {
    // 校验其他 token 也包含此 ID
    let hit = true;
    for (const tok of tokens) {
      const postings = idx.get(tok);
      if (!postings || !postings.has(id)) {
        hit = false;
        break;
      }
    }
    if (hit) {
      out.push(id);
    }
  }
  return out;
}

/**
 * 校验 Episode 必填字段
 * 行为与 Go 版本 Episode.Validate() 一致
 */
function validateEpisode(episode: Episode): void {
  if (!episode.id) {
    throw new Error("episode ID cannot be empty");
  }
  if (!episode.sessionId) {
    throw new Error("session ID cannot be empty");
  }
  if (!episode.role) {
    throw new Error("role cannot be empty");
  }
  if (!episode.content) {
    throw new Error("content cannot be empty");
  }
  if (
    episode.importance !== undefined &&
    (episode.importance < 0 || episode.importance > 1)
  ) {
    throw new Error("importance must be between 0 and 1");
  }
}

// 自增 ID 计数器
let episodeIdCounter = 0;

/**
 * 生成唯一的 episode ID
 * 行为与 Go 版本 generateEpisodeID 一致
 */
function generateEpisodeId(): string {
  episodeIdCounter++;
  let s = episodeIdCounter.toString();
  // 左侧填充 0 至 13 位
  while (s.length < 13) {
    s = "0" + s;
  }
  return "ep_" + s;
}

/**
 * InMemoryStore 内存版记忆存储实现
 *
 * 使用 Map<string, Episode> 存储条目，
 * 倒排索引 ftsIndex（token → episode ID 集合）实现全文搜索。
 * 行为与 Go 版本 InMemoryStore 一致。
 */
export class InMemoryStore implements Memory {
  // 条目存储
  private episodes = new Map<string, Episode>();
  // 倒排索引：lowercased token → episode ID 集合
  private ftsIndex = new Map<string, Set<string>>();

  /**
   *