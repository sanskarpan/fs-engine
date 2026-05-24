import { Suspense, lazy } from 'react';
import { BrowserRouter, Routes, Route, NavLink, Navigate } from 'react-router-dom';
import Explorer from './pages/Explorer';

const DiskLayout = lazy(() => import('./pages/DiskLayout'));
const InodeInspector = lazy(() => import('./pages/InodeInspector'));
const JournalViewer = lazy(() => import('./pages/JournalViewer'));
const Terminal = lazy(() => import('./pages/Terminal'));
const Stats = lazy(() => import('./pages/Stats'));

const NAV_ITEMS = [
  { path: '/explorer', label: 'Explorer', icon: '📂' },
  { path: '/disk', label: 'Disk Layout', icon: '💾' },
  { path: '/inode', label: 'Inode Inspector', icon: '🔍' },
  { path: '/journal', label: 'Journal', icon: '📋' },
  { path: '/terminal', label: 'Terminal', icon: '💻' },
  { path: '/stats', label: 'Stats', icon: '📊' },
];

export default function App() {
  return (
    <BrowserRouter>
      <div className="flex flex-col h-screen bg-[#0d1117]">
        {/* Top navigation */}
        <header className="flex-shrink-0 border-b border-[#30363d] bg-[#161b22]">
          <div className="flex items-center px-4 h-12">
            <div className="flex items-center gap-2 mr-6">
              <div className="w-3 h-3 rounded-full bg-[#f85149]" />
              <div className="w-3 h-3 rounded-full bg-[#e3b341]" />
              <div className="w-3 h-3 rounded-full bg-[#3fb950]" />
              <span className="ml-3 text-sm font-semibold text-[#c9d1d9] font-mono">fs-engine</span>
              <span className="text-xs text-[#484f58] font-mono">v0.1.0</span>
            </div>

            <nav className="flex items-center gap-1 overflow-x-auto">
              {NAV_ITEMS.map((item) => (
                <NavLink
                  key={item.path}
                  to={item.path}
                  className={({ isActive }) =>
                    `flex items-center gap-1.5 px-3 py-1.5 rounded text-sm transition-colors whitespace-nowrap ${
                      isActive
                        ? 'bg-[#1f2937] text-[#58a6ff] border border-[#388bfd]'
                        : 'text-[#8b949e] hover:text-[#c9d1d9] hover:bg-[#21262d]'
                    }`
                  }
                >
                  <span className="text-xs">{item.icon}</span>
                  {item.label}
                </NavLink>
              ))}
            </nav>

            <div className="ml-auto flex items-center gap-2 text-xs text-[#484f58] font-mono">
              <div className="w-1.5 h-1.5 rounded-full bg-[#3fb950] animate-pulse" />
              <span>localhost:8080</span>
            </div>
          </div>
        </header>

        {/* Main content */}
        <main className="flex-1 min-h-0 overflow-hidden">
          <Suspense fallback={<div className="h-full flex items-center justify-center text-sm text-[#8b949e]">Loading view...</div>}>
            <Routes>
              <Route path="/" element={<Navigate to="/explorer" replace />} />
              <Route path="/explorer" element={<Explorer />} />
              <Route path="/disk" element={<DiskLayout />} />
              <Route path="/inode" element={<InodeInspector />} />
              <Route path="/journal" element={<JournalViewer />} />
              <Route path="/terminal" element={<Terminal />} />
              <Route path="/stats" element={<Stats />} />
            </Routes>
          </Suspense>
        </main>
      </div>
    </BrowserRouter>
  );
}
