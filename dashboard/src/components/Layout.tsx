import { ReactNode, useState } from 'react';
import { NavLink } from 'react-router-dom';
import { 
  FileText, 
  Bug, 
  AlertTriangle, 
  DollarSign, 
  CheckCircle,
  Menu,
  X
} from 'lucide-react';

interface LayoutProps {
  children: ReactNode;
}

const navItems = [
  { path: '/docs', label: 'Documentation', icon: FileText },
  { path: '/qa', label: 'QA Insights', icon: Bug },
  { path: '/issues', label: 'Issue Debugger', icon: AlertTriangle },
  { path: '/cost', label: 'Cost Analytics', icon: DollarSign },
  { path: '/fixes', label: 'Fix Tracking', icon: CheckCircle },
];

export default function Layout({ children }: LayoutProps) {
  const [sidebarOpen, setSidebarOpen] = useState(false);

  return (
    <div className="min-h-screen flex">
      {/* Mobile sidebar backdrop */}
      {sidebarOpen && (
        <div 
          className="fixed inset-0 bg-black bg-opacity-50 z-40 lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Sidebar */}
      <aside className={`
        fixed lg:static inset-y-0 left-0 z-50
        w-64 bg-gray-900 text-white
        transform transition-transform duration-200
        lg:translate-x-0
        ${sidebarOpen ? 'translate-x-0' : '-translate-x-full'}
      `}>
        <div className="flex items-center justify-between p-4 border-b border-gray-800">
          <h1 className="text-xl font-bold">QA Dashboard</h1>
          <button 
            className="lg:hidden"
            onClick={() => setSidebarOpen(false)}
          >
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
                transition-colors
                ${isActive 
                  ? 'bg-primary-600 text-white' 
                  : 'text-gray-300 hover:bg-gray-800 hover:text-white'}
              `}
              onClick={() => setSidebarOpen(false)}
            >
              <item.icon className="w-5 h-5" />
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>

      {/* Main content */}
      <div className="flex-1 flex flex-col min-h-screen">
        {/* Mobile header */}
        <header className="lg:hidden flex items-center justify-between p-4 bg-white border-b">
          <h1 className="text-lg font-semibold">QA Dashboard</h1>
          <button onClick={() => setSidebarOpen(true)}>
            <Menu className="w-6 h-6" />
          </button>
        </header>

        {/* Page content */}
        <main className="flex-1 p-6">
          {children}
        </main>
      </div>
    </div>
  );
}