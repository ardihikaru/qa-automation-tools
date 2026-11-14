import { randomUUID } from 'crypto';
import { EVENT_RING, RunEventLevel, RunRecord } from './types';
import { RunRequest } from './validate';

/**
 * Runs one job at a time. `signal` aborts when the run exceeds its timeout or
 * the service shuts down; handlers must close their browser when it fires.
 */
export type EmitEvent = (level: RunEventLevel, step: string, message: string) => void;

export type RunHandler = (run: RunRecord, signal: AbortSignal, emit: EmitEvent) => Promise<Record<string, unknown>>;

export interface QueueOptions {
  maxQueue: number;
  timeoutMs: number;
  ttlMs: number;
  log?: (msg: string, fields?: Record<string, unknown>) => void;
}

export class RunQueue {
  private runs = new Map<string, RunRecord>();
  private pending: string[] = [];
  private active: { id: string; ctrl: AbortController } | null = null;
  private stopped = false;
  private eventSeq = new Map<string, number>();

  constructor(private handler: RunHandler, private opts: QueueOptions) {}

  /** Returns the queued record, or null when the queue is full or stopped. */
  submit(req: RunRequest): RunRecord | null {
    if (this.stopped) return null;
    const waiting = this.pending.length;
    if (this.active && waiting >= this.opts.maxQueue) return null;
    this.prune();
    const run: RunRecord = {
      id: `run-${randomUUID()}`,
      job_id: req.job_id,
      type: req.type,
      params: req.params,
      status: 'queued',
      events: [],
      created_at: new Date().toISOString(),
    };
    this.runs.set(run.id, run);
    this.pending.push(run.id);
    queueMicrotask(() => void this.pump());
    return run;
  }

  /** The record; with `since`, a copy holding only events whose n > since. */
  get(id: string, since?: number): RunRecord | undefined {
    const r = this.runs.get(id);
    if (!r || since === undefined) return r;
    return { ...r, events: r.events.filter((e) => e.n > since) };
  }

  /** Appends an event to the run's ring (last EVENT_RING kept). */
  emit(run: RunRecord, level: RunEventLevel, step: string, message: string): void {
    const n = (this.eventSeq.get(run.id) ?? 0) + 1;
    this.eventSeq.set(run.id, n);
    run.events.push({ n, at: new Date().toISOString(), level, step, message });
    if (run.events.length > EVENT_RING) run.events.splice(0, run.events.length - EVENT_RING);
  }

  stats(): { busy: boolean; queued: number; active?: string } {
    return { busy: this.active !== null, queued: this.pending.length, active: this.active?.id };
  }

  /** Stop accepting runs and abort the active one. Resolves when it finished. */
  async stop(): Promise<void> {
    this.stopped = true;
    for (const id of this.pending.splice(0)) {
      const r = this.runs.get(id);
      if (r) this.finish(r, 'failed', undefined, 'executor shutting down');
    }
    const a = this.active;
    if (a) {
      a.ctrl.abort(new Error('executor shutting down'));
      while (this.active) await new Promise((r) => setTimeout(r, 50));
    }
  }

  private async pump(): Promise<void> {
    if (this.active || this.stopped) return;
    const id = this.pending.shift();
    if (!id) return;
    const run = this.runs.get(id);
    if (!run) return void this.pump();

    const ctrl = new AbortController();
    this.active = { id, ctrl };
    run.status = 'running';
    run.started_at = new Date().toISOString();
    this.opts.log?.('run started', { id, type: run.type, job_id: run.job_id });

    const timer = setTimeout(
      () => ctrl.abort(new Error(`run exceeded ${Math.round(this.opts.timeoutMs / 1000)}s timeout`)),
      this.opts.timeoutMs,
    );
    const work = Promise.resolve().then(() => this.handler(run, ctrl.signal, (l, st, m) => this.emit(run, l, st, m)));
    try {
      const aborted = new Promise<never>((_, reject) => {
        ctrl.signal.addEventListener('abort', () => reject(ctrl.signal.reason), { once: true });
      });
      const result = await Promise.race([work, aborted]);
      this.finish(run, 'completed', result);
    } catch (err) {
      this.finish(run, 'failed', undefined, err instanceof Error ? err.message : String(err));
      // Never start the next browser while the aborted one may still be alive:
      // wait for the handler to unwind (it closes its browser on abort), bounded.
      await Promise.race([work.catch(() => undefined), new Promise((r) => setTimeout(r, 30_000))]);
    } finally {
      clearTimeout(timer);
      this.active = null;
      void this.pump();
    }
  }

  private finish(run: RunRecord, status: 'completed' | 'failed', result?: Record<string, unknown>, error?: string) {
    run.status = status;
    run.result = result;
    run.error = error;
    run.completed_at = new Date().toISOString();
    if (run.started_at) run.duration_ms = Date.parse(run.completed_at) - Date.parse(run.started_at);
    this.opts.log?.('run finished', { id: run.id, status, duration_ms: run.duration_ms, error });
  }

  private prune(): void {
    const cutoff = Date.now() - this.opts.ttlMs;
    for (const [id, r] of this.runs) {
      if (r.completed_at && Date.parse(r.completed_at) < cutoff) {
        this.runs.delete(id);
        this.eventSeq.delete(id);
      }
    }
  }
}
