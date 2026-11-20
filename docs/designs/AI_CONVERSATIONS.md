# AI_CONVERSATIONS: every AI exchange stored per job, readable, archivable and prunable

*   **Author:** ardihikaru (drafted by Claude)
*   **Status:** Rev 2, **implemented** (2026-11-21) after one zero-context review round (implementation readiness, design, security); section 11 resolves the findings and supersedes earlier wording. The product owner asked for it and delegated the choices. Not deployed; the migration has run only on scratch databases.
*   **Depends on:** [CASE_AUTOMATION.md](CASE_AUTOMATION.md) (`case_create`, `caseauto/machine.go`), [PRODUCT_KNOWLEDGE_SYNC.md](PRODUCT_KNOWLEDGE_SYNC.md) (the sync's AI summary), [AI_MANAGER_AND_SELF_HOSTED_DEPLOY.md](AI_MANAGER_AND_SELF_HOSTED_DEPLOY.md) (`ai.Service`, the only LLM path), [RUN_HISTORY.md](RUN_HISTORY.md) (the history page this links from).

## 1. Problem

The product owner: "We need to see all historical conversation in each ai-manager work (if the conversation UI is not shown yet, add it). Add an archive button so I can delete them anytime. Also add an archive page to see all archived conversations, and inside that page a button to prune all archived conversations, which permanently deletes them." The AI's replies must be readable as written: markdown with nested bullets, fenced code and inline code.

What exists today:

*   **Nothing stores a conversation.** `ai.Service.ChatMessages` (`services/ai/aiService.go`) sends the messages and returns the reply text; its recorder (`ai.SQLRecorder`) writes one `ai_call_log` row per call with tokens, cost, latency and status, and no message content. `case_create` keeps its conversation in memory (`run.convo`, `caseauto/machine.go`) and loses it when the job ends; the job page shows only events and the final spec.
*   **Who calls the LLM** (every `ChatMessages` caller, `grep -rn "ChatMessages(\|\.Chat(\|ChatFeature("`):
    1.  `caseauto.Machine.build` (feature `case-automation`): the first generation, every repair round (refused reply, failed run) and every "Needs changes" round. The conversation grows by appending: the first user prompt (rules, case, repo guide, fixtures, examples), each assistant reply, then a user message per refusal (`Your reply was refused: …`), per failed run (`The spec was run against staging and did not pass: …`) or per reviewer request (`A reviewer asked for changes: …`, `machine.go` `finish`). After a backend restart, `Resume` rebuilds the first prompt (`initialConvo`) when changes are requested.
    2.  `knowledge.Syncer` AI summary (feature `knowledge-summary`, `knowledge/sync.go`): one system + one user message per sync, reply stored in `kb_syncs.ai_summary`.
    3.  `ai.Service.Chat` / `ChatFeature` wrappers, used by `ClassifyIssue`, `GenerateSuggestion`, `GenerateSkill` (no HTTP route reaches them today) and by the `cmd/verifyai` smoke test (no recorder is installed there).

## 2. Goal and non-goals

Goal: every AI exchange made by the backend is stored, grouped into one **conversation** per piece of work (a `case_create` job, a knowledge sync, or a single call that has no owner), and shown in order with safe markdown: from the job page, from a conversation page, and from the run history page. A conversation can be **archived** (hidden from the normal views) and **unarchived**; an **Archive** page lists the archived ones and offers **Prune all archived**, which permanently deletes their messages after a confirmation, Admin only, with an audit record.

Non-goals: changing what is sent to the LLM or to GAOS (nothing in `gaossync` or the GAOS contracts changes; D-077 stands); backfilling past conversations (they were never stored); editing messages; per-conversation delete without archiving first; search inside message text; storing provider-side chain-of-thought (only what the backend sent and received).

## 3. Decisions

1.  **Capture at the one seam.** `ai.Service.ChatMessages` hands every call, after the provider returns, to an optional `ConversationStore` (`SetConversationStore`), next to the existing `CallRecorder`. Nil store = nothing stored (tests, `cmd/verifyai`). The call's result and error are never changed by the store; a store error is logged (`ai conversations: <kind>/<ref>: <err>`) and dropped. The write runs synchronously with its own 5 s context, so a job's next step sees its previous exchange stored.
2.  **Which conversation.** `ai.ChatOpts` gains `Conversation *ConvRef{Kind, Ref, ProductID int, Title}`:
    *   `case_create`: `Kind "job"`, `Ref` = job id, `Title` = case code, set by the machine on every call.
    *   knowledge summary: `Kind "sync"`, `Ref` = `kb_syncs.id` as text, `Title` = product name.
    *   Anything without a ref (the legacy wrappers): `Kind "call"`, `Ref` = `call-<unix nanos>-<4 random hex>`, `Title` = the feature. Nothing is lost if a new caller forgets to set a ref.
3.  **Append-only, positional de-duplication.** Each call carries the whole conversation so far. The store keeps per message a SHA-256 of `role + "\n" + content` and appends only what is new: it finds the longest common prefix between the stored messages (excluding notes) and the call's messages, then appends the call's messages after that prefix, then the reply (role `assistant`) or, for a failed call, a note `error: <message>`. If the prefix is shorter than what is stored (the caller rebuilt the conversation, e.g. `Resume` after a restart), a note `The conversation was rebuilt (for example after a backend restart); the messages below were sent again.` is appended first. Writes for one conversation are serialised by `SELECT … FOR UPDATE` on its row inside one transaction (`db.WithTx`).
4.  **Message kinds**, so the UI can show each one right: `models.Message` gains `Kind string` with `json:"-"` (never sent to a provider; the wire format is unchanged). The machine sets `prompt` (the first user message), `refusal`, `run_result`, `feedback` (reviewer "Needs changes" text) and `reply` (assistant). Without a kind the store derives one: role `system` → `prompt`, the first stored message → `prompt`, assistant → `reply`, else `message`. Notes have role `note` and kind `rebuilt` or `error`.
5.  **Redaction and size.** The machine already redacts every message it builds (`Redactor`). The store applies a `caseauto.Redactor` built from the same secrets (`CaseAutomationConfig.Secrets`) as a second line, so a caller that forgets cannot persist a known token. One message is clipped at 256 KiB (`… [clipped, N bytes]`); the conversation row keeps `message_count` and `bytes`.
6.  **Archive.** `archived_at`/`archived_by` on the conversation. Archived conversations are left out of every list and of the job page (which then says "This conversation is archived" with a link to it). Archiving and unarchiving need `canVerify` (Admin, DevOps, Developer, QA, QC). Archiving is refused (409 `active`) while the owner is still working: a `job` whose `job_runs.status` is `queued`/`running`, or a `sync` whose `kb_syncs.status` is `running`; otherwise a later call would add messages to a hidden conversation. Unarchive is always allowed.
7.  **Prune.** Two steps, **Admin only** (`Service` cannot prune):
    *   `GET /api/conversations/prune` (preview) → `{conversations, messages, bytes, archived_before}` where `archived_before` is the newest `archived_at` included (RFC3339, null when nothing is archived).
    *   `POST /api/conversations/prune` body `{"confirm": "prune", "archived_before": "<from the preview>"}` deletes exactly the conversations with `archived_at IS NOT NULL AND archived_at <= archived_before` (a conversation archived after the preview is not touched) and their messages (FK cascade), in one transaction, and writes one `ai_conversation_prunes` row (who, when, counts, bytes). Answer: the counts deleted.
    *   **What prune deletes:** rows of `ai_conversations` that are archived and their `ai_messages`. **What it never touches:** `job_runs`, `job_events`, `case_approvals`, `pw_runs`/`pw_run_cases`, `kb_syncs` (including `ai_summary`), `ai_call_log` (tokens and cost) and the audit table. Run history, build durations and the cost page are unchanged by a prune.
8.  **Read access.** Every signed-in user may read conversations (like the job page, which already shows the generated spec to any signed-in user).
9.  **Markdown.** The dashboard reuses `views/assistants/Markdown.tsx` (React elements only, raw HTML shown as text, links limited to http(s)/mailto/anchors; nested and ordered lists, fenced code, inline code, tables). Assistant replies and reviewer feedback render as markdown; prompts, refusals and run results render as preformatted text (they are machine-built, not markdown), collapsed by default when longer than 600 characters.

## 4. Data (`migrations.go`, appended, `CREATE TABLE IF NOT EXISTS`, utf8mb4_general_ci)

```sql
CREATE TABLE IF NOT EXISTS ai_conversations (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  kind VARCHAR(8) NOT NULL,                 -- job | sync | call
  ref VARCHAR(64) NOT NULL,                 -- job id, kb_syncs.id, or call-<nanos>-<hex>
  product_id INT NULL,
  feature VARCHAR(64) NOT NULL DEFAULT '',
  title VARCHAR(200) NOT NULL DEFAULT '',
  message_count INT NOT NULL DEFAULT 0,
  bytes BIGINT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  archived_at DATETIME(3) NULL,
  archived_by VARCHAR(64) NULL,             -- user id
  UNIQUE KEY uq_ai_conversations_ref (kind, ref),
  INDEX idx_ai_conversations_list (archived_at, updated_at),
  INDEX idx_ai_conversations_product (product_id, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS ai_messages (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  conversation_id BIGINT NOT NULL,
  seq INT NOT NULL,
  role VARCHAR(12) NOT NULL,                -- system | user | assistant | note
  kind VARCHAR(12) NOT NULL,                -- prompt | message | reply | feedback | refusal | run_result | rebuilt | error
  content MEDIUMTEXT NOT NULL,
  content_hash CHAR(64) NOT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  UNIQUE KEY uq_ai_messages_seq (conversation_id, seq),
  CONSTRAINT fk_ai_messages_conversation FOREIGN KEY (conversation_id) REFERENCES ai_conversations(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS ai_conversation_prunes (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  pruned_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  pruned_by VARCHAR(64) NOT NULL,           -- user id
  pruned_by_name VARCHAR(100) NOT NULL DEFAULT '',
  archived_before DATETIME(3) NOT NULL,
  conversations INT NOT NULL,
  messages INT NOT NULL,
  bytes BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
```

No FK from `ai_conversations` to `products`/`job_runs`: a conversation outlives nothing it should not, and a product delete leaves its conversations listable (shown without a product name). Rollback: the old binary ignores the tables.

## 5. Routes

Cookie auth; errors `{"error", "code"}`; lists `{items, total, page, page_size}` (`parsePagination`, default 25).

*   `GET /api/conversations?archived=0|1&kind=job|sync|call&product=<id>&q=<title or ref, exact>` → items `{id, kind, ref, product_id, product_name, feature, title, message_count, bytes, created_at, updated_at, archived_at, archived_by_name}`, newest `updated_at` first. `archived` defaults to 0.
*   `GET /api/conversations/{id}` → the item plus `messages: [{seq, role, kind, content, created_at}]` in `seq` order (archived ones too: the Archive page opens them).
*   `GET /api/jobs/{id}/conversation` → the same for the job's conversation, or `{"conversation": null}` when none is stored; an archived one answers `{"conversation": {…, "archived_at": …}, "messages": []}` (the page links to it instead).
*   `POST /api/conversations/{id}/archive`, `POST /api/conversations/{id}/unarchive` → the item; 403 without `canVerify`; 404; 409 `active` (archive only, 3.6).
*   `GET /api/conversations/prune`, `POST /api/conversations/prune` (3.7): 403 unless Admin; 400 `confirm` when the body is not exactly `{"confirm":"prune","archived_before":…}`; 409 `nothing` when nothing matches.
*   `GET /api/conversations/prunes` → the audit rows, newest first (Admin).
*   The run history (`/api/history/builds` and `/syncs` items) gains `conversation_id` (the non-archived conversation of the job or sync, else null).

## 6. Dashboard

*   `components/ConversationThread.tsx`: one message per block in `seq` order with role/kind label and time. `prompt` collapsed ("Prompt sent to claude-v3, 42 KB, show"), `reply` and `feedback` as markdown (`Markdown`), `refusal`/`run_result` as compact preformatted notes, `rebuilt`/`error` notes in grey/red.
*   **Job page** (`JobView`): an "AI conversation" card for `case_create` jobs (polls with the existing 2 s loop while the job runs) with an **Archive** button (confirm dialog); archived → the notice and a link.
*   **Conversation page** `/conversations/:id`: header (title, kind, product, job or sync link, counts), the thread, Archive/Unarchive.
*   **Run history**: a fourth tab **AI conversations** (non-archived list: title, kind, product, messages, size, updated, link, Archive); Builds and Syncs rows get a "Conversation" link when `conversation_id` is set.
*   **Archive page** `/archive` (nav entry "Archive"): the archived list (open, Unarchive), the **Prune all archived** button for Admins (others see it disabled with "Admins only"). The button opens a dialog with the preview counts; the confirm button stays disabled until the user types `prune`; after success a toast and the list reloads. Below: the prune audit (who, when, how many).

## 7. Security

New writers: the store (server-side only, from `ai.Service`), archive/unarchive (`canVerify`), prune (Admin, explicit confirm string plus the preview's cut-off, one audit row). Prompts may carry repo files and case scenarios, already visible to signed-in users on the job and knowledge pages; secrets are redacted twice (3.5). No new allow-list path, no new external origin, no new token. Rendering never injects HTML.

## 8. Target files

`ai-backend/internal/{models/models.go (Message.Kind), services/ai/{provider.go, aiService.go, conversation.go (new: ConvRef, ConversationStore), aiService_test.go}, services/conversations/ (new: store.go, query.go, README.md, store_test.go, store_integration_test.go), services/caseauto/machine.go (Conversation and kinds), services/knowledge/sync.go (Conversation), api/{conversations.go (new), conversations_test.go (new), job_events.go (sub-route), history.go (conversation_id), router.go, README.md}, db/migrations.go}`, `cmd/server/main.go` (wiring); `dashboard/src/{App.tsx, components/Layout.tsx, components/ConversationThread.tsx (new), views/JobView.tsx, views/conversations/ (new: api.ts, ConversationView.tsx, ArchiveView.tsx, ConversationsTab.tsx, PruneDialog.tsx), views/history/{HistoryView.tsx, BuildsTab.tsx, SyncsTab.tsx, api.ts}}`, `dashboard/README.md`; `this-qa-automation-system-playwright/tests/{api/conversations.spec.ts, dashboard/archive.spec.ts}`, its README; `docs/designs/{AI_CONVERSATIONS.md, SITEMAP.md}`.

## 9. Verification

*   Unit: `ai.Service` hands sent messages, reply and error to the store and returns the same result with and without a store, and with a failing store; `Message.Kind` is not in the provider's JSON; the dedupe (pure function `Plan(stored []hash, sent []Message) (prefix int, rebuilt bool)`): growing conversation, identical re-send, rebuilt prefix, empty store.
*   Integration (`QA_TEST_MYSQL_DSN`): a simulated case_create sequence (prompt → reply → refusal → reply → run_result → reply → feedback → reply) stores 8 messages in order with kinds; a rebuilt conversation adds the note; an error call adds an `error` note; the redactor removes a configured secret; archive refused while the job is running, allowed after; lists hide archived; prune: 403 for QA and Service, 400 without the confirm string, deletes only conversations archived up to the cut-off, keeps `job_runs`, `job_events`, `ai_call_log` and `pw_runs` rows, writes the audit row.
*   Machine test (fakes): every `ChatMessages` call carries the job's `ConvRef` and the kinds above.
*   Dashboard: `npx tsc --noEmit` (no new errors), `npm run build`; local stack: thread renders nested lists, fenced and inline code from a stored reply; archive → gone from the history tab and the job page; Archive page lists it; prune dialog flow; screenshots.
*   E2E specs (deployment, credentials): conversations list/detail shape; 403 on prune for a non-Admin; the Archive page and its disabled prune button for non-Admins.

## 10. Alternatives weighed

*   **Store in `job_events`**: its messages are clipped to 1000 bytes and it is the live log; a 40 KB prompt does not belong there. **In `ai_call_log`**: it is the cost ledger that prune must keep, and each call would hold the whole conversation again (quadratic). **Capture in each caller**: two callers today, but every new caller would have to remember; the `ai.Service` seam sees every call and the callers only add a reference. Hence two new tables behind the one seam.

## 11. Review round 1 resolutions (supersede earlier wording)

Reviewers: implementation readiness, design, security (zero context). Blockers: Resume breaks the dedupe; a sync could be archived before its summary is written; the `Service` account could prune by impersonating an Admin. All resolved below.

1.  **Dedupe against the last call, not the whole history** (replaces 3.3's comparison). The conversation row keeps `last_hashes` (JSON array): the hashes of the last call's sent messages plus its reply (no reply after an error). For a new call, `p` = the longest common prefix of `last_hashes` and the call's sent hashes. The store appends `sent[p:]` and then the reply (or an `error` note). It appends a `rebuilt` note first when `p < len(last_hashes)`, i.e. the caller did not simply extend the previous call. Then it sets `last_hashes` = hashes of `sent` + reply. A normal round matches the whole previous call; an identical retry after an error appends only the reply; after `Resume` (`[prompt, render(file), feedback]`) one note and the three messages are added once, and later rounds match again. `Plan(last []string, sent []string) (p int, rebuilt bool)` is the pure function tested for growth, identical re-send, retry after error, resume-then-two-rounds and an empty conversation.
2.  **Order and hash.** For every message: redact with the store's screen (11.6) → hash `sha256(role + "\n" + redacted)` over the unclipped redacted text → clip at 256 KiB → store. Both sides of the comparison use the same function, so a message that only the store redacts still matches.
3.  **The store interface.** `ai.ConversationStore{ SaveExchange(ctx context.Context, ex Exchange) error }`, `Exchange{Ref ConvRef; Feature, Provider, Model string; Sent []models.Message; Reply string; Err error}`. `ChatMessages` always passes a `ConvRef` (the `call` default of 3.2 is filled in by the service). On error the reply is not stored (the service returns `""` then), only the `error` note with the error text (redacted, 1000 bytes). The service calls the store with `context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)` inside a `recover()`; a panic or error is logged and the AI result is returned unchanged. As built, the store does not change `innodb_lock_wait_timeout` (a session setting would stay on the pooled connection); the 5 s deadline cancels a waiting statement instead. The conversation row also keeps `provider` and `model` of the last call.
4.  **First write and locking.** `INSERT … ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)` then `SELECT … FOR UPDATE` on that id, in one `db.WithTx` (retried on deadlock). Appending to an archived conversation clears `archived_at` (new activity brings it back) and logs it; with the guards in 11.5 this should not happen for jobs or syncs.
5.  **Archive guard, atomic** (replaces 3.6's guard). Archive is one statement: `UPDATE ai_conversations c SET archived_at = ?, archived_by = ? WHERE c.id = ? AND c.archived_at IS NULL AND NOT (<owner active>)`, where owner active is, for `job`, `EXISTS (SELECT 1 FROM job_runs j WHERE j.id = c.ref COLLATE utf8mb4_general_ci AND j.status IN ('queued','running'))` (a job waiting for approval is `running`), and for `sync`, `EXISTS (SELECT 1 FROM kb_syncs s WHERE CAST(s.id AS CHAR) = c.ref AND (s.status = 'running' OR s.ai_status = 'pending'))`. Zero rows changed → a follow-up read answers 404 `not_found`, 409 `already_archived` or 409 `active`.
6.  **Redaction for storage.** The store's screen is the `caseauto.Redactor` (same literal secrets as case automation, plus `DB_PASSWORD`, `ADMIN_PASSWORD`, `AI_SERVICE_PASSWORD`, `RESULTS_INGEST_TOKEN` and `PLAYWRIGHT_EXECUTOR_TOKEN` values) followed by store-only patterns: PEM private-key blocks, JWTs (`eyJ…\.eyJ…\.…`), `AKIA[0-9A-Z]{16}`, `sk-[A-Za-z0-9_-]{20,}`, and `password|passwd|pwd|secret|api[_-]?key|token` followed by `:` or `=` and a value (the value only is replaced). The store-only patterns never touch what is sent to the LLM. "Redacted twice" in 3.5 holds for `case_create` only; for syncs and single calls the store is the screen. One integration test per kind.
7.  **Who may do what** (replaces 3.7's role and 3.8). Read the list (metadata): every signed-in user. Read messages (`GET …/{id}`, the job route's `messages`): `canVerify` (Admin, DevOps, Developer, QA, QC); others get 403 `forbidden` on the detail and `messages: []` with `"restricted": true` on the job route. There is no product boundary (products are visible to every signed-in user; stated, not new). Archive/unarchive: `canVerify`. Prune (preview, prune, audit list): `auth.RequireRealUser(u) && u.Role == "Admin"`; an impersonated session, including `Service` acting as an Admin, gets 403 `real_admin_only`.
8.  **Audit, append-only** (replaces `ai_conversation_prunes`). Table `ai_conversation_audit(id, at DATETIME(3), action ENUM('archive','unarchive','prune'), actor_id, actor_name (the real user), as_user_id (the impersonated identity, NULL when none), conversation_id NULL, detail JSON)`; prune's `detail` = `{archived_before, conversations, messages, bytes, ids}` (ids capped at 5000). `archived_by` on the row is the real actor. Every new POST requires `Content-Type: application/json` (`isJSON`, 415 otherwise).
9.  **Prune mechanics** (replaces 3.7's transaction). `archived_before` is UTC with milliseconds, `2006-01-02T15:04:05.000Z`; the body must be exactly `{"confirm":"prune","archived_before":"…"}` (unknown keys, another confirm word or a malformed or null time → 400 `confirm`). The delete runs in batches of 50 conversations (`SELECT id … WHERE archived_at IS NOT NULL AND archived_at <= ? ORDER BY id LIMIT 50`, then `DELETE` by id, cascade), each its own transaction; the audit row is written once at the end with the summed counts (also after a failing batch, then 500 with the counts so far). Nothing matching → 409 `nothing`.
10. **Incremental reads.** `GET /api/conversations/{id}?since_seq=N` and `GET /api/jobs/{id}/conversation?since_seq=N` return only messages with `seq > N`. The job route has one shape: `{conversation: item | null, messages: [], restricted?: true}`; an archived conversation gives the item (with `archived_at`) and no messages. The job page polls with its existing 2 s loop and passes the last seq it holds.
11. **History links, exact SQL.** Builds: `(SELECT c.id FROM ai_conversations c WHERE c.kind = 'job' AND c.ref = j.id COLLATE utf8mb4_general_ci AND c.archived_at IS NULL) AS conversation_id`; syncs: `… c.kind = 'sync' AND c.ref = CAST(s.id AS CHAR) …`. The "AI conversations" history tab stays (it is the "see all historical conversations" list); its filters are `kind`, `product` and `q` = `(c.ref = ? OR c.title = ?)`.
12. **Kinds after a restart.** The rebuilt first prompt is `prompt`, `render(r.file)` is `reply`, the reviewer text is `feedback`.
13. **Rendering.** `Markdown` gains two props used by the conversation view: `idPrefix` (heading ids become `md-<conversation>-<slug>`) and `showLinkHost` (each external link is followed by its host in grey, so a reader sees where it goes). Prompts, refusals and run results are text nodes in a `<pre>`. The prompt label is "Prompt" plus its size.
14. **Storage and retention.** One build is about 40 KB (prompt) + ~10 × (≤ 20 KB reply + ~3.5 KB run tail) ≈ 280 KB; 100 builds a day would be ~28 MB/day. The Archive page shows the total size of archived and of all conversations. Reclaiming disk after a large prune needs `OPTIMIZE TABLE ai_messages` (an operator step, in the README). An automatic archive age is a follow-up, not built: the owner asked to archive by hand.
15. **Error envelope.** Every new route uses the existing `writeCodedError` (`epic_batches.go`): `forbidden`, `real_admin_only`, `not_found`, `already_archived`, `not_archived`, `active`, `confirm`, `nothing`, `bad_request`, `unsupported_media_type`.
16. **Verification additions.** Providers: an `httptest` server captures the request body of `aimanager` and `minimax` with a `Kind` set and asserts it has no `kind` key. Store: a panicking store and a failing store leave `ChatMessages`'s result unchanged. Prune: Service-as-Admin → 403; archived after the preview → kept; a cut-off with milliseconds; unarchived after the preview → kept; job, event, call-log and run rows kept. Archive: refused while a job is `running`, while a sync is `ai_status = 'pending'`. Rendering (local Playwright, seeded reply): the thread contains `ul ul`, `pre code` and inline `code`, and no element from raw HTML in the reply. TypeScript: the 9 errors already on main in `NavigationGraphView.tsx` are the baseline; no others.

## 12. As built

Target-file differences: `services/conversations/` is `plan.go` (Plan, Hash, Screen, Clip, DeriveKind), `store.go`, `query.go`, `plan_test.go`, `store_integration_test.go` and `README.md`; the AI seam test is `services/ai/conversation_test.go`. The dashboard adds `views/conversations/{ArchiveButton.tsx, ConversationList.tsx, JobConversationCard.tsx}` (shared pieces) and `views/history/ConversationsTab.tsx`; `views/knowledge/api.ts` gains `JobEvents.job_type` (already sent by the backend). The e2e specs are `tests/api/conversations.spec.ts` and `tests/dashboard/archive.spec.ts`; `layout-width.spec.ts` covers `/archive`. The prune log route is `/api/conversations/prunes` and reads `ai_conversation_audit` (11.8).
