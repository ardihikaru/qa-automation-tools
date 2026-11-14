// Page title bar: breadcrumb trail, page title, page actions and the user chip.
// State lives in ./pageHeader (pages call `usePageHeader`).
import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { ChevronLeft, ChevronRight, Menu } from 'lucide-react';
import UserChip from './UserChip';
import { TopBarCrumb, useCurrentPageHeader } from './pageHeader';

function CrumbLink({ c }: { c: TopBarCrumb }) {
  return (
    <Link
      to={c.to}
      title={c.title ?? c.label}
      onClick={c.onClick}
      className="truncate max-w-[14rem] rounded hover:text-gray-900 hover:underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
    >
      {c.label}
    </Link>
  );
}

const Sep = () => <ChevronRight className="w-4 h-4 shrink-0 text-gray-400" aria-hidden="true" />;

/** The middle crumbs of a long trail behind one "…" button; the list scales in from the button. */
function CrumbMenu({ crumbs }: { crumbs: TopBarCrumb[] }) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLSpanElement>(null);
  const button = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const down = (e: PointerEvent) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false);
    };
    const key = (e: globalThis.KeyboardEvent) => {
      if (e.key !== 'Escape') return;
      setOpen(false);
      button.current?.focus();
    };
    document.addEventListener('pointerdown', down);
    document.addEventListener('keydown', key);
    return () => {
      document.removeEventListener('pointerdown', down);
      document.removeEventListener('keydown', key);
    };
  }, [open]);

  return (
    <span ref={box} className="relative shrink-0">
      <button
        ref={button}
        type="button"
        aria-expanded={open}
        aria-controls="topbar-crumb-menu"
        aria-label={`Show ${crumbs.length} more breadcrumbs`}
        title={crumbs.map((c) => c.label).join(' › ')}
        onClick={() => setOpen((o) => !o)}
        className="inline-flex items-center justify-center h-7 min-w-7 px-1.5 rounded text-gray-500 hover:text-gray-900 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
      >
        …
      </button>
      <ul
        id="topbar-crumb-menu"
        data-open={open}
        className="topbar-pop absolute left-0 top-full mt-1 z-30 w-max max-w-[24rem] rounded-lg border border-gray-200 bg-white py-1 shadow-lg"
      >
        {crumbs.map((c, i) => (
          <li key={`${i}:${c.to}`}>
            <Link
              to={c.to}
              title={c.title ?? c.label}
              tabIndex={open ? 0 : -1}
              onClick={() => {
                setOpen(false);
                c.onClick?.();
              }}
              className="block truncate px-3 py-2 text-sm text-gray-700 hover:bg-gray-50 focus-visible:outline-none focus-visible:bg-gray-50"
            >
              {c.label}
            </Link>
          </li>
        ))}
      </ul>
    </span>
  );
}

/** Long trails keep the first crumb and the two nearest parents; the rest fold into "…". */
const MAX_CRUMBS = 3;

function CrumbTrail({ crumbs }: { crumbs: TopBarCrumb[] }) {
  const fold = crumbs.length > MAX_CRUMBS;
  const head = fold ? crumbs.slice(0, 1) : crumbs;
  const middle = fold ? crumbs.slice(1, -2) : [];
  const tail = fold ? crumbs.slice(-2) : [];
  return (
    <nav aria-label="Breadcrumb" className="hidden lg:block min-w-0 shrink">
      <ol className="flex items-center gap-1 text-sm text-gray-500 min-w-0">
        {head.map((c, i) => (
          <li key={`h${i}:${c.to}`} className="flex items-center gap-1 min-w-0">
            <CrumbLink c={c} />
            <Sep />
          </li>
        ))}
        {middle.length > 0 && (
          <li className="flex items-center gap-1 shrink-0">
            <CrumbMenu crumbs={middle} />
            <Sep />
          </li>
        )}
        {tail.map((c, i) => (
          <li key={`t${i}:${c.to}`} className="flex items-center gap-1 min-w-0">
            <CrumbLink c={c} />
            <Sep />
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
        <h1 className="min-w-0 lg:shrink-0 lg:max-w-[50%] truncate text-base font-semibold text-gray-900" title={header?.titleHint ?? title}>
          {title}
        </h1>
      </div>

      {header?.actions && <div className="flex items-center gap-2 shrink-0">{header.actions}</div>}
      <UserChip />
    </header>
  );
}
