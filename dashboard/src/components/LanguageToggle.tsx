import { useRef, useState } from 'react';
import { useAuth } from '../auth/AuthContext';
import type { Language } from '../auth/api';

const OPTIONS: { value: Language; label: string; title: string }[] = [
  { value: 'en', label: 'EN', title: 'Show the knowledge base in English' },
  { value: 'id', label: 'ID', title: 'Show the knowledge base in Indonesian (machine translated)' },
];

// Display language of the knowledge base only; the dashboard chrome stays English.
// Optimistic: AuthContext flips the language at once (which reloads the page data) and rolls back on error.
export default function LanguageToggle() {
  const { language, changeLanguage } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const inflight = useRef(false);

  async function pick(next: Language) {
    if (next === language || inflight.current) return;
    inflight.current = true;
    setError(null);
    try {
      await changeLanguage(next);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not save the language');
    } finally {
      inflight.current = false;
    }
  }

  return (
    <div className="relative shrink-0">
      <div role="group" aria-label="Knowledge language" className="inline-flex rounded-lg border border-gray-200 bg-white p-0.5 text-xs font-medium">
        {OPTIONS.map((o) => (
          <button
            key={o.value}
            type="button"
            title={o.title}
            aria-pressed={language === o.value}
            onClick={() => pick(o.value)}
            className={`min-w-9 h-7 px-2 rounded-md transition-[background-color,color,transform] duration-150 ease-out active:scale-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 ${
              language === o.value ? 'bg-primary-600 text-white' : 'text-gray-600 hover:bg-gray-100'
            }`}
          >
            {o.label}
          </button>
        ))}
      </div>
      {error && (
        <div role="alert" className="absolute right-0 top-full mt-1 z-30 w-56 rounded-lg border border-red-100 bg-red-50 px-3 py-2 text-xs text-red-700 shadow-lg break-words">
          {error}
        </div>
      )}
    </div>
  );
}
