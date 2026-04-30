#!/usr/bin/env node

import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const DEFAULT_BASE_URL = process.env.BASE_URL || 'http://localhost:8080';
const DEFAULT_COUNT = 30;

function parseArgs(argv) {
  const args = { baseUrl: DEFAULT_BASE_URL, count: DEFAULT_COUNT };

  for (let i = 0; i < argv.length; i += 1) {
    const token = argv[i];
    if (token === '--base-url' && argv[i + 1]) {
      args.baseUrl = argv[i + 1];
      i += 1;
      continue;
    }

    if (token === '--count' && argv[i + 1]) {
      const parsed = Number(argv[i + 1]);
      if (!Number.isFinite(parsed) || parsed <= 0) {
        throw new Error(`Invalid --count value: ${argv[i + 1]}`);
      }
      args.count = Math.floor(parsed);
      i += 1;
      continue;
    }
  }

  return args;
}

function randomItem(items) {
  const idx = Math.floor(Math.random() * items.length);
  return items[idx];
}

async function readIssuesPostPath() {
  const issuesDocPath = resolve('ai-backend/resources/issues.http');
  const doc = await readFile(issuesDocPath, 'utf8');
  const match = doc.match(/^POST\s+\{\{base_url\}\}(\/api\/issues)\s*$/m);
  return match?.[1] || '/api/issues';
}

function buildIssuePayload(index) {
  const pages = ['login', 'checkout', 'search', 'profile', 'settings', 'reports'];
  const actions = ['click_submit', 'open_modal', 'submit_form', 'navigate_next', 'toggle_filter'];
  const errorTypes = ['validation_error', 'timeout_error', 'network_error', 'ui_error'];
  const severities = ['low', 'medium', 'high', 'critical'];
  const categories = ['validation', 'performance', 'network', 'ui'];

  const errorTextTemplates = [
    'Field validation failed for required input',
    'Request timed out while loading API response',
    'Unexpected UI state after interaction',
    'Missing element after route transition',
    'Failed API dependency during flow execution',
  ];

  return {
    page: randomItem(pages),
    action: randomItem(actions),
    error_type: randomItem(errorTypes),
    error_text: `${randomItem(errorTextTemplates)} (seed #${index + 1})`,
    category: randomItem(categories),
    severity: randomItem(severities),
  };
}

async function main() {
  const { baseUrl, count } = parseArgs(process.argv.slice(2));
  const route = await readIssuesPostPath();
  const targetUrl = `${baseUrl.replace(/\/$/, '')}${route}`;

  let success = 0;
  let failed = 0;

  for (let i = 0; i < count; i += 1) {
    const payload = buildIssuePayload(i);
    try {
      const res = await fetch(targetUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });

      if (!res.ok) {
        failed += 1;
        const text = await res.text();
        console.error(`Failed [${i + 1}/${count}] ${res.status}: ${text}`);
        continue;
      }

      success += 1;
    } catch (error) {
      failed += 1;
      console.error(`Request failed [${i + 1}/${count}]`, error);
    }
  }

  console.log(`Seeding completed. Success: ${success}, Failed: ${failed}, Total: ${count}`);
  if (failed > 0) {
    process.exitCode = 1;
  }
}

main().catch((error) => {
  console.error('Seed script failed:', error);
  process.exit(1);
});
