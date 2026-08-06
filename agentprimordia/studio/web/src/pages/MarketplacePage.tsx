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
import { PageTitle } from '../PageTitle';
import { IconStar, IconDownload } from '../icons';
import { useConfirmDialog } from '../useConfirmDialog';

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

interface Deployment {
  id: string;
  templateId: string;
  name: string;
  version: string;
  category: string;
  status: 'running' | 'stopped';
  deployedAt: string;
}

export function MarketplacePage() {
  // query/category 从 URL 初始化（?q=&category=），支持跨刷新保留筛选
  const urlParams = typeof window !== 'undefined' ? new URLSearchParams(window.location.search) : null;
  const [templates, setTemplates] = useState<AgentTemplate[]>([]);
  const [query, setQuery] = useState(urlParams?.get('q') ?? '');
  const [category, setCategory] = useState(urlParams?.get('category') ?? '');
  const [deploying, setDeploying] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [notice, setNotice] = useState<{ kind: 'success' | 'error'; text: string } | null>(null);
  const [deployedTemplate, setDeployedTemplate] = useState<AgentTemplate | null>(null);
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [deployError, setDeployError] = useState<string | null>(null);
  const [stoppingId, setStoppingId] = useState<string | null>(null);
  // 待确认停止的部署：非空时弹出确认对话框
  const [confirmingStop, setConfirmingStop] = useState<Deployment | null>(null);
  // 待确认部署的模板：非空时弹出确认对话框
  const [confirming, setConfirming] = useState<AgentTemplate | null>(null);
  const debounceRef = useRef<number | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const fetchDeployments = async () => {
    setDeployError(null);
    try {
      const res = await fetch('/api/v1/marketplace/deployments');
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setDeployments(await res.json());
    } catch (e) {
      setDeployError(e instanceof Error ? e.message : '未知错误');
    }
  };

  useEffect(() => { fetchDeployments(); }, []);

  const fetchTemplates = async (overrides?: { q?: string; cat?: string; keepData?: boolean }) => {
    const q = overrides?.q ?? query;
    const cat = overrides?.cat ?? category;
    // 中止上一次未完成的请求，避免乱序响应覆盖新结果
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    // 已有数据时保留旧数据，避免每次按键/搜索都闪烁成骨架屏
    setLoading(templates.length === 0 || overrides?.keepData !== true);
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

  // 搜索/分类变化统一走防抖：query 与 category 同步生效，避免列表与筛选矛盾
  useEffect(() => {
    if (debounceRef.current) window.clearTimeout(debounceRef.current);
    debounceRef.current = window.setTimeout(() => {
      fetchTemplates({ q: query, cat: category, keepData: true });
    }, 300);
    return () => {
      if (debounceRef.current) window.clearTimeout(debounceRef.current);
      abortRef.current?.abort();
    };
  }, [query, category]);

  // 筛选变化时写入 URL（不触发导航）
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (query) params.set('q', query); else params.delete('q');
    if (category) params.set('category', category); else params.delete('category');
    const qs = params.toString();
    window.history.replaceState(null, '', qs ? `?${qs}` : window.location.pathname);
  }, [query, category]);

  // 部署确认对话框：统一焦点管理（初始聚焦、Esc、Tab 陷阱）
  const deployDialog = useConfirmDialog({
    open: confirming !== null,
    busy: deploying !== null,
    focusTarget: 'button.btn-primary',
    onClose: () => setConfirming(null),
  });

  // 停止部署确认对话框：统一焦点管理
  const stopDialog = useConfirmDialog({
    open: confirmingStop !== null,
    busy: stoppingId !== null,
    onClose: () => setConfirmingStop(null),
  });

  const deploy = async (tmpl: AgentTemplate) => {
    setDeploying(tmpl.id);
    setNotice(null);
    setDeployedTemplate(null);
    try {
      const res = await fetch(`/api/v1/marketplace/deploy`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ template_id: tmpl.id }),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setDeployedTemplate(tmpl);
      setConfirming(null);
      await fetchDeployments();
    } catch (e) {
      setNotice({ kind: 'error', text: `部署失败：${e instanceof Error ? e.message : '未知错误'}` });
    } finally {
      setDeploying(null);
    }
  };

  // 执行停止（卸载）已部署的 Agent
  const stopDeployment = async (dep: Deployment) => {
    setStoppingId(dep.id);
    try {
      const res = await fetch(`/api/v1/marketplace/deployments/${dep.id}/stop`, {
        method: 'POST',
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setNotice({ kind: 'success', text: `「${dep.name}」已停止` });
      setConfirmingStop(null);
      await fetchDeployments();
    } catch (e) {
      setNotice({ kind: 'error', text: `停止失败：${e instanceof Error ? e.message : '未知错误'}` });
    } finally {
      setStoppingId(null);
    }
  };

  // 重启（恢复）已停止的 Agent
  const startDeployment = async (dep: Deployment) => {
    setStoppingId(dep.id);
    try {
      const res = await fetch(`/api/v1/marketplace/deployments/${dep.id}/start`, {
        method: 'POST',
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      setNotice({ kind: 'success', text: `「${dep.name}」已启动` });
      await fetchDeployments();
    } catch (e) {
      setNotice({ kind: 'error', text: `启动失败：${e instanceof Error ? e.message : '未知错误'}` });
    } finally {
      setStoppingId(null);
    }
  };

  // 分类下拉：内置默认分类 + 从模板数据中出现的分类合并，避免硬编码漂移
  const DEFAULT_CATEGORIES = ['research', 'coding', 'analysis', 'chat', 'automation'];
  const categories = Array.from(new Set([...DEFAULT_CATEGORIES, ...templates.map((t) => t.category)])).filter(Boolean);
  // 分类显示名本地化（避免裸英文破坏中文界面）
  const CATEGORY_LABELS: Record<string, string> = {
    research: '研究',
    coding: '编码',
    analysis: '分析',
    chat: '对话',
    automation: '自动化',
  };
  const categoryLabel = (c: string) => CATEGORY_LABELS[c] ?? c;

  return (
    <div className="panel marketplace">
      <PageTitle title="Agent 市场" subtitle="Agent Marketplace" />

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
          {categories.map((c) => <option key={c} value={c}>{categoryLabel(c)}</option>)}
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

      {/* 部署状态反馈 */}
      {deployedTemplate && (
        <div className="deploy-status" role="status">
          <div className="deploy-status-icon" aria-hidden="true">✓</div>
          <div className="deploy-status-body">
            <p className="deploy-status-title">「{deployedTemplate.name}」已部署到集群</p>
            <p className="deploy-status-meta">
              模板 {deployedTemplate.version} · 分类 {categoryLabel(deployedTemplate.category)} · 可在集群中查看运行状态
            </p>
          </div>
          <button className="banner-close" onClick={() => setDeployedTemplate(null)} aria-label="关闭">✕</button>
        </div>
      )}

      {/* 部署历史与治理 */}
      {(deployments.length > 0 || deployError) && (
        <section className="deployments-panel">
          <h3>已部署 Agent{deployments.length > 0 ? `（${deployments.length}）` : ''}</h3>
          {deployError && (
            <ErrorPanel message={`加载部署失败：${deployError}`} onRetry={fetchDeployments} />
          )}
          <table>
            <thead>
              <tr>
                <th>名称</th>
                <th>版本</th>
                <th>分类</th>
                <th>状态</th>
                <th>部署时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {deployments.map((dep) => (
                <tr key={dep.id}>
                  <td>{dep.name}</td>
                  <td>{dep.version}</td>
                  <td>{categoryLabel(dep.category)}</td>
                  <td>
                    <span className={`status-badge status-${dep.status}`} aria-label={dep.status === 'running' ? '运行中' : '已停止'}>
                      <span aria-hidden="true">{dep.status === 'running' ? '●' : '○'}</span>
                      {dep.status === 'running' ? '运行中' : '已停止'}
                    </span>
                  </td>
                  <td>{dep.deployedAt ? new Date(dep.deployedAt).toLocaleString() : '-'}</td>
                  <td>
                    {dep.status === 'running' ? (
                      <button
                        className="btn-danger btn-sm"
                        onClick={() => setConfirmingStop(dep)}
                        disabled={stoppingId === dep.id}
                      >
                        {stoppingId === dep.id ? '停止中...' : '停止'}
                      </button>
                    ) : (
                      <button
                        className="btn-secondary btn-sm"
                        onClick={() => startDeployment(dep)}
                        disabled={stoppingId === dep.id}
                      >
                        {stoppingId === dep.id ? '启动中...' : '启动'}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
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
          <div className="empty-state">
            <p className="empty">未找到模板</p>
            {(query || category) && (
              <button
                className="btn-secondary btn-sm empty-reset"
                onClick={() => { setQuery(''); setCategory(''); }}
              >
                清空筛选
              </button>
            )}
          </div>
        ) : (
          templates.map((tmpl) => (
            <div key={tmpl.id} className="template-card">
              <h3>{tmpl.name}</h3>
              <p className="desc">{tmpl.description}</p>
              <div className="meta">
                <span className="category">{categoryLabel(tmpl.category)}</span>
                <span className="rating"><IconStar size={12} /> {tmpl.rating.toFixed(1)}</span>
                <span className="downloads"><IconDownload size={12} /> {tmpl.downloads}</span>
              </div>
              <div className="tags">
                {tmpl.tags?.map((tag) => <span key={tag} className="tag">{tag}</span>)}
              </div>
              <footer>
                <span className="author">作者 {tmpl.author}</span>
                <button
                  onClick={() => setConfirming(tmpl)}
                  disabled={deploying === tmpl.id}
                >
                  {deploying === tmpl.id ? '部署中...' : '一键部署'}
                </button>
              </footer>
            </div>
          ))
        )}
      </section>

      {/* 部署确认对话框 */}
      {confirming && (
        <div className="modal-overlay" onClick={() => !deploying && deployDialog.closeAndRestore()}>
          <div
            ref={deployDialog.dialogRef}
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="deploy-confirm-title"
            aria-describedby="deploy-confirm-desc"
            onClick={(e) => e.stopPropagation()}
          >
            <h3 id="deploy-confirm-title">确认部署模板</h3>
            <p id="deploy-confirm-desc" className="confirm-desc">
              将把模板部署到当前集群，创建对应的 Agent 实例。
            </p>
            <dl className="confirm-detail">
              <dt>模板</dt><dd>{confirming.name} v{confirming.version}</dd>
              <dt>作者</dt><dd>{confirming.author}</dd>
              <dt>分类</dt><dd>{categoryLabel(confirming.category)}</dd>
            </dl>
            <div className="confirm-actions">
              <button className="btn-secondary" onClick={deployDialog.closeAndRestore} disabled={deploying !== null}>取消</button>
              <button
                className="btn-primary"
                onClick={() => deploy(confirming)}
                disabled={deploying !== null}
              >
                {deploying !== null ? '部署中...' : '确认部署'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 停止部署确认对话框 */}
      {confirmingStop && (
        <div className="modal-overlay" onClick={() => !stoppingId && stopDialog.closeAndRestore()}>
          <div
            ref={stopDialog.dialogRef}
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="stop-confirm-title"
            aria-describedby="stop-confirm-desc"
            onClick={(e) => e.stopPropagation()}
          >
            <h3 id="stop-confirm-title">确认停止 Agent</h3>
            <p id="stop-confirm-desc" className="confirm-desc">
              停止后该 Agent 将不再处理请求，可在列表中随时重新启动。
            </p>
            <dl className="confirm-detail">
              <dt>Agent</dt><dd>{confirmingStop.name} v{confirmingStop.version}</dd>
              <dt>分类</dt><dd>{categoryLabel(confirmingStop.category)}</dd>
              <dt>部署时间</dt><dd>{confirmingStop.deployedAt ? new Date(confirmingStop.deployedAt).toLocaleString() : '-'}</dd>
            </dl>
            <div className="confirm-actions">
              <button className="btn-secondary" onClick={stopDialog.closeAndRestore} disabled={stoppingId !== null}>取消</button>
              <button
                className="btn-danger"
                onClick={() => stopDeployment(confirmingStop)}
                disabled={stoppingId !== null}
              >
                {stoppingId !== null ? '停止中...' : '确认停止'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
