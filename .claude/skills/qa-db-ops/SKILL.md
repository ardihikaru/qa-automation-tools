---
name: qa-db-ops
description: Operate the qa-automation MySQL database (schema qa_automation on the shared rnd-v2 master-compose MySQL, 10.10.0.4:5306 over WireGuard). Use for migrations, schema changes, ad-hoc queries, backups/dumps, restores, row-count checks, or credential rotation — without ever printing secrets.
---

# qa_automation database ops

- **Server:** MySQL 8.0.33, container `master-compose-ai-mysqldb-master-1` on **rnd-v2-vpn**, published at `10.10.0.4:5306` (reachable only over WireGuard).
- **This app owns only** the schema `qa_automation`, used by user `qa_automation@%`. The server is shared with other projects (ai-manager's `ai_manager`, ...), so never touch other schemas.
- **App credentials:** `DB_PASSWORD` in `deploy/his-v3-qa/.env.prod` on rnd-v3.
- **Root credentials:** `MYSQL_ROOT_PASSWORD` in `~/devel/projects/infra-prod/.env` on rnd-v2 (ssh user `sismedika`). Use root only for user and grant changes.
- The old source DB was `202.10.39.46:31006/qa_automation`. It was migrated on 2026-11-13 and the per-table row counts matched. The local `.env` `MYSQL_*` variables point at the **HISv3 production** schema on that server. Never use them for this app.

## Load credentials without printing them
```bash
# app user, from rnd-v3's .env.prod
export MYSQL_PWD="$(ssh rnd-v3-vpn 'grep -E "^DB_PASSWORD=" ~/devel/projects/qa-automation-tools/deploy/his-v3-qa/.env.prod | cut -d= -f2-')"
mysql -h 10.10.0.4 -P 5306 -u qa_automation qa_automation -e 'SHOW TABLES'
```
Root, run on rnd-v2 only:
```bash
ssh rnd-v2-vpn 'cd ~/devel/projects/infra-prod && R=$(grep -E "^MYSQL_ROOT_PASSWORD=" .env | cut -d= -f2-) && docker exec -i -e MYSQL_PWD="$R" master-compose-ai-mysqldb-master-1 mysql -uroot -e "SELECT 1"'
```

## Migrations
1. Append an idempotent statement to `ai-backend/internal/db/migrations.go`.
2. After running the load-credentials block above (`MYSQL_PWD` is the mysql client's own variable, not one of the forbidden `.env` `MYSQL_*` keys). `DATABASE_URL=` is blanked so a local `.env` value can't override `DB_*`:
   `cd ai-backend && DB_HOST=10.10.0.4 DB_PORT=5306 DB_USER=qa_automation DB_PASSWORD="$MYSQL_PWD" DB_NAME=qa_automation DATABASE_URL= go run -mod=mod ./cmd/databasemigration`
3. Deploy the backend afterwards (`deploy-his-v3-qa`).

## Backup and restore
```bash
mysqldump -h 10.10.0.4 -P 5306 -u qa_automation --single-transaction --routines --triggers --no-tablespaces --set-gtid-purged=OFF --column-statistics=0 qa_automation > qa_automation-$(date +%F).sql
mysql -h 10.10.0.4 -P 5306 -u qa_automation qa_automation < qa_automation-YYYY-MM-DD.sql
```
Keep dumps out of the repo; they contain user records and bcrypt hashes. Use a scratch directory with `umask 077`.

## Row-count check
```sql
SET SESSION group_concat_max_len = 100000;
SELECT GROUP_CONCAT(CONCAT('SELECT ''',table_name,''', COUNT(*) FROM `',table_name,'`') SEPARATOR ' UNION ALL ')
FROM information_schema.tables WHERE table_schema='qa_automation' AND table_type='BASE TABLE';
-- run the generated statement
```

## Rotate the app password
Generate a password, then run `ALTER USER 'qa_automation'@'%' IDENTIFIED BY '…'` as root, piping the new value over ssh stdin. Update `DB_PASSWORD` in `.env.prod` the same way, then run `deploy.sh`. Confirm that `/health` shows `database: connected`.
