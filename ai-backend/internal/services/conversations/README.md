# services/conversations

## Overview
Stored AI conversations of [AI_CONVERSATIONS.md](../../../../docs/designs/AI_CONVERSATIONS.md). `ai.Service.ChatMessages` hands every call to `Store.SaveExchange` (installed with `SetConversationStore` in `cmd/server/main.go`); the HTTP routes are `internal/api/conversations.go`.

## Interface
- `Store{DB, Screen}.SaveExchange(ctx, ai.Exchange)` (`store.go`): one `db.WithTx`. `INSERT … ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)` on `(kind, ref)`, then `SELECT … FOR UPDATE`. It compares the call's sent messages with `last_hashes` (the previous call's sent messages and reply) through `Plan`, appends `sent[p:]`, then the reply (or an `error` note), with a `rebuilt` note first when the call did not extend the previous one. New activity on an archived conversation lists it again.
- `Plan(last, sent []string) (p int, rebuilt bool)`, `Hash(role, redacted)`, `Clip` (256 KiB), `DeriveKind` (`plan.go`): pure. Order per message: redact → hash (unclipped) → clip → store.
- `Screen` / `NewScreen(literals...)`: the `caseauto.Redactor` (literal secrets, bearer, GitHub, `gaos_` tokens, URL userinfo) plus storage-only patterns (PEM private keys, JWTs, `AKIA…`, `sk-…`, `password|secret|api_key|token[:=]value`). The extra patterns never touch what is sent to the LLM.
- `query.go`: `List` (live or archived, `kind`, `product`, `q` = exact ref or title), `Get`, `GetByRef`, `Messages(id, since)`, `Archive` (one conditional `UPDATE`: refused with `ErrActive` while the job is `queued`/`running` or the sync is `running` or has `ai_status = 'pending'`; `ErrNotFound`, `ErrAlreadyArchived`), `Unarchive` (`ErrNotArchived`), `Sizes`, `Preview` (counts and the newest `archived_at`), `Prune(cutoff)` (batches of 50 conversations, each its own transaction, messages by FK cascade; returns what was deleted, also after a failing batch), `Audit` / `AuditLog` (`ai_conversation_audit`, append-only).

## Invariants
- Never changes an AI call: the service calls the store with its own 5 s deadline inside `recover()`, and only logs errors.
- Prune deletes `ai_conversations` rows archived at or before the cut-off and their `ai_messages`; nothing else (`job_runs`, `job_events`, `case_approvals`, `pw_runs`, `kb_syncs`, `ai_call_log` and the audit table stay).
- `job_events`/`case_approvals` use the server's default collation; comparisons with `job_runs.id` carry `COLLATE utf8mb4_general_ci`.

## Side effects
Writes `ai_conversations`, `ai_messages`, `ai_conversation_audit`; reads `job_runs`, `kb_syncs`, `products`, `users`.

## Operations
InnoDB keeps the space of deleted rows: after a large prune, an operator may run `OPTIMIZE TABLE ai_messages` to give it back to the filesystem. Rough size: one build ≈ 280 KB (a ~40 KB prompt plus ~10 rounds).

## Testing
`go test -mod=mod ./internal/services/conversations/` (`Plan`, `Screen`, kinds, clipping). `store_integration_test.go` needs `QA_TEST_MYSQL_DSN`: a full case_create sequence stored once with kinds, an error then a retry, a resume (one `rebuilt` note, then rounds match again), redaction, archive guards (running job, pending sync summary), lists, a prune that keeps a conversation archived after the preview and keeps job/run/call-log rows.
