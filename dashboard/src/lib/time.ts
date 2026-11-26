// Formatting in the system timezone (app_settings.timezone, default Asia/Jakarta).
// The API sends UTC timestamps; only the dashboard converts them.
import { useEffect, useSyncExternalStore } from 'react';

export const DEFAULT_TIMEZONE = 'Asia/Jakarta';

let zone = DEFAULT_TIMEZONE;
let loaded = false;
let inflight: Promise<void> | null = null;
const listeners = new Set<() => void>();

const emit = () => listeners.forEach((l) => l());

const isZone = (z: unknown): z is string => {
  if (typeof z !== 'string' || !z) return false;
  try {
    new Intl.DateTimeFormat('en-GB', { timeZone: z });
    return true;
  } catch {
    return false;
  }
};

/** Sets the cached zone (after a successful PUT /api/settings/timezone). */
export function setSystemTimezone(z: string) {
  if (!isZone(z) || z === zone) return;
  zone = z;
  loaded = true;
  emit();
}

function loadOnce(): Promise<void> {
  if (loaded) return Promise.resolve();
  inflight ??= fetch('/api/settings', { credentials: 'include' })
    .then((r) => (r.ok ? r.json() : null))
    .then((d: { timezone?: string } | null) => {
      if (d && isZone(d.timezone)) zone = d.timezone;
      loaded = true;
      emit();
    })
    .catch(() => undefined)
    .finally(() => {
      inflight = null;
    });
  return inflight;
}

/** The system timezone (IANA name). Fetched once per page load and shared by every caller. */
export function useSystemTimezone(): string {
  const z = useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    () => zone,
  );
  useEffect(() => {
    void loadOnce();
  }, []);
  return z;
}

/** "27 Nov 2026, 14:05:09 WIB" in `tz`; '' for a missing or invalid timestamp. */
export function formatInZone(at: string | null | undefined, tz: string = zone, opts: { seconds?: boolean; zone?: boolean } = {}): string {
  if (!at) return '';
  const d = new Date(at);
  if (Number.isNaN(d.getTime())) return '';
  const timeZone = isZone(tz) ? tz : DEFAULT_TIMEZONE;
  const base = new Intl.DateTimeFormat('en-GB', {
    timeZone,
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    ...(opts.seconds === false ? {} : { second: '2-digit' }),
    hour12: false,
  }).format(d);
  if (opts.zone === false) return base;
  return `${base} ${zoneAbbr(d, timeZone)}`;
}

/** Short zone name: WIB for Asia/Jakarta, else Intl's short name (falls back to GMT+x). */
export function zoneAbbr(d: Date, timeZone: string): string {
  const ids: Record<string, string> = { 'Asia/Jakarta': 'WIB', 'Asia/Makassar': 'WITA', 'Asia/Jayapura': 'WIT' };
  if (ids[timeZone]) return ids[timeZone];
  const part = new Intl.DateTimeFormat('en-GB', { timeZone, timeZoneName: 'short' }).formatToParts(d).find((p) => p.type === 'timeZoneName');
  return part?.value ?? timeZone;
}

/** "1h 02m 05s", "4m 12s", "7s" from milliseconds. */
export function formatElapsed(ms: number | null | undefined): string {
  if (ms === null || ms === undefined || !Number.isFinite(ms) || ms < 0) return '';
  const s = Math.floor(ms / 1000);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) return `${h}h ${String(m).padStart(2, '0')}m ${String(sec).padStart(2, '0')}s`;
  if (m > 0) return `${m}m ${String(sec).padStart(2, '0')}s`;
  return `${sec}s`;
}
