#!/usr/bin/env node

import { readFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const DEFAULT_BASE_URL = process.env.BASE_URL || 'http://localhost:8080';
const DEFAULT_COUNT = 30;

function parseArgs(argv) {
  const args = { baseUrl: DEFAULT_BASE_URL, count: DEFAULT_COUNT, dryRun: false };

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

    if (token === '--dry-run') {
      args.dryRun = true;
      continue;
    }
  }

  return args;
}

function randomInt(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

function randomItem(items) {
  return items[randomInt(0, items.length - 1)];
}

function shuffle(items) {
  for (let i = items.length - 1; i > 0; i -= 1) {
    const j = randomInt(0, i);
    [items[i], items[j]] = [items[j], items[i]];
  }
  return items;
}

function statusPool(count) {
  const fixedCount = Math.round(count * 0.55);
  const openCount = Math.round(count * 0.3);
  const inProgressCount = Math.max(0, count - fixedCount - openCount);

  return shuffle([
    ...Array.from({ length: fixedCount }, () => 'fixed'),
    ...Array.from({ length: openCount }, () => 'open'),
    ...Array.from({ length: inProgressCount }, () => 'in_progress'),
  ]);
}

async function readIssuesPostPath() {
  const issuesDocPath = resolve('ai-backend/resources/issues.http');
  const doc = await readFile(issuesDocPath, 'utf8');
  const match = doc.match(/^POST\s+\{\{base_url\}\}(\/api\/issues)\s*$/m);
  return match?.[1] || '/api/issues';
}

function buildSeedRecords(count) {
  const pages = ['login', 'checkout', 'search', 'profile', 'settings', 'reports', 'billing'];
  const actions = ['click_submit', 'submit_form', 'open_modal', 'navigate_next', 'toggle_filter', 'retry_request'];
  const errorTypes = ['validation_error', 'timeout_error', 'network_error', 'ui_error', 'state_error'];
  const severities = ['low', 'medium', 'high', 'critical'];
  const categories = ['frontend', 'backend', 'unknown'];
  const notesTemplates = [
    'Reproduced in staging; attached HAR log and console traces.',
    'Likely race condition between route transition and state hydration.',
    'Backend returned intermittent 502 from dependency service.',
    'Added guard clause and null checks; pending QA verification.',
    'Observed only on slower networks and mobile viewport.',
  ];

  const statuses = statusPool(count);

  return statuses.map((status, index) => {
    const isFixed = status === 'fixed';
    const severity = randomItem(severities);
    const page = randomItem(pages);
    const action = randomItem(actions);
    const errorType = randomItem(errorTypes);
    const category = randomItem(categories);
    const ticket = `FT-${String(1000 + index)}`;

    return {
      page,
      action,
      error_type: errorType,
      error_text: `[${ticket}] ${errorType.replaceAll('_', ' ')} on ${page} during ${action}. Severity=${severity}.`,
      category,
      severity,
      is_fixed: isFixed,
      notes: `${randomItem(notesTemplates)} status=${status}; owner=qa-automation;`,
    };
  });
}

async function main() {
  const { baseUrl, count, dryRun } = parseArgs(process.argv.slice(2));
  const route = await readIssuesPostPath();
  const root = baseUrl.replace(/\/$/, '');
  const createUrl = `${root}${route}`;
  const statusUrl = `${root}/api/issues/status`;
  const notesUrl = `${root}/api/issues/notes`;

  const records = buildSeedRecords(count);

  let created = 0;
  let statusUpdated = 0;
  let notesUpdated = 0;
  let failed = 0;

  for (const record of records) {
    if (dryRun) {
      created += 1;
      if (record.is_fixed) statusUpdated += 1;
      notesUpdated += 1;
      continue;
    }

    try {
      const createRes = await fetch(createUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          page: record.page,
          action: record.action,
          error_type: record.error_type,
          error_text: record.error_text,
          category: record.category,
          severity: record.severity,
        }),
      });

      if (!createRes.ok) {
        failed += 1;
        const text = await createRes.text();
        console.error(`Create failed: ${createRes.status} ${text}`);
        continue;
      }

      const createdIssue = await createRes.json();
      created += 1;

      if (record.is_fixed) {
        const statusRes = await fetch(statusUrl, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id: createdIssue.id, is_fixed: true }),
        });

        if (!statusRes.ok) {
          failed += 1;
          const text = await statusRes.text();
          console.error(`Status update failed id=${createdIssue.id}: ${statusRes.status} ${text}`);
          continue;
        }

        statusUpdated += 1;
      }

      const notesRes = await fetch(notesUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: createdIssue.id, notes: record.notes }),
      });

      if (!notesRes.ok) {
        failed += 1;
        const text = await notesRes.text();
        console.error(`Notes update failed id=${createdIssue.id}: ${notesRes.status} ${text}`);
        continue;
      }

      notesUpdated += 1;
    } catch (error) {
      failed += 1;
      console.error('Seed request failed:', error);
    }
  }

  console.log(
    `Seed completed. Created=${created}, StatusUpdated=${statusUpdated}, NotesUpdated=${notesUpdated}, Failed=${failed}, Total=${records.length}`
  );

  if (failed > 0) {
    process.exitCode = 1;
  }
}

main().catch((error) => {
  console.error('Seeder failed:', error);
  process.exit(1);
});
