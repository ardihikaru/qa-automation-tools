import { ReactNode, useState } from 'react';
import { NavLink } from 'react-router-dom';
import {
  FileText,
  Bug,
  AlertTriangle,
  DollarSign,
  CheckCircle,
  Menu,
  X,
  Zap,
  GitBranch,
  Brain,
  Bot,
  ScrollText,
} from 'lucide-react';
import UserChip from './UserChip';
import ImpersonationBanner from './ImpersonationBanner';
import PageTransition from './PageTransition';
import { LogoMark, ArchitectureIcon, ProductsIcon } from './icons';

interface LayoutProps {
  children: ReactNode;
}

const navItems = [
  { path: '/docs', label: 'Documentation', icon: FileText },
  { path: '/qa', label: 'QA Insights', icon: Bug },
  { path: '/issues', label: 'Issue Debugger', icon: AlertTriangle },
  { path: '/cost', label: 'Cost Analytics', icon: DollarSign },
  { path: '/fixes', label: 'Fix Tracking', icon: CheckCircle },
  { path: '/skills', label: 'Skills', icon: Zap },
  { path: '/agents', label: 'Agents', icon: Bot },
  { path: '/rules', label: 'Rules', icon: ScrollText },
  { path: '/navigation', label: 'Navigation Graph', icon: GitBranch },
  { path: '/learning', label: 'Learning Progress', icon: Brain },
  { path: '/products', label: 'Products', icon: ProductsIcon },
  { path: '/settings', label: 'Configuration', icon: ArchitectureIcon },
];

export default function Layout({ children }: LayoutProps) {
  const [sidebarOpen, setSidebarOpen] = useState(false);

  return (
    <div className="min-h-screen flex">
      {sidebarOpen && (
        <div
          className="fixed inset-0 bg-black bg-opacity-50 z-40 lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      <aside
        className={`
        fixed lg:static inset-y-0 left-0 z-50
        w-64 bg-gray-900 text-white
        transform transition-transform duration-200
        lg:translate-x-0
        ${sidebarOpen ? 'translate-x-0' : '-translate-x-full'}
      `}
      >
        <div className="flex items-center justify-between p-4 border-b border-gray-800">
          <h1 className="text-xl font-bold flex items-center gap-2">
            <LogoMark className="w-7 h-7 text-primary-400" />
            QA Dashboard
          </h1>
          <button className="lg:hidden" onClick={() => setSidebarOpen(false)}>
            <X className="w-6 h-6" />
          </button>
        </div>

        <nav className="p-4">
          {navItems.map((item) => (
            <NavLink
              key={item.path}
              to={item.path}
              className={({ isActive }) => `
                flex items-center gap-3 px-4 py-3 rounded-lg mb-1
                transition-colors duration-150 ease-out-strong
                ${isActive ? 'bg-primary-600 text-white' : 'text-gray-300 hover:bg-gray-800 hover:text-white'}
              `}
              onClick={() => setSidebarOpen(false)}
            >
              <item.icon className="w-5 h-5" />
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>

      <div className="flex-1 min-w-0 flex flex-col min-h-screen">
        <ImpersonationBanner />
        <header className="flex items-center justify-between gap-3 p-4 bg-white border-b">
          <button className="lg:hidden" onClick={() => setSidebarOpen(true)}>
            <Menu className="w-6 h-6" />
          </button>
          <h1 className="lg:hidden text-lg font-semibold">QA Dashboard</h1>
          <div className="hidden lg:block" />
          <UserChip />
        </header>

        <main className="flex-1 w-full min-w-0 p-6">
          <PageTransition>{children}</PageTransition>
        </main>
      </div>
    </div>
  );
}
