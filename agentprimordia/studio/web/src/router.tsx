/**
 * AgentPrimordia Studio — 路由树（入口与测试共享）
 *
 *   /                  Overview（系统概览）
 *   /chaos             Chaos Lab（混沌实验）
 *   /cluster           Cluster Dashboard（集群拓扑）
 *   /learning          Learning Monitor（知识蒸馏监控）
 *   /marketplace       Agent Marketplace（模板市场）
 *   /*                  NotFound（未知路径）
 */
import { Route, Routes } from 'react-router-dom';

import App from './App';
import { Overview } from './pages/Overview';
import { ChaosLab } from './pages/ChaosLab';
import { ClusterDashboard } from './pages/ClusterDashboard';
import { LearningMonitor } from './pages/LearningMonitor';
import { MarketplacePage } from './pages/MarketplacePage';
import { NotFound } from './pages/NotFound';

export default function StudioApp() {
  return (
    <Routes>
      <Route path="/" element={<App />}>
        <Route index element={<Overview />} />
        <Route path="chaos" element={<ChaosLab />} />
        <Route path="cluster" element={<ClusterDashboard />} />
        <Route path="learning" element={<LearningMonitor />} />
        <Route path="marketplace" element={<MarketplacePage />} />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  );
}
