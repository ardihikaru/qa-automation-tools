export interface AnalyticsData {
  flows: number;
  issues: number;
  skills: number;
}

export interface CostOverview {
  total_cost: number;
}

export interface IssueCount {
  total: number;
  open: number;
  fixed: number;
}

export type JobStatus = 'idle' | 'queued' | 'running' | 'completed' | 'failed';

export type JobType = 'qa_run' | 'docs_generate';

export interface JobRecord {
  id: string;
  type: JobType;
  status: JobStatus;
  payload?: Record<string, unknown>;
  result?: Record<string, unknown>;
  error?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  duration_ms?: number;
}

export interface MockQAInsightsData {
  analytics: AnalyticsData;
  cost: CostOverview;
  issueCounts: IssueCount;
  jobs: JobRecord[];
}

const ERROR_MESSAGES = [
  'Playwright timeout while waiting for selector: [data-testid="checkout-submit"]',
  'Assertion failed: expected HTTP 200 but received 500 on /api/payment/confirm',
  'Element detached from DOM during click action on #sign-in-button',
  'Network idle state not reached within 30 seconds during docs generation',
  'Rate limit reached for LLM provider during summarization step',
  'Snapshot mismatch detected in dashboard smoke test',
];

const QA_PAGES = ['login', 'checkout', 'profile', 'search', 'settings', 'reports'];

function randomInt(min: number, max: number): number {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

function randomItem<T>(items: readonly T[]): T {
  return items[randomInt(0, items.length - 1)];
}

function shuffle<T>(items: T[]): T[] {
  for (let i = items.length - 1; i > 0; i -= 1) {
    const j = Math.floor(Math.random() * (i + 1));
    [items[i], items[j]] = [items[j], items[i]];
  }
  return items;
}

function buildStatusPool(count: number): JobStatus[] {
  const completedCount = Math.round(count * 0.7);
  const failedCount = Math.round(count * 0.2);
  const queuedRunningCount = Math.max(0, count - completedCount - failedCount);
  const queuedCount = Math.floor(queuedRunningCount / 2);
  const runningCount = queuedRunningCount - queuedCount;

  return shuffle([
    ...Array.from({ length: completedCount }, () => 'completed' as const),
    ...Array.from({ length: failedCount }, () => 'failed' as const),
    ...Array.from({ length: runningCount }, () => 'running' as const),
    ...Array.from({ length: queuedCount }, () => 'queued' as const),
  ]);
}

function randomTimestampWithinLastDays(days: number): Date {
  const now = Date.now();
  const windowMs = days * 24 * 60 * 60 * 1000;
  return new Date(now - randomInt(5 * 60 * 1000, windowMs));
}

function makeJob(index: number, status: JobStatus): JobRecord {
  const type: JobType = index % 3 === 0 ? 'docs_generate' : 'qa_run';
  const createdAt = randomTimestampWithinLastDays(30);
  const startedAt = new Date(createdAt.getTime() + randomInt(5_000, 180_000));

  let durationMs: number | undefined;
  let completedAt: Date | undefined;
  let error: string | undefined;
  let result: Record<string, unknown> | undefined;

  if (status === 'completed') {
    durationMs = type === 'qa_run' ? randomInt(12_000, 180_000) : randomInt(25_000, 260_000);
    completedAt = new Date(startedAt.getTime() + durationMs);
    result = {
      summary: type === 'qa_run' ? 'Regression suite passed' : 'Documentation generated',
      checks: randomInt(12, 45),
      warnings: randomInt(0, 3),
    };
  }

  if (status === 'failed') {
    durationMs = type === 'qa_run' ? randomInt(10_000, 90_000) : randomInt(18_000, 130_000);
    completedAt = new Date(startedAt.getTime() + durationMs);
    error = randomItem(ERROR_MESSAGES);
  }

  if (status === 'running') {
    durationMs = randomInt(8_000, 95_000);
  }

  const payload: Record<string, unknown> = {
    triggered_by: index % 5 === 0 ? 'scheduler' : 'dashboard',
    source: 'qa_insights',
    target_page: randomItem(QA_PAGES),
    run_id: `run-${randomInt(1000, 9999)}`,
  };

  return {
    id: `job_mock_${String(index + 1).padStart(3, '0')}`,
    type,
    status,
    payload,
    result,
    error,
    created_at: createdAt.toISOString(),
    started_at: status === 'queued' ? undefined : startedAt.toISOString(),
    completed_at: completedAt?.toISOString(),
    duration_ms: durationMs,
  };
}

export function generateJobRecords(count = 30): JobRecord[] {
  const safeCount = Math.max(1, count);
  const statuses = buildStatusPool(safeCount);
  const jobs = statuses.map((status, index) => makeJob(index, status));

  return jobs.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());
}

export function generateAnalyticsData(): AnalyticsData {
  return {
    flows: 30,
    issues: 15,
    skills: 12,
  };
}

export function generateIssueCount(): IssueCount {
  return {
    total: 15,
    open: 6,
    fixed: 9,
  };
}

export function generateCostOverview(jobs: JobRecord[]): CostOverview {
  const totalDurationMs = jobs.reduce((sum, job) => sum + (job.duration_ms || 0), 0);
  const baseCost = totalDurationMs / 1_000_000;
  const jitter = randomInt(12, 45) / 1_000;

  return {
    total_cost: Number((baseCost + jitter).toFixed(4)),
  };
}

export function generateMockQAInsightsData(jobCount = 30): MockQAInsightsData {
  const jobs = generateJobRecords(jobCount);

  return {
    analytics: generateAnalyticsData(),
    issueCounts: generateIssueCount(),
    cost: generateCostOverview(jobs),
    jobs,
  };
}

export function simulateNetworkDelay(minMs = 350, maxMs = 900): Promise<void> {
  const delay = randomInt(minMs, maxMs);
  return new Promise((resolve) => setTimeout(resolve, delay));
}
