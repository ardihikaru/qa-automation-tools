import { ReactNode } from 'react';
import { useLocation } from 'react-router-dom';

// Re-mounts on route change so the CSS enter animation replays.
// Animates opacity + transform only (see .page-enter in index.css).
export default function PageTransition({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  return (
    <div key={pathname} className="page-enter">
      {children}
    </div>
  );
}
