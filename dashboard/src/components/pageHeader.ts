// Page header state for the top bar. Pages describe themselves with
// `usePageHeader({ title, crumbs, actions })`; pages that say nothing get the matching nav
// label (computed by Layout) as their title.
//
// Several components on one page may claim the header (a route shell while it loads, then the
// loaded page). The most recent claim wins; a released claim falls back to the previous one.
import { ReactNode, createContext, createElement, useCallback, useContext, useEffect, useMemo, useState } from 'react';

export interface TopBarCrumb {
  label: string;
  to: string;
  /** Tooltip when the label is shortened (e.g. "F-01 Registration"). */
  title?: string;
  /** Extra behaviour on click (e.g. close a detail view that lives on the same route). */
  onClick?: () => void;
}

export interface PageHeader {
  title?: string;
  /** Plain-text tooltip for the title (defaults to the title). */
  titleHint?: string;
  crumbs?: TopBarCrumb[];
  /** Compact page actions, shown left of the user chip. */
  actions?: ReactNode;
}

type Claim = { owner: symbol; header: PageHeader };
type Setter = (owner: symbol, header: PageHeader | null) => void;

// Split contexts: pages only subscribe to the stable setter, so updating the header never
// re-renders the page that set it.
const SetHeaderCtx = createContext<Setter>(() => undefined);
const HeaderCtx = createContext<PageHeader | null>(null);

export function TopBarProvider({ children }: { children: ReactNode }) {
  const [claims, setClaims] = useState<Claim[]>([]);
  const set = useCallback<Setter>((owner, header) => {
    setClaims((cur) => {
      const i = cur.findIndex((c) => c.owner === owner);
      if (!header) return i < 0 ? cur : cur.filter((c) => c.owner !== owner);
      if (i < 0) return [...cur, { owner, header }];
      const next = cur.slice();
      next[i] = { owner, header };
      return next;
    });
  }, []);
  const current = claims.length ? claims[claims.length - 1].header : null;
  return createElement(SetHeaderCtx.Provider, { value: set }, createElement(HeaderCtx.Provider, { value: current }, children));
}

/** The header the current page asked for (null: use the default title). */
export const useCurrentPageHeader = () => useContext(HeaderCtx);

/**
 * Sets the top bar for as long as the calling component is mounted. Pass `null` to leave the
 * header to another component (for example while a child page owns it).
 */
export function usePageHeader(header: PageHeader | null) {
  const set = useContext(SetHeaderCtx);
  const [owner] = useState(() => Symbol('page-header'));
  const crumbsKey = header?.crumbs ? JSON.stringify(header.crumbs.map((c) => [c.label, c.to, c.title ?? ''])) : '';
  const active = header !== null;
  const title = header?.title;
  const titleHint = header?.titleHint;
  const actions = header?.actions;
  // Re-claim only when the crumbs' text or targets change; their onClick handlers are expected
  // to be stable (state setters).
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const crumbs = useMemo(() => header?.crumbs, [crumbsKey]);

  useEffect(() => {
    set(owner, active ? { title, titleHint, crumbs, actions } : null);
  }, [set, owner, active, title, titleHint, crumbs, actions]);

  useEffect(() => () => set(owner, null), [set, owner]);
}
