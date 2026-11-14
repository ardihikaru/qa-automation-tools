// Paging vocabulary for the knowledge tabs (kept out of ui.tsx so it only exports components).
import { useEffect, useRef } from 'react';
import { Coverage } from './api';

export const PAGE_SIZES = [5, 10, 25, 50] as const;
export const DEFAULT_PAGE_SIZE = 5;

/** `?size=` value, or the default when missing or not one of PAGE_SIZES. */
export function parseSize(v: string | null): number {
  const n = Number(v);
  return (PAGE_SIZES as readonly number[]).includes(n) ? n : DEFAULT_PAGE_SIZE;
}

/** `?page=` value: a positive integer, else 1. */
export function parsePage(v: string | null): number {
  const n = Number(v);
  return Number.isInteger(n) && n > 0 ? n : 1;
}

export const pageCount = (total: number, size: number) => Math.max(1, Math.ceil(total / size));

/**
 * Page buttons for `page` of `pages`: always the first and last, a window of two around the
 * current page, and `null` for each gap ("1 … 4 5 6 … 89"). A gap of one page shows the page.
 */
export function pageWindow(page: number, pages: number, around = 1): (number | null)[] {
  const want = new Set<number>([1, pages]);
  for (let p = page - around; p <= page + around; p++) if (p >= 1 && p <= pages) want.add(p);
  const sorted = [...want].sort((a, b) => a - b);
  const out: (number | null)[] = [];
  let prev = 0;
  for (const p of sorted) {
    if (p - prev === 2) out.push(prev + 1);
    else if (p - prev > 2) out.push(null);
    out.push(p);
    prev = p;
  }
  return out;
}

/** What a paged tab needs from the view: its page (from the URL), the shared size, and setters. */
export interface Paging {
  page: number;
  size: number;
  /** `replace` swaps the history entry (resets and clamps) instead of adding one (clicks). */
  setPage: (p: number, replace?: boolean) => void;
  setSize: (s: number) => void;
}

/**
 * Calls `reset` when `key` changes after the first render (filters changed: back to page 1).
 * The first render is skipped so a `?page=` from the URL survives a reload.
 */
export function useResetOnChange(key: string, reset: () => void) {
  const prev = useRef(key);
  useEffect(() => {
    if (prev.current === key) return;
    prev.current = key;
    reset();
    // `reset` is rebuilt every render; only `key` matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
}

/** Moves to the last page when the current one ran past the end (fewer rows, larger size). */
export function useClampPage(paging: Paging, total: number | null | undefined) {
  const { page, size, setPage } = paging;
  useEffect(() => {
    if (total === null || total === undefined) return;
    const last = pageCount(total, size);
    if (page > last) setPage(last, true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [total, size, page]);
}

/** True when a child has at least one active test case. */
export const isTested = (c: Coverage | null | undefined) => !!c && c.test_cases > 0;

/** Stable sort: tested items first, original order kept inside each group. */
export function testedFirst<T>(items: T[], tested: (t: T) => boolean): T[] {
  return [...items.filter(tested), ...items.filter((t) => !tested(t))];
}

/** Longer runs of untested children than this collapse into one "+ N … without test cases" line. */
export const UNTESTED_RUN = 3;
