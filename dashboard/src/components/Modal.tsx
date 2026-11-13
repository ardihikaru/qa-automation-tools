import { ReactNode, useEffect } from 'react';

interface ModalProps {
  title: string;
  onClose: () => void;
  children: ReactNode;
}

// Shared centred dialog: Escape and a click on the backdrop call onClose, and
// the panel is labelled by its title for screen readers. Callers that must not
// close while a write is in flight guard it in onClose themselves
// (e.g. `onClose={() => !saving && setOpen(false)}`).
//
// Enter motion is the `.modal-enter` class in index.css (opacity + scale from
// 0.97, ease-out, 160ms; fade only under prefers-reduced-motion). Modals stay
// centred rather than scaling from a trigger.
export default function Modal({ title, onClose, children }: ModalProps) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/40"
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="modal-enter bg-white rounded-xl shadow-xl w-full max-w-lg p-6 max-h-[90vh] overflow-y-auto"
      >
        <h3 className="text-lg font-semibold mb-4">{title}</h3>
        {children}
      </div>
    </div>
  );
}
