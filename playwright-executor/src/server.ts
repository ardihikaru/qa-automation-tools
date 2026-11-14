/**
 * playwright-executor HTTP API.
 *
 *   GET  /health        public: status, queue, Chromium, cgroup memory
 *   POST /v1/runs       bearer: enqueue {job_id, type, params}  -> 202 {id, status}
 *   GET  /v1/runs/:id   bearer: run status/result; ?since=n returns only events with n > since
 *
 * The executor never calls ai-backend or any LLM: the backend submits runs and
 * polls for results.
 */
import * as fs from 'fs';
import * as http from 'http';
import * as path from 'path';
import { chromium } from 'playwright';
import { checkBearer } from './auth';
import { launchBrowser } from './browser';
import { ExecutorConfig, loadConfig } from './config';
import { EmitEvent, RunQueue } from './queue';
import { docsGenerate } from './runs/docsGenerate';
import { qaRun } from './runs/qaRun';
import { caseRun } from './runs/caseRun';
import { CaseRunParams, RunParams, RunRecord } from './types';
import { parseRunRequest } from './validate';

const MAX_BODY = 64 * 1024;

function log(msg: string, fields: Record<string, unknown> = {}): void {
  process.stdout.write(`${JSON.stringify({ ts: new Date().toISOString(), msg, ...fields })}\n`);
}

/** Memory of this service's cgroup (the systemd unit), in MiB. */
function cgroupMemory(): { current_mb?: number; max_mb?: number; peak_mb?: number } {
  try {
    const rel = fs.readFileSync('/proc/self/cgroup', 'utf8').trim().split('\n').find((l) => l.startsWith('0::'))?.slice(3);
    if (!rel) return {};
    const dir = path.join('/sys/fs/cgroup', rel);
    const read = (f: string) => {
      try {
        const v = fs.readFileSync(path.join(dir, f), 'utf8').trim();
        return v === 'max' ? undefined : Math.round(Number(v) / 1048576);
      } catch {
        return undefined;
      }
    };
    return { current_mb: read('memory.current'), max_mb: read('memory.max'), peak_mb: read('memory.peak') };
  } catch {
    return {};
  }
}

function pruneArtifacts(cfg: ExecutorConfig): void {
  const cutoff = Date.now() - cfg.retentionDays * 86_400_000;
  let entries: string[] = [];
  try {
    entries = fs.readdirSync(cfg.outputDir);
  } catch {
    return;
  }
  for (const e of entries) {
    const p = path.join(cfg.outputDir, e);
    try {
      if (fs.statSync(p).mtimeMs < cutoff) fs.rmSync(p, { recursive: true, force: true });
    } catch {
      /* ignore */
    }
  }
}

function makeHandler(cfg: ExecutorConfig) {
  return async (run: RunRecord, signal: AbortSignal, emit: EmitEvent) => {
    // case_run drives child processes in the repo pool, not an executor-owned browser.
    if (run.type === 'case_run') return caseRun({ cfg, signal, runId: run.id, emit }, run.params as CaseRunParams);
    pruneArtifacts(cfg);
    const outDir = path.join(cfg.outputDir, run.id);
    fs.mkdirSync(outDir, { recursive: true });
    const browser = await launchBrowser(cfg.headless, signal);
    try {
      const ctx = { cfg, browser, signal, runId: run.id, outDir };
      return run.type === 'qa_run' ? await qaRun(ctx, run.params as RunParams) : await docsGenerate(ctx, run.params as RunParams);
    } finally {
      await browser.close().catch(() => undefined);
    }
  };
}

function send(res: http.ServerResponse, status: number, body: unknown): void {
  const data = JSON.stringify(body);
  res.writeHead(status, { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(data) });
  res.end(data);
}

function readJson(req: http.IncomingMessage): Promise<unknown> {
  return new Promise((resolve, reject) => {
    let size = 0;
    const chunks: Buffer[] = [];
    req.on('data', (c: Buffer) => {
      size += c.length;
      if (size > MAX_BODY) {
        reject(new Error('body too large'));
        req.destroy();
        return;
      }
      chunks.push(c);
    });
    req.on('end', () => {
      try {
        resolve(chunks.length ? JSON.parse(Buffer.concat(chunks).toString('utf8')) : {});
      } catch {
        reject(new Error('invalid JSON'));
      }
    });
    req.on('error', reject);
  });
}

export function createServer(cfg: ExecutorConfig, queue: RunQueue): http.Server {
  const chromiumPath = chromium.executablePath();
  const chromiumInstalled = fs.existsSync(chromiumPath);
  const knownRoles = Object.keys(cfg.credentials);

  return http.createServer(async (req, res) => {
    const url = new URL(req.url ?? '/', 'http://x');
    try {
      if (url.pathname === '/health' && req.method === 'GET') {
        return send(res, chromiumInstalled ? 200 : 503, {
          status: chromiumInstalled ? 'ok' : 'degraded',
          service: 'playwright-executor',
          ...queue.stats(),
          chromium: chromiumInstalled ? 'installed' : 'missing',
          roles: knownRoles.length,
          memory: cgroupMemory(),
          rss_mb: Math.round(process.memoryUsage().rss / 1048576),
        });
      }

      if (!url.pathname.startsWith('/v1/')) return send(res, 404, { error: 'not found' });
      if (!checkBearer(req.headers.authorization, cfg.token)) return send(res, 401, { error: 'unauthorized' });

      if (url.pathname === '/v1/runs' && req.method === 'POST') {
        let body: unknown;
        try {
          body = await readJson(req);
        } catch (err) {
          return send(res, 400, { error: (err as Error).message });
        }
        const parsed = parseRunRequest(body, knownRoles, cfg.defaultRoles, { repoPool: cfg.repoPool, envDir: cfg.envDir });
        if (!parsed.ok) return send(res, 400, { error: parsed.error });
        const run = queue.submit(parsed.value);
        if (!run) return send(res, 429, { error: 'queue full', ...queue.stats() });
        log('run queued', { id: run.id, type: run.type, job_id: run.job_id, roles: (run.params as RunParams).roles });
        return send(res, 202, { id: run.id, status: run.status, job_id: run.job_id });
      }

      const m = /^\/v1\/runs\/([A-Za-z0-9-]+)$/.exec(url.pathname);
      if (m && req.method === 'GET') {
        const sinceRaw = url.searchParams.get('since');
        let since: number | undefined;
        if (sinceRaw !== null) {
          if (!/^\d{1,12}$/.test(sinceRaw)) return send(res, 400, { error: 'since must be a non-negative integer' });
          since = Number(sinceRaw);
        }
        const run = queue.get(m[1], since);
        return run ? send(res, 200, run) : send(res, 404, { error: 'run not found' });
      }
      return send(res, 404, { error: 'not found' });
    } catch (err) {
      log('request error', { path: url.pathname, error: (err as Error).message });
      if (!res.headersSent) send(res, 500, { error: 'internal error' });
    }
  });
}

async function main(): Promise<void> {
  const cfg = loadConfig();
  fs.mkdirSync(cfg.outputDir, { recursive: true });
  const queue = new RunQueue(makeHandler(cfg), {
    maxQueue: cfg.maxQueue,
    timeoutMs: cfg.runTimeoutMs,
    ttlMs: 24 * 3_600_000,
    log,
  });
  const server = createServer(cfg, queue);
  server.listen(cfg.port, cfg.bind, () =>
    log('listening', {
      bind: cfg.bind,
      port: cfg.port,
      staging: new URL(cfg.stagingUrl).host,
      roles: Object.keys(cfg.credentials).length,
      default_roles: cfg.defaultRoles,
      memory: cgroupMemory(),
    }),
  );

  const shutdown = async (sig: string) => {
    log('shutting down', { signal: sig });
    server.close();
    await queue.stop();
    process.exit(0);
  };
  process.on('SIGTERM', () => void shutdown('SIGTERM'));
  process.on('SIGINT', () => void shutdown('SIGINT'));
}

if (require.main === module) {
  main().catch((err) => {
    log('fatal', { error: (err as Error).message });
    process.exit(1);
  });
}
