/**
 * Filesystem rules of case_run: confinement of repo_dir / spec_paths inside the
 * repo pool, and the env-file directory. Everything is realpath-based so a
 * symlink cannot lead out of its root.
 */
import * as fs from 'fs';
import * as path from 'path';

export const CASE_CODE_RE = /^TC-[A-Z0-9]+-[0-9]{3,4}$/;
/** Only automation env files are selectable: the dir also holds the executor's own secrets. */
export const ENV_FILE_RE = /^automation-[A-Za-z0-9_-]+\.env$/;
export const SPEC_ROOTS = ['tests/web/', 'tests/api/rest/'] as const;
export const MAX_SPECS = 5;

export class CaseError extends Error {}

/** True when `p` is `root` or lies below it (both already realpath'd). */
export function within(root: string, p: string): boolean {
  const rel = path.relative(root, p);
  return rel === '' || (rel !== '..' && !rel.startsWith(`..${path.sep}`) && !path.isAbsolute(rel));
}

function real(p: string, what: string): string {
  try {
    return fs.realpathSync(p);
  } catch {
    throw new CaseError(`${what} does not exist`);
  }
}

/** realpath of repo_dir, which must be absolute, inside the pool and hold playwright.config.ts. */
export function resolveRepoDir(repoDir: string, pool: string): string {
  if (!path.isAbsolute(repoDir) || repoDir.includes('\0')) throw new CaseError('repo_dir must be an absolute path');
  const realPool = real(pool, 'EXECUTOR_REPO_POOL');
  const realRepo = real(repoDir, 'repo_dir');
  if (realRepo === realPool || !within(realPool, realRepo)) throw new CaseError('repo_dir is outside the repo pool');
  const cfg = path.join(realRepo, 'playwright.config.ts');
  let ok = false;
  try {
    ok = within(realRepo, fs.realpathSync(cfg)) && fs.statSync(cfg).isFile();
  } catch {
    /* missing */
  }
  if (!ok) throw new CaseError('repo_dir has no playwright.config.ts');
  return realRepo;
}

/** Shape check only (no filesystem). Returns the path unchanged. */
export function checkSpecShape(spec: unknown): string {
  if (typeof spec !== 'string' || spec === '') throw new CaseError('spec_paths entries must be non-empty strings');
  if (spec.includes('\0') || spec.includes('\\') || path.isAbsolute(spec)) throw new CaseError(`invalid spec path: ${spec}`);
  const segs = spec.split('/');
  if (segs.some((s) => s === '' || s === '.' || s === '..')) throw new CaseError(`invalid spec path: ${spec}`);
  if (!spec.endsWith('.spec.ts')) throw new CaseError(`spec path must end with .spec.ts: ${spec}`);
  if (!SPEC_ROOTS.some((r) => spec.startsWith(r))) throw new CaseError(`spec path must be under ${SPEC_ROOTS.join(' or ')}: ${spec}`);
  if (segs.includes('mq')) throw new CaseError(`mq specs are not run: ${spec}`);
  return spec;
}

/** Each spec must exist as a regular file whose realpath stays inside the repo. */
export function resolveSpecs(realRepo: string, specs: string[]): string[] {
  return specs.map((s) => {
    const abs = real(path.join(realRepo, s), `spec ${s}`);
    if (!within(realRepo, abs)) throw new CaseError(`spec escapes the repo: ${s}`);
    if (!fs.statSync(abs).isFile()) throw new CaseError(`spec is not a file: ${s}`);
    // The symlink-resolved location must still be an allowed spec root and not mq.
    checkSpecShape(path.relative(realRepo, abs).split(path.sep).join('/'));
    return s;
  });
}

/** Names of selectable env files (regular, non-symlink files matching ENV_FILE_RE). */
export function listEnvFiles(envDir: string): string[] {
  let names: string[] = [];
  try {
    names = fs.readdirSync(envDir);
  } catch {
    return [];
  }
  return names
    .filter((n) => {
      if (!ENV_FILE_RE.test(n)) return false;
      try {
        return fs.lstatSync(path.join(envDir, n)).isFile();
      } catch {
        return false;
      }
    })
    .sort();
}

/** Parses dotenv text: KEY=VALUE, optional `export`, quotes, `#` comments. */
export function parseEnvFile(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  const lines = text.split(/\r?\n/);
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim();
    if (!line || line.startsWith('#')) continue;
    const m = /^(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$/.exec(line);
    if (!m) continue;
    let v = m[2];
    const q = v[0];
    if (q === '"' || q === "'") {
      // A quoted value may span lines (a PEM key): read on until the closing quote.
      while (v.lastIndexOf(q) === 0 && i + 1 < lines.length) {
        i++;
        v += '\n' + lines[i];
      }
      const end = v.lastIndexOf(q);
      if (end > 0) {
        v = v.slice(1, end);
        // Like dotenv, a double-quoted value expands \n, so a PEM fits on one line too.
        if (q === '"') v = v.replace(/\\n/g, '\n');
      }
    } else {
      v = v.replace(/\s+#.*$/, '').trim();
    }
    out[m[1]] = v;
  }
  return out;
}

/** Reads a chosen env file. Refuses non-listed names, symlinks and group/world-accessible files. */
export function loadEnvFile(envDir: string, name: string): Record<string, string> {
  if (!ENV_FILE_RE.test(name) || !listEnvFiles(envDir).includes(name)) throw new CaseError(`unknown env_file: ${name}`);
  const p = path.join(envDir, name);
  const st = fs.lstatSync(p);
  if (!st.isFile()) throw new CaseError(`env_file is not a regular file: ${name}`);
  if (st.mode & 0o077) throw new CaseError(`env_file ${name} must be mode 600`);
  return parseEnvFile(fs.readFileSync(p, 'utf8'));
}

export function hostAllowed(host: string, allowed: string[]): boolean {
  const h = host.toLowerCase();
  return allowed.some((a) => (a.startsWith('*.') ? h.endsWith(a.slice(1)) && h.length > a.length - 1 : h === a));
}

/**
 * Every http(s) URL among the env values must point at an allow-listed host.
 * An env file with no such URL is refused too: nothing would prove it is staging.
 */
export function checkEnvHosts(env: Record<string, string>, allowed: string[]): void {
  const hosts = new Set<string>();
  for (const v of Object.values(env)) {
    if (!/^https?:\/\//i.test(v)) continue;
    try {
      hosts.add(new URL(v).hostname);
    } catch {
      throw new CaseError('env file holds an unparsable http(s) URL');
    }
  }
  if (hosts.size === 0) throw new CaseError('env file has no http(s) base URL to check against the allow-list');
  const bad = [...hosts].filter((h) => !hostAllowed(h, allowed));
  if (bad.length) throw new CaseError(`host not on the staging allow-list: ${bad.join(', ')}`);
}
