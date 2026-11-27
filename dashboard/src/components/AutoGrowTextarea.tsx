import { KeyboardEvent, TextareaHTMLAttributes, useLayoutEffect, useRef } from 'react';

interface Props extends Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, 'rows'> {
  value: string;
  /** Minimum visible rows. */
  minRows?: number;
  /** Height cap in px; taller content scrolls. */
  maxHeight?: number;
  /**
   * Chat-style keys: Enter (and Ctrl/Cmd+Enter) calls this and inserts nothing; Shift+Enter
   * inserts a newline. Without it the textarea behaves natively. Keys are ignored while an IME
   * composition is active.
   */
  onSubmitKey?: () => void;
}

/** A textarea that grows with its content up to `maxHeight`, then scrolls. */
export default function AutoGrowTextarea({ value, minRows = 2, maxHeight = 240, onSubmitKey, onKeyDown, className = '', style, ...rest }: Props) {
  const ref = useRef<HTMLTextAreaElement>(null);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, maxHeight)}px`;
    el.style.overflowY = el.scrollHeight > maxHeight ? 'auto' : 'hidden';
  }, [value, maxHeight]);

  function keyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    onKeyDown?.(e);
    if (e.defaultPrevented || !onSubmitKey) return;
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      onSubmitKey();
    }
  }

  return <textarea {...rest} ref={ref} value={value} rows={minRows} onKeyDown={keyDown} className={`resize-none ${className}`} style={style} />;
}
