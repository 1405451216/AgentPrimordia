/**
 * AgentPrimordia Studio — 布局壳（侧边导航 + 内容区）
 */
import { NavLink, Outlet } from 'react-router-dom';

const NAV_ITEMS = [
  { to: '/', label: 'Chaos Lab', icon: '🧪', end: true },
  { to: '/cluster', label: 'Cluster', icon: '🕸️', end: false },
  { to: '/learning', label: 'Learning', icon: '🧠', end: false },
  { to: '/marketplace', label: 'Marketplace', icon: '🏪', end: false },
];

export default function App() {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-logo">⚡</span>
          <span className="brand-name">AgentPrimordia</span>
        </div>
        <nav className="nav">
          {NAV_ITEMS.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) => `nav-item${isActive ? ' active' : ''}`}
            >
              <span className="nav-icon">{item.icon}</span>
              <span>{item.label}</span>
            </NavLink>
          ))}
        </nav>
        <footer className="sidebar-footer">Studio v3.2.0</footer>
      </aside>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
