// Layout component with sidebar navigation
import { Link, useLocation } from 'react-router-dom';
import { useAuthStore, useStatusStore, useUIStore } from '../../store';
import {
  LayoutDashboard,
  Radio,
  Settings,
  FileText,
  BarChart3,
  Menu,
  LogOut,
  Clock,
  Wifi,
  WifiOff
} from 'lucide-react';
import clsx from 'clsx';

interface LayoutProps {
  children: React.ReactNode;
}

const navItems = [
  { path: '/', label: 'Dashboard', icon: LayoutDashboard },
  { path: '/sources', label: 'Sources', icon: Radio },
  { path: '/config', label: 'Configuration', icon: Settings },
  { path: '/logs', label: 'Logs', icon: FileText },
  { path: '/metrics', label: 'Metrics', icon: BarChart3 },
];

export default function Layout({ children }: LayoutProps) {
  const location = useLocation();
  const { user, logout } = useAuthStore();
  const { status, connected } = useStatusStore();
  const { sidebarOpen, toggleSidebar } = useUIStore();

  return (
    <div className="min-h-screen bg-gray-900 flex">
      {/* Sidebar */}
      <aside
        className={clsx(
          'fixed inset-y-0 left-0 z-50 w-64 bg-gray-800 border-r border-gray-700 transition-transform duration-300 lg:translate-x-0 lg:static',
          sidebarOpen ? 'translate-x-0' : '-translate-x-full'
        )}
      >
        <div className="flex flex-col h-full">
          {/* Logo */}
          <div className="flex items-center gap-3 px-6 py-4 border-b border-gray-700">
            <Clock className="w-8 h-8 text-primary-500" />
            <span className="text-xl font-bold text-white">SPTime</span>
          </div>

          {/* Navigation */}
          <nav className="flex-1 px-4 py-6 space-y-1">
            {navItems.map((item) => {
              const Icon = item.icon;
              const isActive = location.pathname === item.path;
              return (
                <Link
                  key={item.path}
                  to={item.path}
                  className={clsx(
                    'flex items-center gap-3 px-4 py-3 rounded-lg transition-colors',
                    isActive
                      ? 'bg-primary-600 text-white'
                      : 'text-gray-400 hover:bg-gray-700 hover:text-white'
                  )}
                >
                  <Icon className="w-5 h-5" />
                  <span className="font-medium">{item.label}</span>
                </Link>
              );
            })}
          </nav>

          {/* User section */}
          <div className="px-4 py-4 border-t border-gray-700">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium text-gray-200">{user?.username}</p>
                <p className="text-xs text-gray-400 capitalize">{user?.role}</p>
              </div>
              <button
                onClick={logout}
                className="p-2 text-gray-400 hover:text-white hover:bg-gray-700 rounded-lg transition-colors"
                title="Logout"
              >
                <LogOut className="w-5 h-5" />
              </button>
            </div>
          </div>
        </div>
      </aside>

      {/* Main content */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Header */}
        <header className="sticky top-0 z-40 bg-gray-800 border-b border-gray-700">
          <div className="flex items-center justify-between px-4 py-3">
            <button
              onClick={toggleSidebar}
              className="lg:hidden p-2 text-gray-400 hover:text-white hover:bg-gray-700 rounded-lg"
            >
              <Menu className="w-6 h-6" />
            </button>

            <div className="flex items-center gap-4 ml-auto">
              {/* Connection status */}
              <div className="flex items-center gap-2">
                {connected ? (
                  <>
                    <Wifi className="w-4 h-4 text-green-400" />
                    <span className="text-sm text-green-400">Connected</span>
                  </>
                ) : (
                  <>
                    <WifiOff className="w-4 h-4 text-red-400" />
                    <span className="text-sm text-red-400">Disconnected</span>
                  </>
                )}
              </div>

              {/* Clock state badge */}
              {status?.clock && (
                <div
                  className={clsx(
                    'px-3 py-1 rounded-full text-sm font-medium',
                    status.clock.state === 'LOCKED' && 'bg-green-900 text-green-300',
                    status.clock.state === 'SYNCING' && 'bg-yellow-900 text-yellow-300',
                    status.clock.state === 'HOLDOVER' && 'bg-orange-900 text-orange-300',
                    status.clock.state === 'FAULT' && 'bg-red-900 text-red-300',
                    status.clock.state === 'INIT' && 'bg-blue-900 text-blue-300'
                  )}
                >
                  {status.clock.state}
                </div>
              )}

              {/* Current time */}
              <div className="text-sm font-mono text-gray-300">
                {status?.time ? new Date(status.time).toISOString() : '--:--:--'}
              </div>
            </div>
          </div>
        </header>

        {/* Page content */}
        <main className="flex-1 p-6 overflow-auto">
          {children}
        </main>
      </div>

      {/* Mobile overlay */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/50 lg:hidden"
          onClick={toggleSidebar}
        />
      )}
    </div>
  );
}
