// Page title bar: breadcrumb trail, page title, page actions and the user chip.
// State lives in ./pageHeader (pages call `usePageHeader`).
import { useEffect } from 'react';
import { Link } from 'react-router-dom';
import { ChevronLeft, ChevronRight, Menu } from 'lucide-react';
import UserChip from './UserChip';
import { TopBarCrumb, useCurrentPageHeader } from './pageHeader';

function CrumbTrail({ crumbs }: { crumbs: TopBarCrumb[] }) {
  return (
    <nav aria-label="Breadcrumb" className="hidden lg:block min-w-0 shrink">
      <ol className="flex items-center gap-1 text-sm text-gray-500 min-w-0">
        {crumbs.map((c, i) => (
          <li key={`${i}:${c.to}`} className="flex items-center gap-1 min-w-0">
            <Link
              to={c.to}
              title={c.title ?? c.label}
              onClick={c.onClick}
              className="truncate max-w-[14rem] rounded hover:text-gray-900 hover:underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
            >
              {c.label}
            </Link>
            <ChevronRight className="w-4 h-4 shrink-0 text-gray-400" aria-hidden="true" />
          </li>
        ))}
      </ol>
    </nav>
  );
}

interface TopBarProps {
  /** Title when the page sets none (the matching nav label). */
  fallbackTitle: string;
  onMenu: () => void;
}

export default function TopBar({ fallbackTitle, onMenu }: TopBarProps) {
  const header = useCurrentPageHeader();
  const title = header?.title || fallbackTitle;
  const crumbs = header?.crumbs ?? [];
  const parent = crumbs[crumbs.length - 1];

  useEffect(() => {
    document.title = title ? `${title} · QA Dashboard` : 'QA Dashboard';
  }, [title]);

  return (
    <header className="flex items-center gap-2 lg:gap-3 h-14 lg:h-16 px-3 lg:px-6 bg-white border-b border-gray-200">
      <button
        type="button"
        className="lg:hidden -ml-1 inline-flex items-center justify-center w-10 h-10 rounded-lg text-gray-700 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
        onClick={onMenu}
        aria-label="Open menu"
      >
        <Menu className="w-6 h-6" aria-hidden="true" />
      </button>
      {parent && (
        <Link
          to={parent.to}
          onClick={parent.onClick}
          aria-label={`Back to ${parent.label}`}
          title={`Back to ${parent.label}`}
          className="lg:hidden inline-flex items-center justify-center w-9 h-9 shrink-0 rounded-lg text-gray-600 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
        >
          <ChevronLeft className="w-5 h-5" aria-hidden="true" />
        </Link>
      )}

      <div className="flex items-center gap-1 min-w-0 flex-1">
        {crumbs.length > 0 && <CrumbTrail crumbs={crumbs} />}
        <h1 className="min-w-0 truncate text-base font-semibold text-gray-900" title={header?.titleHint ?? title}>
          {title}
        </h1>
      </div>

      {header?.actions && <div className="flex items-center gap-2 shrink-0">{header.actions}</div>}
      <UserChip />
    </header>
  );
}
