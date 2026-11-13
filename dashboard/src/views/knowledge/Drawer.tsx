import { KeyboardEvent, ReactNode, useEffect, useRef } from 'react';
import { X } from 'lucide-react';

interface DrawerProps {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
}

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

// Right-side panel. It stays mounted, so open/close are interruptible CSS transitions
// (transform + opacity, ease-out-strong, see `.kb-drawer` in index.css). While open it
// traps Tab focus, closes on Escape (window-level, so it still works after the content
// changes) and on a backdrop click, and restores focus to the element that opened it. Callers keep rendering the last content during the exit.
export default function Drawer({ open, title, onClose, children }: DrawerProps) {
  const panel = useRef<HTMLDivElement>(null);
  const opener = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;
    opener.current = document.activeElement as HTMLElement | null;
    const raf = requestAnimationFrame(() => panel.current?.focus({ preventScroll: true }));
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      cancelAnimationFrame(raf);
      document.body.style.overflow = prevOverflow;
      const el = opener.current;
      if (el && document.contains(el)) el.focus({ preventScroll: true });
    };
  }, [open]);

  // Escape anywhere closes the drawer (focus may sit on <body> after the content was replaced),
  // except inside another dialog stacked on top, such as a Modal, which handles its own Escape.
  const closeRef = useRef(onClose);
  closeRef.current = onClose;
  useEffect(() => {
    if (!open) return;
    const inOtherDialog = (t: EventTarget | null) => {
      const d = t instanceof Element ? t.closest('[role="dialog"]') : null;
      return !!d && d !== panel.current;
    };
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.key !== 'Escape' || e.defaultPrevented || inOtherDialog(e.target)) return;
      closeRef.current();
    };
    // Keep focus inside: if it lands outside the panel (and not in a stacked dialog), pull it back.
    const onFocusIn = (e: FocusEvent) => {
      if (!panel.current || panel.current.contains(e.target as Node) || inOtherDialog(e.target)) return;
      panel.current.focus({ preventScroll: true });
    };
    window.addEventListener('keydown', onKey);
    document.addEventListener('focusin', onFocusIn);
    return () => {
      window.removeEventListener('keydown', onKey);
      document.removeEventListener('focusin', onFocusIn);
    };
  }, [open]);

  // Navigating inside the drawer replaces the clicked element: keep focus in the panel.
  useEffect(() => {
    if (open && (document.activeElement === document.body || document.activeElement === null)) panel.current?.focus({ preventScroll: true });
  }, [open, title]);

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (e.key !== 'Tab' || !panel.current) return;
    const items = Array.from(panel.current.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((el) => el.offsetParent !== null);
    if (items.length === 0) {
      e.preventDefault();
      panel.current.focus();
      return;
    }
    const first = items[0];
    const last = items[items.length - 1];
    const active = document.activeElement;
    if (e.shiftKey && (active === first || active === panel.current)) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && active === last) {
      e.preventDefault();
      first.focus();
    }
  }

  return (
    <div className="kb-drawer fixed inset-0 z-50" data-open={open} aria-hidden={!open}>
      <div className="kb-drawer-backdrop absolute inset-0 bg-black/30" onMouseDown={onClose} />
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
        onKeyDown={onKeyDown}
        className="kb-drawer-panel absolute inset-y-0 right-0 flex w-full sm:w-[min(44rem,92vw)] flex-col bg-white shadow-2xl outline-none"
      >
        <div className="flex items-center justify-between gap-3 border-b border-gray-200 px-5 py-3">
          <h2 className="text-sm font-medium text-gray-500 truncate">{title}</h2>
          <button type="button" className="btn btn-secondary p-1.5 min-h-8 min-w-8 inline-flex items-center justify-center" onClick={onClose} aria-label="Close">
            <X className="w-4 h-4" aria-hidden="true" />
          </button>
        </div>
        <div className="flex-1 overflow-y-auto overscroll-contain px-5 py-4">{children}</div>
      </div>
    </div>
  );
}
