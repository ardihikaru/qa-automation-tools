'use strict';
/**
 * Brain Town reporter. Posts one run (one spec file, executed once) to Brain Town as
 * qa.run.started, one qa.step per top-level test.step, and qa.run.finished, so the QA Lab can show
 * a QA bot walking through the spec. Brain Town never calls back.
 *
 * Settings (env, because case_run selects reporters with --reporter, which cannot carry options;
 * constructor options of a playwright.config.ts reporter entry win over env):
 *   BRAIN_TOWN_URL     ingest endpoint; unset means the reporter does nothing at all
 *   BRAIN_TOWN_TOKEN   sent as "Authorization: Bearer <token>"
 *   BRAIN_TOWN_PRODUCT product key (option `product`)
 *   BRAIN_TOWN_ORG     optional GitHub org (option `org`)
 *   VENDOR_E2E_RUN     run id shared by every event of the execution (fallback run-<epoch>)
 *
 * Never fails or slows a run: 2 s timeout per request, errors swallowed, no retries. After the first
 * failed request the rest are dropped, so an unreachable Brain Town costs at most one timeout.
 * onEnd awaits the queue so the last event is not lost. Sends titles, statuses and counts only:
 * no page content, screenshots, credentials or error text.
 *
 * Plain CommonJS with no dependency, like nav-reporter.cjs.
 */
const fs = require('fs');
const path = require('path');

const TIMEOUT_MS = 2000;

class BrainTownReporter {
  constructor(options) {
    const o = options || {};
    const env = process.env;
    this.url = String(env.BRAIN_TOWN_URL || '').trim();
    this.token = String(env.BRAIN_TOWN_TOKEN || '').trim();
    this.product = String(o.product || env.BRAIN_TOWN_PRODUCT || '').trim();
    this.org = String(o.org || env.BRAIN_TOWN_ORG || '').trim();
    this.run = String(env.VENDOR_E2E_RUN || `run-${Math.floor(Date.now() / 1000)}`);
    this.enabled = this.url !== '' && typeof fetch === 'function';
    this.rootDir = '';
    this.seq = 0;
    this.dead = false;
    this.chain = Promise.resolve();
    this.specs = new Map(); // file -> {name, total, done, passed, failed, started, steps, t0}
  }

  printsToStdio() {
    return false;
  }

  onBegin(config, suite) {
    if (!this.enabled) return;
    try {
      this.rootDir = (config && config.configFile && path.dirname(config.configFile)) || (config && config.rootDir) || '';
      for (const t of suite.allTests()) {
        const s = this.spec(t);
        s.total += 1;
      }
    } catch {
      /* keep going */
    }
  }

  spec(test) {
    const file = (test.location && test.location.file) || 'unknown';
    let s = this.specs.get(file);
    if (!s) {
      let name = file;
      try {
        name = this.rootDir ? path.relative(this.rootDir, file) : path.basename(file);
      } catch {
        /* keep the file */
      }
      s = { name, total: 0, done: 0, passed: 0, failed: 0, started: false, steps: 0, t0: 0 };
      this.specs.set(file, s);
    }
    return s;
  }

  onTestBegin(test) {
    if (!this.enabled) return;
    try {
      const s = this.spec(test);
      if (s.started) return;
      s.started = true;
      s.t0 = Date.now();
      this.emit('qa.run.started', { spec: s.name, steps: stepTitles(test.location && test.location.file) });
    } catch {
      /* never fail the run */
    }
  }

  onStepEnd(test, _result, step) {
    if (!this.enabled) return;
    try {
      if (!step || step.category !== 'test.step' || step.parent) return;
      const s = this.spec(test);
      s.steps += 1;
      this.emit('qa.step', {
        index: s.steps,
        step: String(step.title || ''),
        status: step.error ? 'failed' : 'passed',
        duration_ms: Math.max(0, Math.round(step.duration || 0)),
      });
    } catch {
      /* never fail the run */
    }
  }

  onTestEnd(test, result) {
    if (!this.enabled) return;
    try {
      const s = this.spec(test);
      s.done += 1;
      if (result.status === 'passed' || result.status === 'skipped') s.passed += 1;
      else s.failed += 1;
      // retries re-run a test: only the last attempt closes it.
      if (s.done >= s.total) {
        this.emit('qa.run.finished', {
          spec: s.name,
          passed: s.passed,
          failed: s.failed,
          total: s.total,
          duration_ms: Math.max(0, Date.now() - (s.t0 || Date.now())),
        });
      }
    } catch {
      /* never fail the run */
    }
  }

  async onEnd() {
    if (!this.enabled) return;
    try {
      await this.chain;
    } catch {
      /* swallowed */
    }
  }

  emit(type, data) {
    this.seq += 1;
    const ev = { id: `${this.run}-${this.seq}`, type, at: new Date().toISOString(), run: this.run, data };
    if (this.product) ev.product = this.product;
    if (this.org) ev.org = this.org;
    // Sequential, so Brain Town sees the events in order.
    this.chain = this.chain.then(() => this.post(ev)).catch(() => {});
  }

  async post(ev) {
    if (this.dead) return;
    try {
      const headers = { 'Content-Type': 'application/json' };
      if (this.token) headers.Authorization = `Bearer ${this.token}`;
      const res = await fetch(this.url, {
        method: 'POST',
        headers,
        body: JSON.stringify(ev),
        signal: AbortSignal.timeout(TIMEOUT_MS),
      });
      // Drain so the socket is released; the body is not used.
      await res.arrayBuffer().catch(() => undefined);
    } catch {
      this.dead = true;
    }
  }
}

/**
 * Titles of the test.step('...') calls found in the spec source, in order, so the town can lay the
 * route out before the run. Only the titles leave this process. A dynamic title (template with ${})
 * is skipped; the step event still carries its real name.
 */
function stepTitles(file) {
  try {
    if (!file) return [];
    const src = fs.readFileSync(file, 'utf8');
    const out = [];
    const re = /\btest\.step\(\s*(['"`])((?:\\.|(?!\1)[^\\\n])*)\1/g;
    let m;
    while ((m = re.exec(src)) !== null && out.length < 50) {
      if (m[1] === '`' && m[2].includes('${')) continue;
      out.push(m[2].slice(0, 120));
    }
    return out;
  } catch {
    return [];
  }
}

module.exports = BrainTownReporter;
