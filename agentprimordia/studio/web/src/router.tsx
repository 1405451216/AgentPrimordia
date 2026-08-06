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
import { HelpPage } from './pages/HelpPage';
import { NotFound } from './pages/NotFound';
import { AutonomyMonitor } from './pages/AutonomyMonitor';
import { SkillLibrary } from './pages/SkillLibrary';
import { A2AInterop } from './pages/A2AInterop';
import { RealtimeConsole } from './pages/RealtimeConsole';

export default function StudioApp() {
  return (
    <Routes>
      <Route path="/" element={<App />}>
        <Route index element={<Overview />} />
        <Route path="chaos" element={<ChaosLab />} />
        <Route path="cluster" element={<ClusterDashboard />} />
        <Route path="learning" element={<LearningMonitor />} />
        <Route path="marketplace" element={<MarketplacePage />} />
        <Route path="help" element={<HelpPage />} />
        <Route path="autonomy" element={<AutonomyMonitor />} />
        <Route path="skills" element={<SkillLibrary />} />
        <Route path="a2a-interop" element={<A2AInterop />} />
        <Route path="realtime" element={<RealtimeConsole />} />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  );
}
