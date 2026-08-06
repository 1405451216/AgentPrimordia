/**
 * NotFound 页面
 *
 * 未知路径展示明确提示，并提供返回首页入口。
 */
import { Link } from 'react-router-dom';
import { PageTitle } from '../PageTitle';
import { IconBrand } from '../icons';

export function NotFound() {
  return (
    <div className="panel not-found">
      <div className="not-found-glyph" aria-hidden="true">
        <IconBrand size={40} />
      </div>
      <PageTitle title="页面不存在" subtitle="Not Found" />
      <p className="not-found-text">你访问的地址不存在，可能已被移动或删除。</p>
      <Link className="btn-secondary not-found-link" to="/">
        返回概览
      </Link>
    </div>
  );
}

