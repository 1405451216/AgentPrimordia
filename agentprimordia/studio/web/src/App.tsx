/**
 * AgentPrimordia Studio — 布局壳（侧边导航 + 内容区）
 *
 * 导航使用 SVG 线性图标（见 icons.tsx），以 1.5px 圆角描边
 * 建立统一视觉身份，替代 emoji。
 */
import { NavLink, Outlet } from 'react-router-dom';
import { IconOverview, IconChaos, IconCluster, IconLearning, IconMarket, IconBrand } from './icons';

const NAV_ITEMS = [
  { to: '/', label: '概览', icon: IconOverview, end: true },
  { to: '/chaos', label: '混沌实验', icon: IconChaos, end: false },
  { to: '/cluster', label: '集群', icon: IconCluster, end: false },
  { to: '/learning', label: '学习', icon: IconLearning, end: false },
  { to: '/marketplace', label: '市场', icon: IconMarket, end: false },
];

export default function App() {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-logo"><IconBrand size={20} /></span>
          <span className="brand-name">AgentPrimordia</span>
        </div>
        <nav className="nav">
          {NAV_ITEMS.map((item) => {
            const Icon = item.icon;
            return (
              <NavLink
                key={item.to}
                to={item.to}
                end={item.end}
                className={({ isActive }) => `nav-item${isActive ? ' active' : ''}`}
              >
                <span className="nav-icon"><Icon size={16} /></span>
                <span>{item.label}</span>
              </NavLink>
            );
          })}
        </nav>
        <footer className="sidebar-footer">Studio v3.2.0</footer>
      </aside>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
