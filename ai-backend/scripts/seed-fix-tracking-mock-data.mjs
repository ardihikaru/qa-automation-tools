#!/usr/bin/env node

import { readFile, access } from 'node:fs/promises';
import { constants as fsConstants } from 'node:fs';
import { resolve } from 'node:path';
import { spawnSync } from 'node:child_process';

const DEFAULT_BASE_URL = process.env.BASE_URL || 'http://localhost:8080';
const DEFAULT_COUNT = 100;
const DEFAULT_MODE = 'api';

function parseArgs(argv) {
  const args = {
    mode: DEFAULT_MODE,
    baseUrl: DEFAULT_BASE_URL,
    count: DEFAULT_COUNT,
    dryRun: false,
  };

  for (let i = 0; i < argv.length; i += 1) {
    const token = argv[i];

    if (token === '--mode' && argv[i + 1]) {
      const mode = argv[i + 1];
      if (mode !== 'api' && mode !== 'sql') {
        throw new Error(`Invalid --mode value: ${mode}. Use api|sql.`);
      }
      args.mode = mode;
      i += 1;
      continue;
    }

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

function randomDateWithinLastDays(days) {
  const now = Date.now();
  const windowMs = days * 24 * 60 * 60 * 1000;
  return new Date(now - randomInt(60_000, windowMs));
}

function statusPool(count) {
  const fixedCount = Math.round(count * 0.6);
  const verifiedCount = Math.round(count * 0.2);
  const openCount = Math.round(count * 0.15);
  const inProgressCount = Math.max(0, count - fixedCount - verifiedCount - openCount);

  return shuffle([
    ...Array.from({ length: fixedCount }, () => 'fixed'),
    ...Array.from({ length: verifiedCount }, () => 'verified'),
    ...Array.from({ length: openCount }, () => 'open'),
    ...Array.from({ length: inProgressCount }, () => 'in_progress'),
  ]);
}

function generateFixTrackingSeedRecords(count) {
  const pages = ['login', 'checkout', 'search', 'profile', 'settings', 'reports', 'billing'];
  const actions = ['click_submit', 'open_modal', 'submit_form', 'navigate_next', 'toggle_filter', 'upload_document'];
  const errorTypes = ['validation_error', 'timeout_error', 'network_error', 'ui_error', 'state_error'];
  const severities = ['low', 'medium', 'high', 'critical'];
  const categories = ['frontend', 'backend', 'unknown'];
  const statuses = statusPool(count);

  return statuses.map((status, idx) => {
    const createdAt = randomDateWithinLastDays(30);
    const page = randomItem(pages);
    const action = randomItem(actions);
    const errorType = randomItem(errorTypes);
    const severity = randomItem(severities);
    const category = randomItem(categories);

    const isResolved = status === 'fixed' || status === 'verified';
    const resolutionHours = isResolved ? randomInt(1, 96) : null;
    const resolvedAt = isResolved
      ? new Date(createdAt.getTime() + resolutionHours * 60 * 60 * 1000)
      : null;

    const prNumber = randomInt(100, 999);
    const commitSha = Math.random().toString(16).slice(2, 14);

    const notesParts = [
      `status=${status}`,
      `fix_commit_url=https://github.com/example-org/qa-automation-tools/commit/${commitSha}`,
      `fix_pr_url=https://github.com/example-org/qa-automation-tools/pull/${prNumber}`,
      resolutionHours !== null ? `resolution_hours=${resolutionHours}` : 'resolution_hours=',
      resolvedAt ? `resolved_at=${resolvedAt.toISOString()}` : 'resolved_at=',
    ];

    return {
      idHint: idx + 1,
      page,
      action,
      error_type: errorType,
      error_text: `${errorType.replaceAll('_', ' ')} detected at ${page} during ${action}.`,
      category,
      severity,
      is_fixed: isResolved,
      notes: notesParts.join(' | '),
      created_at: createdAt.toISOString(),
      status,
    };
  });
}

async function readIssuesPostPath() {
  const issuesDocPath = resolve('ai-backend/resources/issues.http');
  const doc = await readFile(issuesDocPath, 'utf8');
  const match = doc.match(/^POST\s+\{\{base_url\}\}(\/api\/issues)\s*$/m);
  return match?.[1] || '/api/issues';
}

function getDatabaseUrl() {
  if (process.env.DATABASE_URL && process.env.DATABASE_URL.trim()) {
    return process.env.DATABASE_URL.trim();
  }

  try {
    const envPath = resolve('ai-backend/.env');
    const envRaw = spawnSync('bash', ['-lc', `grep -E '^DATABASE_URL=' ${envPath} | cut -d= -f2-`], {
      encoding: 'utf8',
    });

    if (envRaw.status === 0 && envRaw.stdout.trim()) {
      return envRaw.stdout.trim();
    }
  } catch {
    // fallback handled below
  }

  return '';
}

function escapeSqlText(value) {
  return String(value).replace(/'/g, "''");
}

async function seedByApi(records, baseUrl, dryRun) {
  const route = await readIssuesPostPath();
  const targetUrl = `${baseUrl.replace(/\/$/, '')}${route}`;

  let created = 0;
  let updatedStatus = 0;
  let updatedNotes = 0;
  let failed = 0;

  for (const item of records) {
    if (dryRun) {
      created += 1;
      continue;
    }

    try {
      const createRes = await fetch(targetUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          page: item.page,
          action: item.action,
          error_type: item.error_type,
          error_text: item.error_text,
          category: item.category,
          severity: item.severity,
        }),
      });

      if (!createRes.ok) {
        failed += 1;
        const text = await createRes.text();
        console.error(`Create failed [${item.idHint}] ${createRes.status}: ${text}`);
        continue;
      }

      const createdIssue = await createRes.json();
      created += 1;

      if (item.is_fixed) {
        const statusRes = await fetch(`${baseUrl.replace(/\/$/, '')}/api/issues/status`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id: createdIssue.id, is_fixed: true }),
        });

        if (statusRes.ok) {
          updatedStatus += 1;
        } else {
          failed += 1;
          console.error(`Status update failed [${createdIssue.id}] ${statusRes.status}`);
        }
      }

      const notesRes = await fetch(`${baseUrl.replace(/\/$/, '')}/api/issues/notes`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: createdIssue.id, notes: item.notes }),
      });

      if (notesRes.ok) {
        updatedNotes += 1;
      } else {
        failed += 1;
        console.error(`Notes update failed [${createdIssue.id}] ${notesRes.status}`);
      }
    } catch (error) {
      failed += 1;
      console.error(`Request failed [${item.idHint}]`, error);
    }
  }

  console.log(
    `API seeding completed. Created=${created}, StatusUpdated=${updatedStatus}, NotesUpdated=${updatedNotes}, Failed=${failed}, Total=${records.length}`
  );

  if (failed > 0) {
    process.exitCode = 1;
  }
}

async function seedBySql(records, dryRun) {
  const databaseUrl = getDatabaseUrl();
  if (!databaseUrl) {
    throw new Error('DATABASE_URL is missing. Set env var or add it to ai-backend/.env');
  }

  const rowsSql = records
    .map((item) => {
      return `('${escapeSqlText(item.page)}','${escapeSqlText(item.action)}','${escapeSqlText(item.error_type)}','${escapeSqlText(item.error_text)}','${escapeSqlText(item.category)}','${escapeSqlText(item.severity)}',${item.is_fixed ? 'TRUE' : 'FALSE'},'${escapeSqlText(item.notes)}','${escapeSqlText(item.created_at)}'::timestamptz)`;
    })
    .join(',\n');

  const sql = `INSERT INTO raw_issues (page, action, error_type, error_text, category, severity, is_fixed, notes, created_at) VALUES\n${rowsSql};`;

  if (dryRun) {
    console.log('Dry run enabled. SQL preview (first 25 lines):');
    console.log(sql.split('\n').slice(0, 25).join('\n'));
    console.log(`... total records in SQL: ${records.length}`);
    return;
  }

  const checkPsql = spawnSync('bash', ['-lc', 'command -v psql'], { encoding: 'utf8' });
  if (checkPsql.status !== 0) {
    throw new Error('psql not found. Install PostgreSQL client first for --mode sql');
  }

  const result = spawnSync('psql', [databaseUrl, '-v', 'ON_ERROR_STOP=1', '-c', sql], {
    encoding: 'utf8',
  });

  if (result.status !== 0) {
    throw new Error(`psql insert failed: ${result.stderr || result.stdout}`);
  }

  console.log(`SQL seeding completed. Inserted ${records.length} rows into raw_issues.`);
}

async function ensureRequiredFiles() {
  await access(resolve('ai-backend/resources/issues.http'), fsConstants.R_OK);
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  await ensureRequiredFiles();

  const records = generateFixTrackingSeedRecords(args.count);

  if (args.mode === 'api') {
    await seedByApi(records, args.baseUrl, args.dryRun);
    return;
  }

  await seedBySql(records, args.dryRun);
}

main().catch((error) => {
  console.error('Fix tracking seeder failed:', error.message || error);
  process.exit(1);
});
