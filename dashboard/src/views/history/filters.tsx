// Filter controls of the run history page.
import { ReactNode, useEffect, useState } from 'react';
import { inputCls, selectCls } from '../knowledge/styles';

export function FilterField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1 text-xs font-medium text-gray-600 min-w-0">
      {label}
      {children}
    </label>
  );
}

export function FilterSelect({ label, value, options, onChange }: { label: string; value: string; options: [string, string][]; onChange: (v: string) => void }) {
  return (
    <FilterField label={label}>
      <select className={selectCls} value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">All</option>
        {options.map(([v, l]) => (
          <option key={v} value={v}>
            {l}
          </option>
        ))}
      </select>
    </FilterField>
  );
}

/** A code filter (case, epic, job): applied on Enter or when the field loses focus. */
export function FilterText({ label, value, placeholder, onChange }: { label: string; value: string; placeholder: string; onChange: (v: string) => void }) {
  const [draft, setDraft] = useState(value);
  useEffect(() => setDraft(value), [value]);
  const commit = () => {
    const v = draft.trim();
    if (v !== value) onChange(v);
  };
  return (
    <FilterField label={label}>
      <input
        className={`${inputCls} w-36`}
        value={draft}
        placeholder={placeholder}
        maxLength={64}
        spellCheck={false}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault();
            commit();
          }
        }}
      />
    </FilterField>
  );
}

