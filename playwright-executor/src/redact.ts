/** Redaction of child-process output before it becomes a run event. */

export const MAX_LINE = 300;
const MIN_SECRET_LEN = 4;
const BENIGN = new Set(['true', 'false', 'null', 'none', 'info', 'warn', 'debug']);

// ANSI escape sequences and other control characters (tab and newline are not kept either).
// eslint-disable-next-line no-control-regex
const ANSI = /\u001b\[[0-9;?]*[ -/]*[@-~]/g;
// eslint-disable-next-line no-control-regex
const CTRL = /[\u0000-\u001f\u007f]/g;
const PATTERNS: Array<[RegExp, string]> = [
  [/\bBearer\s+[A-Za-z0-9._~+/=-]+/gi, 'Bearer ***'],
  [/\bgh[pousr]_[A-Za-z0-9]{8,}/g, '***'],
  [/\bgithub_pat_[A-Za-z0-9_]{8,}/g, '***'],
  [/\bgaos_[A-Za-z0-9_-]{4,}/gi, '***'],
  [/(\b[a-z][a-z0-9+.-]*:\/\/)[^\s/@:]*(?::[^\s/@]*)?@/gi, '$1***@'],
];

/** Values from the env file worth masking verbatim (longest first). */
export function secretValues(env: Record<string, string>): string[] {
  return [...new Set(Object.values(env))]
    .filter((v) => v.length >= MIN_SECRET_LEN && !BENIGN.has(v.toLowerCase()))
    .sort((a, b) => b.length - a.length);
}

export function redact(line: string, secrets: string[]): string {
  let s = line.replace(ANSI, '').replace(CTRL, ' ');
  for (const v of secrets) s = s.split(v).join('***');
  for (const [re, to] of PATTERNS) s = s.replace(re, to);
  return s;
}

/** Redacts first, then clips, so a cut can never leave a secret prefix behind. */
export function redactLine(line: string, secrets: string[]): string {
  const s = redact(line, secrets);
  return s.length > MAX_LINE ? `${s.slice(0, MAX_LINE - 1)}…` : s;
}
