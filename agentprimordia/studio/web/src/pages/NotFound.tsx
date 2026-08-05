/**
 * NotFound 页面
 *
 * 未知路径展示明确提示，并提供返回首页入口。
 */
import { Link } from 'react-router-dom';

export function NotFound() {
  return (
    <div className="panel not-found">
      <h2>页面不存在</h2>
      <p className="not-found-text">你访问的地址不存在，可能已被移动或删除。</p>
      <Link className="btn-secondary not-found-link" to="/">
        返回概览
      </Link>
    </div>
  );
}
