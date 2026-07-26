/**
 * Phase 3.4: Studio Web UI — Marketplace
 *
 * 模板浏览/搜索/评分/一键部署
 */
import { useState, useEffect } from 'react';

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

  const fetchTemplates = async () => {
    try {
      const params = new URLSearchParams();
      if (query) params.set('q', query);
      if (category) params.set('category', category);
      const res = await fetch(`/api/v1/marketplace/templates?${params}`);
      if (res.ok) setTemplates(await res.json());
    } catch { /* 忽略 */ }
  };

  useEffect(() => { fetchTemplates(); }, [category]);

  const deploy = async (id: string) => {
    setDeploying(id);
    try {
      await fetch(`/api/v1/marketplace/deploy`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ template_id: id }),
      });
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
        <button onClick={fetchTemplates}>搜索</button>
      </section>

      {/* 模板网格 */}
      <section className="template-grid">
        {templates.length === 0 ? (
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
                  onClick={() => deploy(tmpl.id)}
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
