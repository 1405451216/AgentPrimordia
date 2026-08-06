/**
 * 页面标题组件
 *
 * 统一页面 h2：中文主标题 + 英文副行（次行）。
 * 解决此前「中文导航 + 英文页面标题」的语言分裂。
 */
export function PageTitle({ title, subtitle }: { title: string; subtitle?: string }) {
  return (
    <div className="page-title">
      <h2 tabIndex={-1}>{title}</h2>
      {subtitle && <span className="page-title-sub">{subtitle}</span>}
    </div>
  );
}
