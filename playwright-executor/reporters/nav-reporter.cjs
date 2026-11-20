'use strict';
/**
 * Navigation reporter (docs/designs/NAVIGATION_MAP.md 3.5). Added by case_run next to the json and
 * list reporters. It records, per test, every page navigation ("Navigate" step of page.goto) and
 * every API request step (GET, POST, …) with the URL resolved against the project's baseURL, keeps
 * only the origin and the pathname (never the query), and writes one JSON file to
 * $QA_NAV_OUTPUT when the run ends. It never throws: a failure here must not change the run.
 *
 * Plain CommonJS with no dependency, so it loads in whatever Playwright version the automation repo
 * pins. It reads step.params and step.subtitle (Playwright 1.6x) or the URL inside the step title
 * (1.5x: 'GET "<url>"', 'Navigate to "<url>"'); neither form is documented reporter API, so the
 * executor's tests run it in a real Playwright run.
 */
const fs = require('fs');
const path = require('path');

const MAX_ENTRIES = 500;
const API_TITLES = new Set(['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS', '{method}']);

function resolveUrl(raw, base) {
  if (typeof raw !== 'string' || raw === '') return null;
  let s = raw;
  // A step subtitle drops the scheme ("host:port/path").
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(s) && !s.startsWith('/') && /^[^/]+[.:][^/]*\//.test(s)) s = 'http://' + s;
  try {
    const u = base ? new URL(s, base) : new URL(s);
    if (u.protocol !== 'http:' && u.protocol !== 'https:') return null;
    return u;
  } catch {
    return null;
  }
}

class NavReporter {
  constructor() {
    this.rootDir = '';
    this.entries = new Map();
    this.truncated = false;
  }

  printsToStdio() {
    return false;
  }

  onBegin(config) {
    try {
      // Spec files are reported relative to the repo (the config file's folder), not the testDir.
      this.rootDir = (config && config.configFile && path.dirname(config.configFile)) || (config && config.rootDir) || '';
    } catch {
      /* keep going */
    }
  }

  onStepEnd(test, _result, step) {
    try {
      if (!step || step.category !== 'pw:api') return;
      const title = String(step.title || '');
      const params = step.params || {};
      // 1.6x: title "Navigate" / "GET" with the URL in params (and subtitle).
      // 1.5x: title 'Navigate to "<url>"' / 'GET "<url>"' with the URL in the title.
      let kind;
      let verb = title;
      let raw = params.url !== undefined ? params.url : step.subtitle;
      let m;
      if (title === 'Navigate') kind = 'web';
      else if (API_TITLES.has(title)) kind = 'api';
      else if ((m = /^Navigate to "(.*)"$/.exec(title))) {
        kind = 'web';
        raw = m[1];
      } else if ((m = /^(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|\{method\}) "(.*)"$/.exec(title))) {
        kind = 'api';
        verb = m[1];
        raw = m[2];
      } else return;
      let method = '';
      if (kind === 'api') {
        method = String(params.method || (verb === '{method}' ? 'GET' : verb)).toUpperCase().slice(0, 8);
      }
      let project;
      try {
        project = test.parent.project();
      } catch {
        project = undefined;
      }
      const base = project && project.use && project.use.baseURL;
      const u = resolveUrl(raw, base);
      if (!u) return;
      let file = '';
      try {
        file = path.relative(this.rootDir || process.cwd(), test.location.file).split(path.sep).join('/');
      } catch {
        file = '';
      }
      const e = {
        title: String(test.title || '').slice(0, 300),
        file: file.slice(0, 300),
        project: String((project && project.name) || '').slice(0, 100),
        kind,
        method,
        host: u.host.slice(0, 200),
        path: u.pathname.slice(0, 500),
      };
      const key = JSON.stringify(e);
      const old = this.entries.get(key);
      if (old) {
        old.hits++;
        return;
      }
      if (this.entries.size >= MAX_ENTRIES) {
        this.truncated = true;
        return;
      }
      this.entries.set(key, { ...e, hits: 1 });
    } catch {
      /* never fail the run */
    }
  }

  onEnd() {
    try {
      const out = process.env.QA_NAV_OUTPUT;
      if (!out) return;
      fs.mkdirSync(path.dirname(out), { recursive: true });
      fs.writeFileSync(out, JSON.stringify({ version: 1, truncated: this.truncated, entries: [...this.entries.values()] }));
    } catch {
      /* never fail the run */
    }
  }
}

module.exports = NavReporter;
