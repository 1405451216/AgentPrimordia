/**
 * AgentPrimordia Studio — 应用入口
 *
 * 挂载 React 根节点并注册路由（路由树见 ./router.tsx）。
 */
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';

import StudioApp from './router';
import './styles.css';

const root = createRoot(document.getElementById('root')!);

root.render(
  <StrictMode>
    <BrowserRouter>
      <StudioApp />
    </BrowserRouter>
  </StrictMode>,
);
