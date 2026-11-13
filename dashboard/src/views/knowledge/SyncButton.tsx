import { RefreshCw } from 'lucide-react';

interface Props {
  /** Accessible name, e.g. "Sync knowledge base of HISv3". */
  label: string;
  busy: boolean;
  disabled?: boolean;
  /** Why it is disabled (tooltip). */
  reason?: string;
  onClick: () => void;
  /** Show "Sync" / "Syncing…" next to the icon from `sm` up (top bar); icon only otherwise. */
  withText?: boolean;
}

/**
 * Compact knowledge sync button. While a sync runs it is disabled and `aria-busy`; the icon
 * turns only when motion is allowed, and the text (or the caller's status line) carries the
 * state for reduced motion.
 */
export default function SyncButton({ label, busy, disabled, reason, onClick, withText }: Props) {
  return (
    <button
      type="button"
      aria-label={busy ? `${label} (syncing)` : label}
      title={reason ?? (busy ? 'Syncing…' : label)}
      aria-busy={busy}
      disabled={busy || disabled}
      onClick={onClick}
      className={`btn btn-secondary inline-flex items-center justify-center gap-1.5 h-8 text-sm disabled:cursor-not-allowed ${
        withText ? 'px-2 sm:px-3' : 'w-8 px-0'
      } ${disabled && !busy ? 'disabled:opacity-50' : 'disabled:opacity-80'} focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500`}
    >
      <RefreshCw className={`w-4 h-4 shrink-0 ${busy ? 'motion-safe:animate-spin' : ''}`} aria-hidden="true" />
      {withText && <span className="hidden sm:inline">{busy ? 'Syncing…' : 'Sync'}</span>}
    </button>
  );
}
