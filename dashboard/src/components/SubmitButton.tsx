import { ButtonHTMLAttributes, ReactNode } from 'react';
import { Loader2 } from 'lucide-react';

interface SubmitButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  loading?: boolean;
  loadingLabel?: ReactNode;
}

// Blocks double submission: while `loading`, the button is disabled and
// announces itself as busy. The spinner replaces the label's leading space so
// the button width does not jump.
export default function SubmitButton({
  loading = false,
  loadingLabel,
  disabled,
  className = '',
  children,
  onClick,
  type = 'button',
  ...rest
}: SubmitButtonProps) {
  return (
    <button
      {...rest}
      type={type}
      disabled={disabled || loading}
      aria-busy={loading}
      onClick={(e) => {
        if (loading) {
          e.preventDefault();
          return;
        }
        onClick?.(e);
      }}
      className={`inline-flex items-center justify-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed ${className}`}
    >
      {loading && <Loader2 className="w-4 h-4 animate-spin" aria-hidden="true" />}
      {loading && loadingLabel !== undefined ? loadingLabel : children}
    </button>
  );
}
