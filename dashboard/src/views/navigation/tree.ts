// Path tree of the navigation map: one tree per (surface, app), one node per path segment.
// Pure, so the view only renders.
import { NavRoute, RouteStatus, STATUSES, Surface } from './api';

export interface TreeNode {
  /** Stable id: surface|app|path prefix. */
  id: string;
  segment: string;
  path: string;
  route: NavRoute | null;
  children: TreeNode[];
  /** Routes in this subtree (this node included) by status. */
  counts: Record<RouteStatus, number>;
  routes: number;
  cases: number;
  automated: number;
  observed: number;
}

export interface AppTree {
  surface: Surface;
  app: string;
  root: TreeNode;
}

const emptyCounts = (): Record<RouteStatus, number> => ({ failing: 0, passing: 0, automated: 0, planned: 0, untested: 0 });

function node(id: string, segment: string, path: string): TreeNode {
  return { id, segment, path, route: null, children: [], counts: emptyCounts(), routes: 0, cases: 0, automated: 0, observed: 0 };
}

/** Builds one tree per (surface, app), ordered web before api, then by app; children by segment. */
export function buildTrees(routes: NavRoute[]): AppTree[] {
  const trees = new Map<string, AppTree>();
  for (const r of routes) {
    const tk = `${r.surface}|${r.app}`;
    let t = trees.get(tk);
    if (!t) {
      t = { surface: r.surface, app: r.app, root: node(`${tk}|`, '/', '/') };
      trees.set(tk, t);
    }
    const segs = r.path.split('/').filter(Boolean);
    let cur = t.root;
    let path = '';
    for (const s of segs) {
      path += `/${s}`;
      let next = cur.children.find((c) => c.segment === s);
      if (!next) {
        next = node(`${tk}|${path}`, s, path);
        cur.children.push(next);
      }
      cur = next;
    }
    cur.route = r;
  }
  const out = [...trees.values()];
  for (const t of out) aggregate(t.root);
  out.sort((a, b) => (a.surface === b.surface ? a.app.localeCompare(b.app) : a.surface === 'web' ? -1 : 1));
  return out;
}

function aggregate(n: TreeNode): void {
  n.children.sort((a, b) => a.segment.localeCompare(b.segment));
  for (const c of n.children) {
    aggregate(c);
    for (const s of STATUSES) n.counts[s] += c.counts[s];
    n.routes += c.routes;
    n.cases += c.cases;
    n.automated += c.automated;
    n.observed += c.observed;
  }
  if (n.route) {
    n.counts[n.route.status]++;
    n.routes++;
    n.cases += n.route.cases;
    n.automated += n.route.automated;
    if (n.route.observed_hits > 0) n.observed++;
  }
}

/**
 * Collapses chains of segments that hold no route and a single child ("/api" → "/api/v1"), so the
 * tree does not open three levels deep before the first real route.
 */
export function compact(n: TreeNode): TreeNode {
  const children = n.children.map(compact);
  if (!n.route && children.length === 1 && n.segment !== '/') {
    const only = children[0];
    return { ...only, segment: `${n.segment}/${only.segment}` };
  }
  return { ...n, children };
}

/** The ids of every node, for "expand all". */
export function allIds(n: TreeNode, out: string[] = []): string[] {
  out.push(n.id);
  for (const c of n.children) allIds(c, out);
  return out;
}
