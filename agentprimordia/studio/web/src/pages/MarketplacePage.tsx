/**
 * Phase 3.4: Studio Web UI — Marketplace
 *
 * 模板浏览/搜索/评分/一键部署
 *
 * 加固点：
 *  - deploy 检查 res.ok，失败展示错误横幅
 *  - 部署成功展示成功横幅
 *  - 搜索输入 300ms 防抖（清空即重置），AbortController 防乱序覆盖
 *  - 加载/空/错误 状态区分（骨架屏）
 */
import { useState, useEffect, useRef } from 'react';
import { ErrorPanel, SuccessBanner } from '../Status';

interface AgentTemplate {
  id: string;
  name: string;
  description: string;
  version: string;
  author: string;
  category: string;
  tags: string[];
  rating: number;
  downloads: number;
}

export function MarketplacePage() {
  const [templates, setTemplates] = useState<AgentTemplate[]>([]);
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState('');
  const [deploying, setDeploying] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [notice, setNotice] = useState<{ kind: 'success' | 'error'; text: string } | null>(null);
  const debounceRef = useRef<number | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const fetchTemplates = async (overrides?: { q?: string; cat?: string }) => {
    const q = overrides?.q ?? query;
    const cat = overrides?.cat ?? category;
    // 中止上一次未完成的请求，避免乱序响应覆盖新结果
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setLoading(true);
    setFetchError(null);
    try {
      const params = new URLSearchParams();
      if (q) params.set('q', q);
      if (cat) params.set('category', cat);
      const res = await fetch(`/api/v1/marketplace/templates?${params}`, { signal: controller.signal });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setTemplates(await res.json());
    } catch (e) {
      if ((e as Error)?.name === 'AbortError') return; // 被新请求取代，静默
      setFetchError(e instanceof Error ? e.message : '未知错误');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchTemplates({ q: '', cat: category }); }, [category]);

  // 搜索输入防抖：停止输入 300ms 后再请求；清空输入=重置为全量列表
  useEffect(() => {
    if (debounceRef.current) window.clearTimeout(debounceRef.current);
    debounceRef.current = window.setTimeout(() => {
      fetchTemplates({ q: query, cat: category });
    }, 300);
    return () => {
      if (debounceRef.current) window.clearTimeout(debounceRef.current);
      abortRef.current?.abort();
    };
  }, [query]);

  const deploy = async (id: string, name: string) => {
    setDeploying(id);
    setNotice(null);
    try {
      const res = await fetch(`/api/v1/marketplace/deploy`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ template_id: id }),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setNotice({ kind: 'success', text: `模板「${name}」部署成功` });
    } catch (e) {
      setNotice({ kind: 'error', text: `部署失败：${e instanceof Error ? e.message : '未知错误'}` });
    } finally {
      setDeploying(null);
    }
  };

  const categories = ['research', 'coding', 'analysis', 'chat', 'automation'];

  return (
    <div className="panel marketplace">
      <h2>Agent Marketplace</h2>

      {/* 搜索栏 */}
      <section className="search-bar">
        <input
          placeholder="搜索模板..."
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && fetchTemplates()}
        />
        <select value={category} onChange={(e) => setCategory(e.target.value)}>
          <option value="">全部分类</option>
          {categories.map((c) => <option key={c} value={c}>{c}</option>)}
        </select>
        <button onClick={() => fetchTemplates()}>搜索</button>
      </section>

      {/* 操作反馈横幅 */}
      {notice && (
        notice.kind === 'success' ? (
          <SuccessBanner onDismiss={() => setNotice(null)}>
            {notice.text}
          </SuccessBanner>
        ) : (
          <ErrorPanel message={notice.text} onDismiss={() => setNotice(null)} />
        )
      )}

      {/* 模板网格 */}
      <section className="template-grid">
        {fetchError ? (
          <ErrorPanel message={`加载失败：${fetchError}`} onRetry={() => fetchTemplates()} />
        ) : loading ? (
          <div className="skeleton-list" aria-busy="true">
            <div className="skeleton-row" />
            <div className="skeleton-row" />
            <div className="skeleton-row" />
          </div>
        ) : templates.length === 0 ? (
          <p className="empty">未找到模板</p>
        ) : (
          templates.map((tmpl) => (
            <div key={tmpl.id} className="template-card">
              <h3>{tmpl.name}</h3>
              <p className="desc">{tmpl.description}</p>
              <div className="meta">
                <span className="category">{tmpl.category}</span>
                <span className="rating">⭐ {tmpl.rating.toFixed(1)}</span>
                <span className="downloads">⬇ {tmpl.downloads}</span>
              </div>
              <div className="tags">
                {tmpl.tags?.map((tag) => <span key={tag} className="tag">{tag}</span>)}
              </div>
              <footer>
                <span className="author">by {tmpl.author}</span>
                <button
                  onClick={() => deploy(tmpl.id, tmpl.name)}
                  disabled={deploying === tmpl.id}
                >
                  {deploying === tmpl.id ? '部署中...' : '一键部署'}
                </button>
              </footer>
            </div>
          ))
        )}
      </section>
    </div>
  );
}
