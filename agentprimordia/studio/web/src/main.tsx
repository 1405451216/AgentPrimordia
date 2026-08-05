/**
 * AgentPrimordia Studio — 应用入口
 *
 * 挂载 React 根节点并注册路由（路由树见 ./router.tsx）。
 */
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter, useLocation } from 'react-router-dom';
import { useEffect, useState } from 'react';

import StudioApp from './router';
import { ErrorBoundary } from './ErrorBoundary';
import './styles.css';

/** 每次路由变化重置错误边界，避免导航后仍停留在降级界面 */
function ResetOnRouteChange({ children }: { children: React.ReactNode }) {
  const location = useLocation();
  // 初始化 key 从当前路径，避免首屏额外重挂载
  const [key, setKey] = useState(location.pathname);
  useEffect(() => {
    setKey((k) => (k === location.pathname ? k : location.pathname));
  }, [location.pathname]);
  return <ErrorBoundary key={key}>{children}</ErrorBoundary>;
}

const root = createRoot(document.getElementById('root')!);

root.render(
  <StrictMode>
    <BrowserRouter>
      <ResetOnRouteChange>
        <StudioApp />
      </ResetOnRouteChange>
    </BrowserRouter>
  </StrictMode>,
);
