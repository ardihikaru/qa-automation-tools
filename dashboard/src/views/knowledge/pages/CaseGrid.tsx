// The case grid: every active test case under an entity as a small square coloured by status,
// one wrapping row per story. One tab stop for the whole grid; arrow keys move between squares
// (up/down pick the nearest square in the visual row above/below), Enter opens the test case.
import { KeyboardEvent, ReactNode, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { TestCase, entityHref } from '../api';
import { CASE_STATUSES, excerpt, statusLabel, statusRank } from '../coverage';
import { ErrorNote, Loading } from '../ui';
import { SectionHead, StatusSwatch } from './kit';

interface Props {
  pid: number;
  cases: TestCase[] | null;
  loading?: boolean;
  error?: string | null;
  onRetry?: () => void;
  /** Story code → story text, for the row label tooltip. */
  storyText?: Record<string, string>;
  /** Shown when there are no cases. */
  empty: ReactNode;
}

interface Group {
  story: string;
  cases: TestCase[];
}

const NO_STORY = '';

function group(cases: TestCase[]): Group[] {
  const by = new Map<string, TestCase[]>();
  for (const c of cases) {
    if (c.removed_at) continue;
    const k = c.story_code ?? NO_STORY;
    const list = by.get(k);
    if (list) list.push(c);
    else by.set(k, [c]);
  }
  return [...by.entries()]
    .sort(([a], [b]) => (a === NO_STORY ? 1 : b === NO_STORY ? -1 : a.localeCompare(b, undefined, { numeric: true })))
    .map(([story, list]) => ({
      story,
      cases: list.sort((x, y) => statusRank(x.status) - statusRank(y.status) || x.code.localeCompare(y.code, undefined, { numeric: true })),
    }));
}

export function Legend({ cases }: { cases: TestCase[] }) {
  const counts: Record<string, number> = {};
  for (const c of cases) if (!c.removed_at) counts[c.status] = (counts[c.status] ?? 0) + 1;
  return (
    <ul className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-600" aria-label="Legend">
      {CASE_STATUSES.map((s) => (
        <li key={s.status} className={`inline-flex items-center gap-1.5 ${counts[s.status] ? '' : 'text-gray-400'}`}>
          <StatusSwatch status={s.status} />
          {s.label}
          <span className="tabular-nums">{(counts[s.status] ?? 0).toLocaleString()}</span>
        </li>
      ))}
    </ul>
  );
}

export default function CaseGrid({ pid, cases, loading, error, onRetry, storyText, empty }: Props) {
  const groups = useMemo(() => (cases ? group(cases) : []), [cases]);
  const flat = useMemo(() => groups.flatMap((g) => g.cases), [groups]);
  const [active, setActive] = useState(0);
  const [tip, setTip] = useState<{ c: TestCase; x: number; y: number; alignRight: boolean } | null>(null);
  const box = useRef<HTMLDivElement>(null);
  const cells = useRef<(HTMLAnchorElement | null)[]>([]);

  if (error) return <ErrorNote onRetry={onRetry}>{error}</ErrorNote>;
  if (loading || !cases) return <Loading label="Loading test cases…" />;
  if (flat.length === 0) return <p className="text-sm text-gray-600 rounded-lg border border-dashed border-gray-300 px-4 py-6">{empty}</p>;

  const activeIdx = Math.min(active, flat.length - 1);

  function show(el: HTMLElement, c: TestCase) {
    const b = box.current?.getBoundingClientRect();
    const r = el.getBoundingClientRect();
    if (!b) return;
    const x = r.left - b.left + r.width / 2;
    setTip({ c, x, y: r.top - b.top, alignRight: x > b.width - 160 });
  }

  function move(e: KeyboardEvent<HTMLDivElement>) {
    const i = flat.findIndex((_, k) => cells.current[k] === document.activeElement);
    if (i < 0) return;
    let next = -1;
    if (e.key === 'ArrowRight') next = Math.min(flat.length - 1, i + 1);
    else if (e.key === 'ArrowLeft') next = Math.max(0, i - 1);
    else if (e.key === 'Home') next = 0;
    else if (e.key === 'End') next = flat.length - 1;
    else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      const down = e.key === 'ArrowDown';
      const cur = cells.current[i]?.getBoundingClientRect();
      if (!cur) return;
      let rowTop: number | null = null;
      let best = -1;
      let bestDx = Infinity;
      cells.current.forEach((el, k) => {
        if (!el || k >= flat.length) return;
        const r = el.getBoundingClientRect();
        const beyond = down ? r.top > cur.top + 2 : r.top < cur.top - 2;
        if (!beyond) return;
        if (rowTop === null || (down ? r.top < rowTop - 2 : r.top > rowTop + 2)) {
          rowTop = r.top;
          best = -1;
          bestDx = Infinity;
        }
        if (Math.abs(r.top - rowTop) <= 2) {
          const dx = Math.abs(r.left - cur.left);
          if (dx < bestDx) {
            bestDx = dx;
            best = k;
          }
        }
      });
      next = best;
    } else return;
    e.preventDefault();
    if (next < 0 || next === i) return;
    setActive(next);
    cells.current[next]?.focus();
  }

  let n = 0;
  return (
    <div ref={box} className="relative" onKeyDown={move} onMouseLeave={() => setTip(null)}>
      <p id="kb-grid-help" className="sr-only">
        Each link is one test case. Use the arrow keys to move between test cases and Enter to open one.
      </p>
      <div className="grid gap-x-10 gap-y-1.5 [grid-template-columns:repeat(auto-fill,minmax(min(100%,28rem),1fr))]">
        {groups.map((g) => (
          <div key={g.story || '(none)'} className="min-w-0 grid grid-cols-1 sm:grid-cols-[7rem_minmax(0,1fr)] gap-x-3 gap-y-1 items-start">
            <span className="font-mono text-[11px] leading-[13px] text-gray-400 truncate pt-px" title={g.story ? storyText?.[g.story] ?? g.story : 'Not linked to a story'}>
              {g.story || 'No story'}
            </span>
            <ul className="flex flex-wrap gap-[2px] min-w-0" aria-label={g.story ? `Story ${g.story}` : 'No story'}>
              {g.cases.map((c) => {
                const k = n++;
                return (
                  <li key={c.code}>
                    <Link
                      ref={(el) => (cells.current[k] = el)}
                      to={entityHref(pid, 'test_case', c.code)}
                      tabIndex={k === activeIdx ? 0 : -1}
                      data-s={c.status}
                      className="kb-st kb-cell focus:outline-none"
                      aria-label={`${c.code}, ${statusLabel(c.status)}: ${excerpt(c.scenario, 120)}`}
                      onMouseEnter={(e) => show(e.currentTarget, c)}
                      onFocus={(e) => {
                        setActive(k);
                        show(e.currentTarget, c);
                      }}
                      onBlur={() => setTip(null)}
                    />
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </div>
      <div
        role="presentation"
        aria-hidden="true"
        data-show={!!tip}
        className="kb-tip pointer-events-none absolute z-10 w-max max-w-[18rem] rounded-md bg-gray-900 px-2.5 py-1.5 text-xs text-white shadow-lg"
        style={tip ? { left: tip.x, top: tip.y - 6, transform: `translate(${tip.alignRight ? '-100%' : '-12px'}, -100%)` } : { left: 0, top: 0 }}
      >
        {tip && (
          <>
            <span className="flex items-center gap-1.5 font-mono font-semibold">
              <StatusSwatch status={tip.c.status} size={8} />
              {tip.c.code}
              <span className="font-sans font-normal text-gray-300">{statusLabel(tip.c.status)}</span>
            </span>
            <span className="mt-0.5 block text-gray-200 break-words">{excerpt(tip.c.scenario, 140)}</span>
          </>
        )}
      </div>
    </div>
  );
}

/** "Test cases" section: heading with count and legend, then the grid. */
export function CaseGridSection(props: Props & { title?: string }) {
  const active = props.cases?.filter((c) => !c.removed_at) ?? null;
  return (
    <section aria-labelledby="kb-grid-title" className="space-y-3">
      <SectionHead id="kb-grid-title" title={props.title ?? 'Test cases'} count={active?.length}>
        {active && active.length > 0 && <Legend cases={active} />}
      </SectionHead>
      <CaseGrid {...props} />
    </section>
  );
}
