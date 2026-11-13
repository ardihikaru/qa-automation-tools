// Test case status and coverage vocabulary shared by the case grid, the segmented bars and the
// filters (kept out of the .tsx files so they only export components).
import { Coverage, CoverageState, TestCase } from './api';

export type CaseStatus = 'automated' | 'planned' | 'blocked' | 'manual-only' | 'wont';

/** Display order: what is done first, then what is waiting, then what never will be. */
export const CASE_STATUSES: { status: CaseStatus; label: string; key: keyof Coverage }[] = [
  { status: 'automated', label: 'Automated', key: 'automated' },
  { status: 'planned', label: 'Planned', key: 'planned' },
  { status: 'blocked', label: 'Blocked', key: 'blocked' },
  { status: 'manual-only', label: 'Manual only', key: 'manual_only' },
  { status: 'wont', label: "Won't automate", key: 'wont' },
];

export const statusLabel = (s: string) => CASE_STATUSES.find((x) => x.status === s)?.label ?? s;
const ORDER: Record<string, number> = Object.fromEntries(CASE_STATUSES.map((s, i) => [s.status, i]));
export const statusRank = (s: string) => ORDER[s] ?? CASE_STATUSES.length;

/** "312 of 540 test cases automated · 41 planned · 9 blocked" (the one middle-dot string). */
export function coverageSentence(c: Coverage | null | undefined, noun = 'test case'): string {
  if (!c || c.test_cases === 0) return `No ${noun}s yet`;
  const head = `${c.automated.toLocaleString()} of ${c.test_cases.toLocaleString()} ${c.test_cases === 1 ? noun : `${noun}s`} automated`;
  const rest = CASE_STATUSES.slice(1)
    .filter((s) => (c[s.key] as number) > 0)
    .map((s) => `${(c[s.key] as number).toLocaleString()} ${s.label.toLowerCase()}`);
  return [head, ...rest].join(' · ');
}

/** Coverage counted from a list of cases (used where the server sends cases, not a Coverage). */
export function coverageOf(cases: TestCase[]): Coverage {
  const c: Coverage = { test_cases: 0, automated: 0, planned: 0, blocked: 0, manual_only: 0, wont: 0, pct: 0, state: 'untested' };
  for (const tc of cases) {
    c.test_cases++;
    const k = CASE_STATUSES.find((s) => s.status === tc.status)?.key;
    if (k) (c[k] as number)++;
  }
  c.state = c.test_cases === 0 ? 'untested' : c.automated === 0 ? 'none' : c.automated < c.test_cases ? 'partial' : 'full';
  c.pct = c.test_cases ? (c.automated * 100) / c.test_cases : 0;
  return c;
}

/** Child-list filter: "Automated" means every case is automated; partial coverage is a gap. */
export type ChildFilter = 'all' | 'automated' | 'gaps' | 'untested';
export const CHILD_FILTERS: { id: ChildFilter; label: string; title: string; states: CoverageState[] | null }[] = [
  { id: 'all', label: 'All', title: 'Everything', states: null },
  { id: 'automated', label: 'Automated', title: 'Every test case is automated', states: ['full'] },
  { id: 'gaps', label: 'Not automated yet', title: 'Has test cases that are not automated yet', states: ['none', 'partial'] },
  { id: 'untested', label: 'Untested', title: 'No test cases yet', states: ['untested'] },
];

export function matchesFilter(c: Coverage | null | undefined, f: ChildFilter): boolean {
  const states = CHILD_FILTERS.find((x) => x.id === f)?.states;
  return !states || states.includes(c?.state ?? 'untested');
}

/** Scenario text on one line, `<br>` removed. */
export const flatText = (s: string) => s.replace(/<br\s*\/?>/gi, ' ').replace(/\*\*/g, '').replace(/\s+/g, ' ').trim();
export const excerpt = (s: string, n = 160) => {
  const t = flatText(s);
  return t.length > n ? `${t.slice(0, n - 1)}…` : t;
};
