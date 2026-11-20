// A small, safe markdown renderer for synced repo files (agents, skills, rules, CLAUDE.md).
// It builds React elements only: raw HTML in the source is shown as text, and links are
// limited to http(s), mailto, in-page anchors and repo-relative paths resolved against `base`.
// Covers what these files use: headings, paragraphs, lists (nested, task items), fenced code,
// block quotes, tables, rules, and inline code, emphasis, strike-through and links.
import { Fragment, ReactNode, useMemo } from 'react';

// ---------------------------------------------------------------- block model

type Block =
  | { t: 'h'; level: number; text: string; id: string }
  | { t: 'p'; text: string }
  | { t: 'code'; lang: string; text: string }
  | { t: 'quote'; children: Block[] }
  | { t: 'list'; ordered: boolean; start: number; items: { task: boolean | null; children: Block[] }[] }
  | { t: 'table'; align: ('left' | 'center' | 'right' | null)[]; head: string[]; rows: string[][] }
  | { t: 'hr' };

const FENCE_RE = /^(\s{0,3})(`{3,}|~{3,})\s*([^`\s]*)/;
const HEADING_RE = /^\s{0,3}(#{1,6})\s+(.*?)\s*#*\s*$/;
const HR_RE = /^\s{0,3}([-*_])(\s*\1){2,}\s*$/;
const QUOTE_RE = /^\s{0,3}>\s?/;
const LIST_RE = /^(\s*)([-*+]|\d{1,9}[.)])\s+(.*)$/;
const TABLE_SEP_RE = /^\s*\|?\s*:?-{1,}:?\s*(\|\s*:?-{1,}:?\s*)*\|?\s*$/;

const indentOf = (s: string) => s.match(/^\s*/)?.[0].replace(/\t/g, '    ').length ?? 0;
const isBlank = (s: string) => s.trim() === '';

function slugify(s: string): string {
  return s
    .toLowerCase()
    .replace(/[`*_~[\]()]/g, '')
    .replace(/[^\p{L}\p{N}\s-]/gu, '')
    .trim()
    .replace(/\s+/g, '-');
}

function splitRow(line: string): string[] {
  let s = line.trim();
  if (s.startsWith('|')) s = s.slice(1);
  if (s.endsWith('|') && !s.endsWith('\\|')) s = s.slice(0, -1);
  const cells: string[] = [];
  let cur = '';
  let inCode = false;
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (c === '\\' && s[i + 1] === '|') {
      cur += '|';
      i++;
    } else if (c === '`') {
      inCode = !inCode;
      cur += c;
    } else if (c === '|' && !inCode) {
      cells.push(cur.trim());
      cur = '';
    } else cur += c;
  }
  cells.push(cur.trim());
  return cells;
}

/** True when `line` starts a block that interrupts a paragraph. */
function startsBlock(line: string, next: string | undefined): boolean {
  return (
    FENCE_RE.test(line) ||
    HEADING_RE.test(line) ||
    HR_RE.test(line) ||
    QUOTE_RE.test(line) ||
    LIST_RE.test(line) ||
    (line.includes('|') && next !== undefined && TABLE_SEP_RE.test(next) && next.includes('-'))
  );
}

function dedent(line: string, n: number): string {
  const expanded = line.replace(/^\t+/, (t) => '    '.repeat(t.length));
  let i = 0;
  while (i < n && expanded[i] === ' ') i++;
  return expanded.slice(i);
}

function parseBlocks(lines: string[], ids: Map<string, number>): Block[] {
  const out: Block[] = [];
  let i = 0;
  const n = lines.length;
  while (i < n) {
    const line = lines[i];
    if (isBlank(line)) {
      i++;
      continue;
    }

    const fence = FENCE_RE.exec(line);
    if (fence) {
      const marker = fence[2];
      const body: string[] = [];
      i++;
      while (i < n) {
        const t = lines[i].trim();
        if (t.startsWith(marker[0].repeat(marker.length)) && /^(`+|~+)$/.test(t) && t[0] === marker[0]) {
          i++;
          break;
        }
        body.push(dedent(lines[i], fence[1].length));
        i++;
      }
      out.push({ t: 'code', lang: fence[3] || '', text: body.join('\n') });
      continue;
    }

    const h = HEADING_RE.exec(line);
    if (h) {
      const base = slugify(h[2]) || 'section';
      const seen = ids.get(base) ?? 0;
      ids.set(base, seen + 1);
      out.push({ t: 'h', level: h[1].length, text: h[2], id: seen ? `${base}-${seen}` : base });
      i++;
      continue;
    }

    if (HR_RE.test(line)) {
      out.push({ t: 'hr' });
      i++;
      continue;
    }

    if (QUOTE_RE.test(line)) {
      const inner: string[] = [];
      while (i < n && !isBlank(lines[i]) && (QUOTE_RE.test(lines[i]) || inner.length > 0)) {
        if (!QUOTE_RE.test(lines[i]) && startsBlock(lines[i], lines[i + 1])) break;
        inner.push(lines[i].replace(QUOTE_RE, ''));
        i++;
      }
      out.push({ t: 'quote', children: parseBlocks(inner, ids) });
      continue;
    }

    if (line.includes('|') && i + 1 < n && TABLE_SEP_RE.test(lines[i + 1]) && lines[i + 1].includes('-')) {
      const head = splitRow(line);
      const align = splitRow(lines[i + 1]).map((c) => {
        const l = c.startsWith(':');
        const r = c.endsWith(':');
        return l && r ? 'center' : r ? 'right' : l ? 'left' : null;
      });
      i += 2;
      const rows: string[][] = [];
      while (i < n && !isBlank(lines[i]) && lines[i].includes('|')) {
        rows.push(splitRow(lines[i]));
        i++;
      }
      out.push({ t: 'table', align, head, rows });
      continue;
    }

    const li = LIST_RE.exec(line);
    if (li) {
      const baseIndent = indentOf(li[1]);
      const ordered = /\d/.test(li[2]);
      const start = ordered ? parseInt(li[2], 10) : 1;
      const items: { task: boolean | null; children: Block[] }[] = [];
      while (i < n) {
        const m = LIST_RE.exec(lines[i]);
        if (!m || indentOf(m[1]) > baseIndent + 1 || indentOf(m[1]) < baseIndent || /\d/.test(m[2]) !== ordered) break;
        const contentIndent = indentOf(m[1]) + m[2].length + 1;
        let first = m[3];
        let task: boolean | null = null;
        const tm = /^\[([ xX])\]\s+(.*)$/.exec(first);
        if (tm) {
          task = tm[1] !== ' ';
          first = tm[2];
        }
        const content = [first];
        i++;
        while (i < n) {
          const cur = lines[i];
          if (isBlank(cur)) {
            // A blank line continues the item only if the next text is indented under it.
            let j = i;
            while (j < n && isBlank(lines[j])) j++;
            if (j < n && indentOf(lines[j]) >= baseIndent + 2) {
              for (; i < j; i++) content.push('');
              continue;
            }
            break;
          }
          const ind = indentOf(cur);
          if (ind >= baseIndent + 2) {
            content.push(dedent(cur, Math.min(ind, contentIndent)));
            i++;
            continue;
          }
          // Lazy continuation of the item's paragraph.
          if (!startsBlock(cur, lines[i + 1]) && !isBlank(content[content.length - 1])) {
            content.push(cur.trim());
            i++;
            continue;
          }
          break;
        }
        items.push({ task, children: parseBlocks(content, ids) });
        // A blank line between items keeps the list going.
        let j = i;
        while (j < n && isBlank(lines[j])) j++;
        const nm = j < n ? LIST_RE.exec(lines[j]) : null;
        if (nm && indentOf(nm[1]) >= baseIndent && indentOf(nm[1]) <= baseIndent + 1 && /\d/.test(nm[2]) === ordered) i = j;
      }
      out.push({ t: 'list', ordered, start, items });
      continue;
    }

    const para: string[] = [line.trim()];
    i++;
    while (i < n && !isBlank(lines[i]) && !startsBlock(lines[i], lines[i + 1])) {
      para.push(lines[i].trim());
      i++;
    }
    out.push({ t: 'p', text: para.join('\n') });
  }
  return out;
}

// ---------------------------------------------------------------- inline

/** Returns a safe href, or null when the target must be shown as text. */
function safeHref(raw: string, base?: string | null, idPrefix = ''): string | null {
  const url = raw.trim().replace(/^<|>$/g, '');
  if (!url) return null;
  if (/^(https?:|mailto:)/i.test(url)) return url;
  if (url.startsWith('#')) return `#md-${idPrefix}${slugify(decodeURIComponent(url.slice(1)))}`;
  // Any other scheme (javascript:, data:, vbscript:, file:) and protocol-relative URLs are refused.
  if (/^[a-z][a-z0-9+.-]*:/i.test(url) || url.startsWith('//')) return null;
  if (!base) return null;
  try {
    const u = new URL(url, base);
    return /^https?:$/.test(u.protocol) ? u.href : null;
  } catch {
    return null;
  }
}

interface InlineCtx {
  base?: string | null;
  /** Prefix of heading ids, so several documents on one page cannot collide. */
  idPrefix?: string;
  /** Show each external link's host after it (text from a model: the reader sees where a link goes). */
  showLinkHost?: boolean;
}

function hostOf(href: string): string | null {
  try {
    const u = new URL(href);
    return /^https?:$/.test(u.protocol) ? u.host : null;
  } catch {
    return null;
  }
}

function findClosing(s: string, from: number, open: string, close: string): number {
  let depth = 0;
  for (let i = from; i < s.length; i++) {
    if (s[i] === '\\') {
      i++;
      continue;
    }
    if (s[i] === open) depth++;
    else if (s[i] === close) {
      if (depth === 0) return i;
      depth--;
    }
  }
  return -1;
}

const PUNCT = /[!"#$%&'()*+,\-./:;<=>?@[\\\]^_`{|}~]/;
const BARE_URL_RE = /^https?:\/\/[^\s<>"]*[^\s<>".,:;'")\]!?]/;

function renderLink(label: ReactNode, href: string | null, key: string, title?: string, showHost = false): ReactNode {
  if (!href) return <Fragment key={key}>{label}</Fragment>;
  const external = !href.startsWith('#');
  const host = showHost && external ? hostOf(href) : null;
  if (host) {
    return (
      <Fragment key={key}>
        {renderLink(label, href, `${key}a`, title)}
        <span className="ml-1 text-xs text-gray-500">({host})</span>
      </Fragment>
    );
  }
  return (
    <a
      key={key}
      href={href}
      title={title}
      className="text-primary-700 underline decoration-primary-300 underline-offset-2 hover:decoration-primary-700 break-words"
      {...(external ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
    >
      {label}
    </a>
  );
}

function inline(s: string, ctx: InlineCtx, kp = 'i'): ReactNode[] {
  const out: ReactNode[] = [];
  let buf = '';
  let k = 0;
  const key = () => `${kp}${k++}`;
  const flush = () => {
    if (buf) {
      out.push(buf);
      buf = '';
    }
  };
  let i = 0;
  while (i < s.length) {
    const c = s[i];

    if (c === '\\' && i + 1 < s.length && PUNCT.test(s[i + 1])) {
      buf += s[i + 1];
      i += 2;
      continue;
    }
    if (c === '\n') {
      buf += ' ';
      i++;
      continue;
    }

    if (c === '`') {
      let run = 1;
      while (s[i + run] === '`') run++;
      const fence = '`'.repeat(run);
      let j = s.indexOf(fence, i + run);
      while (j >= 0 && s[j + run] === '`') j = s.indexOf(fence, j + run + 1);
      if (j > 0) {
        flush();
        let code = s.slice(i + run, j).replace(/\n/g, ' ');
        if (code.startsWith(' ') && code.endsWith(' ') && code.trim()) code = code.slice(1, -1);
        out.push(
          <code key={key()} className="rounded bg-gray-100 px-1 py-0.5 font-mono text-[0.85em] text-gray-800 break-words">
            {code}
          </code>,
        );
        i = j + run;
        continue;
      }
      buf += fence;
      i += run;
      continue;
    }

    if (c === '[' || (c === '!' && s[i + 1] === '[')) {
      const img = c === '!';
      const open = img ? i + 1 : i;
      const close = findClosing(s, open + 1, '[', ']');
      if (close > 0 && s[close + 1] === '(') {
        const end = findClosing(s, close + 2, '(', ')');
        if (end > 0) {
          const target = s.slice(close + 2, end).trim();
          const tm = /^(\S+)(?:\s+["'(](.*)["')])?$/.exec(target);
          const label = s.slice(open + 1, close);
          flush();
          const href = tm ? safeHref(tm[1], ctx.base, ctx.idPrefix) : null;
          const content = img ? [`Image: ${label || tm?.[1] || ''}`] : inline(label, ctx, `${kp}${k}l`);
          out.push(renderLink(content, href, key(), tm?.[2], ctx.showLinkHost && !img));
          i = end + 1;
          continue;
        }
      }
    }

    if (c === '<') {
      const m = /^<((?:https?:\/\/|mailto:)[^>\s]+)>/i.exec(s.slice(i));
      if (m) {
        flush();
        out.push(renderLink(m[1], safeHref(m[1]), key(), undefined, ctx.showLinkHost));
        i += m[0].length;
        continue;
      }
    }

    if ((c === 'h' || c === 'H') && (i === 0 || /[\s(]/.test(s[i - 1]))) {
      const m = BARE_URL_RE.exec(s.slice(i));
      if (m) {
        flush();
        out.push(renderLink(m[0], safeHref(m[0]), key(), undefined, ctx.showLinkHost));
        i += m[0].length;
        continue;
      }
    }

    if (c === '~' && s[i + 1] === '~') {
      const j = s.indexOf('~~', i + 2);
      if (j > i + 2) {
        flush();
        out.push(
          <del key={key()} className="text-gray-500">
            {inline(s.slice(i + 2, j), ctx, `${kp}${k}d`)}
          </del>,
        );
        i = j + 2;
        continue;
      }
    }

    if (c === '*' || c === '_') {
      const prev = i > 0 ? s[i - 1] : ' ';
      const wordy = c === '_' && /[\p{L}\p{N}]/u.test(prev);
      if (!wordy) {
        const dbl = s[i + 1] === c;
        const mark = dbl ? c + c : c;
        const from = i + mark.length;
        if (s[from] && !/\s/.test(s[from])) {
          let j = s.indexOf(mark, from + 1);
          while (j > 0 && (/\s/.test(s[j - 1]) || (!dbl && s[j + 1] === c) || (c === '_' && /[\p{L}\p{N}]/u.test(s[j + mark.length] ?? '')))) {
            j = s.indexOf(mark, j + 1);
          }
          if (j > from) {
            flush();
            const inner = inline(s.slice(from, j), ctx, `${kp}${k}e`);
            out.push(
              dbl ? (
                <strong key={key()} className="font-semibold text-gray-900">
                  {inner}
                </strong>
              ) : (
                <em key={key()}>{inner}</em>
              ),
            );
            i = j + mark.length;
            continue;
          }
        }
      }
    }

    buf += c;
    i++;
  }
  flush();
  return out;
}

// ---------------------------------------------------------------- render

const H_CLASS = [
  'text-xl font-semibold text-gray-900 mt-8 first:mt-0 pb-1 border-b border-gray-200',
  'text-lg font-semibold text-gray-900 mt-7 first:mt-0',
  'text-base font-semibold text-gray-900 mt-6 first:mt-0',
  'text-sm font-semibold text-gray-900 mt-5 first:mt-0',
  'text-sm font-medium text-gray-800 mt-4 first:mt-0',
  'text-sm font-medium text-gray-600 mt-4 first:mt-0',
];

// Prose stays at a readable measure; tables and code may use the full column.
const MEASURE = 'max-w-[85ch]';

function renderBlocks(blocks: Block[], ctx: InlineCtx, kp: string, tight = false): ReactNode[] {
  return blocks.map((b, idx) => {
    const key = `${kp}-${idx}`;
    switch (b.t) {
      case 'h': {
        // The page title is the h1, so markdown headings start at h2.
        const Tag = `h${Math.min(6, b.level + 1)}` as 'h2';
        return (
          <Tag key={key} id={`md-${ctx.idPrefix ?? ''}${b.id}`} className={`${H_CLASS[b.level - 1]} ${MEASURE} scroll-mt-4 break-words`}>
            {inline(b.text, ctx, key)}
          </Tag>
        );
      }
      case 'p':
        return tight ? (
          <Fragment key={key}>{inline(b.text, ctx, key)}</Fragment>
        ) : (
          <p key={key} className={`${MEASURE} break-words`}>
            {inline(b.text, ctx, key)}
          </p>
        );
      case 'code':
        return (
          <figure key={key} className="relative min-w-0">
            {b.lang && (
              <figcaption className="absolute right-2 top-1.5 text-[11px] text-gray-400 font-mono select-none" aria-label={`${b.lang} code`}>
                {b.lang}
              </figcaption>
            )}
            <pre className="overflow-x-auto rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 font-mono text-[13px] leading-relaxed text-gray-800">
              <code>{b.text}</code>
            </pre>
          </figure>
        );
      case 'quote':
        return (
          <blockquote key={key} className={`${MEASURE} border-l-2 border-gray-300 pl-4 text-gray-600 space-y-3`}>
            {renderBlocks(b.children, ctx, key)}
          </blockquote>
        );
      case 'hr':
        return <hr key={key} className="border-gray-200" />;
      case 'list': {
        const ListTag = b.ordered ? 'ol' : 'ul';
        const hasTasks = b.items.some((it) => it.task !== null);
        return (
          <ListTag
            key={key}
            start={b.ordered && b.start !== 1 ? b.start : undefined}
            className={`${MEASURE} space-y-1 ${hasTasks ? 'pl-1 list-none' : b.ordered ? 'pl-6 list-decimal' : 'pl-5 list-disc'} marker:text-gray-400`}
          >
            {b.items.map((it, j) => {
              const single = it.children.length === 1 && it.children[0].t === 'p';
              return (
                <li key={j} className="break-words pl-0.5">
                  {it.task !== null && (
                    <span
                      className={`mr-2 inline-flex h-3.5 w-3.5 translate-y-0.5 items-center justify-center rounded border text-[10px] leading-none ${it.task ? 'border-primary-600 bg-primary-600 text-white' : 'border-gray-400 bg-white'}`}
                      role="img"
                      aria-label={it.task ? 'Done' : 'Not done'}
                    >
                      {it.task ? '✓' : ''}
                    </span>
                  )}
                  {single ? renderBlocks(it.children, ctx, `${key}-${j}`, true) : <div className="space-y-2">{renderBlocks(it.children, ctx, `${key}-${j}`)}</div>}
                </li>
              );
            })}
          </ListTag>
        );
      }
      case 'table':
        return (
          <div key={key} className="overflow-x-auto rounded-lg border border-gray-200">
            <table className="min-w-full text-sm">
              <thead className="bg-gray-50">
                <tr>
                  {b.head.map((h, j) => (
                    <th key={j} className="px-3 py-2 font-medium text-gray-700 border-b border-gray-200 align-bottom" style={{ textAlign: b.align[j] ?? 'left' }}>
                      {inline(h, ctx, `${key}h${j}`)}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {b.rows.map((r, ri) => (
                  <tr key={ri}>
                    {b.head.map((_, j) => (
                      <td key={j} className="px-3 py-2 align-top text-gray-700 break-words" style={{ textAlign: b.align[j] ?? 'left' }}>
                        {inline(r[j] ?? '', ctx, `${key}r${ri}c${j}`)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        );
    }
    return null;
  });
}

function parseMarkdown(src: string): Block[] {
  return parseBlocks(src.replace(/\r\n?/g, '\n').split('\n'), new Map());
}

/**
 * Renders `source` as markdown. `base` resolves repo-relative links (the file's GitHub URL);
 * `idPrefix` scopes heading ids; `showLinkHost` prints each external link's host after it;
 * `empty` is shown when there is nothing to render.
 */
export default function Markdown({
  source,
  base,
  idPrefix,
  showLinkHost,
  empty = 'This file has no content after its frontmatter.',
}: {
  source: string;
  base?: string | null;
  idPrefix?: string;
  showLinkHost?: boolean;
  empty?: string;
}) {
  const blocks = useMemo(() => parseMarkdown(source), [source]);
  if (!blocks.length) return <p className="text-sm text-gray-500">{empty}</p>;
  return <div className="space-y-4 text-sm leading-relaxed text-gray-800 min-w-0">{renderBlocks(blocks, { base, idPrefix, showLinkHost }, 'b')}</div>;
}
