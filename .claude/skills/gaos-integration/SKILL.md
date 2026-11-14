---
name: gaos-integration
description: How qa-automation-tools talks to GAOS (gaos.sismedika.com) - product link (p-N ids, badge, status check), outbox push of cases and results, inbound routes, and the Inbox approval API used by case automation. Use when touching gaossync, the products GAOS badge, GAOS env vars/tokens, or the GAOS repo's inbox machine API.
---

# GAOS integration

Specs: `docs/designs/GAOS_INTEGRATION.md`, `GAOS_INBOUND.md`, `CASE_RESULTS.md`, `CASE_AUTOMATION.md` §6. Code: `ai-backend/internal/services/gaossync/` (`sync.go`, `sweeper.go` outbox, `link.go` link check/web URL, `inbox.go` Inbox client), `internal/api/products_gaos.go`, `gaos_inbound.go`, `gaos_inbox_inbound.go`. GAOS repo: `/home/ardi/devel/projects/ardihikaru/my-projects/gaos` (Go + HTMX, event-sourced; `dashboard/api_v1.go`, `external_inbox.go`, `inbox_worker.go`, design `docs/designs/qa-inbox-api.md`).

## Mapping and env
- Per product: `product_settings.gaos_integrated` + `gaos_product_id` (`p-N`; HISv3 = `p-6`). Only Admin/DevOps change them.
- Env: `GAOS_API_URL` (prod is an internal http address), `GAOS_API_TOKEN` (QA bearer, `gaos_`, ≥37 chars), `GAOS_INBOUND_TOKEN` (what GAOS presents to us), `GAOS_WEB_URL` (public `https://gaos.sismedika.com/app/`, what the badge links to). Never print tokens.
- GAOS side: `GAOS_API_QA_PRODUCTS` must list the product ids the QA token may touch; QA scope cannot call admin routes; one write in flight per token (429).

## Behaviour
- Push: cases (`POST /products/{pid}/qa/cases`), results (`.../qa/results`), durable outbox + sweeper; a failed push never affects `/health`.
- Verify the link: `GET /api/products/{id}/gaos/status` → `linked | not_scoped | unauthorized | unreachable | not_integrated` (read-only `GET /products` on GAOS). The `/products` badge shows it.
- Inbox: `POST /v1/products/{pid}/inbox` (idempotent on `externalId`), `GET .../inbox/{externalId}` (poll), webhook `POST /api/v1/gaos/inbox-answer` to us (bearer `GAOS_INBOUND_TOKEN`, idempotent, at-least-once). Treat unknown/expired as "still open"; the dashboard approval door always works if GAOS is down.

## Gotchas
- `cardsDeferred` from GAOS is a string, not a count.
- The GAOS back office cookie gate (`gaos_bo=1`) is not an integration channel.
- GAOS prod deploys follow that repo's `deploy/STACKS.md`; its design docs need a goldfish review before merge.
