# Authentication & session (his-frontend + his-backend GraphQL)

End-to-end coverage of the login of HISv3: the GraphQL API of the `user` module on **his-backend** and
the login form, the session and the logout of **his-frontend**.

The user stories come from cluster `D-HISV3-AUTH` of the QA assessment
(`his-v3-ai/services-impl/202611/backend-rest-e2e-qa-assessment/data-integration/D-HISV3-AUTH/`).
Every case of the catalog is listed in [test-catalog/his-backend/auth.md](test-catalog/his-backend/auth.md).

## What is under test

| Story                         | Surface                                 | Spec                                                                                             |
| ----------------------------- | --------------------------------------- | ------------------------------------------------------------------------------------------------ |
| US-AUTH-01 authenticate       | GraphQL `userLogin`                     | `tests/api/graphql/his-backend/user/user.spec.ts`                                                |
| US-AUTH-02 session and token  | frontend request headers, expired token | `tests/web/his-frontend/auth/login.spec.ts`, `tests/web/his-frontend/auth/session.spec.ts`       |
| US-AUTH-03 error handling     | login form validation, server message   | `tests/web/his-frontend/auth/login.spec.ts`                                                      |
| US-AUTH-04 logout             | GraphQL `userLogout`, logout flow       | `tests/api/graphql/his-backend/user/user.spec.ts`, `tests/web/his-frontend/auth/session.spec.ts` |
| US-AUTH-07 impersonated login | practitioner field on the form          | `tests/web/his-frontend/auth/login.spec.ts`                                                      |

## Configuration

The suite reads everything through `config/env.ts`; the committed templates are `env/.env.local.example`
and `env/.env.staging.example`. Copy them with `npm run env:init` and fill in:

| Variable                             | Meaning                                                                                                                                                                                                                                                  |
| ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `HIS_BACKEND_GRAPHQL_URL`            | Root of the GraphQL gateway. The client appends the module path, so the `user` module ends up at `<root>/user/query`.                                                                                                                                    |
| `HIS_FRONTEND_URL`                   | Base URL of the SPA. The login page is `<url>/auth/login`.                                                                                                                                                                                               |
| `HIS_AUTH_USERNAME`                  | Account used by the session setup and the auth specs.                                                                                                                                                                                                    |
| `HIS_AUTH_PASSWORD`                  | Password of that account. Keep it in the password manager — never in a committed file.                                                                                                                                                                   |
| `HIS_AUTH_PUBLIC_KEY`                | PEM public key that matches the **private** key of the backend under test (`VITE_APP_PUBLIC_KEY` in his-frontend). The backend rejects a plaintext password, so the GraphQL specs have to encrypt it the same way the browser does (`src/utils/rsa.ts`). |
| `HIS_AUTH_IMPERSONATED_PRACTITIONER` | Person name of the practitioner for the impersonated login.                                                                                                                                                                                              |

`PROTECTED_HOSTS` must list the production hostnames on every target except `local`; the suite refuses
to start when a configured URL points at one of them.

### Credentials

Without `HIS_AUTH_USERNAME` / `HIS_AUTH_PASSWORD` the cases that need a real account are **skipped** with
a reason that names the missing variable — they do not fail and they do not pass silently. The setup
project writes an empty storage state in that case, and the affected specs skip themselves through
`skipUnlessSignedIn` (see `src/utils/guards.ts`).

The password never reaches the repository: it lives in the environment only. The placeholder
`__FROM_PASSWORD_MANAGER__` of the templates counts as "not configured".

With the account configured, all 23 cases run: 22 of them are green, the session-expired notice
(`TC-AUTH-022`) is the one exception — see the findings below.

### Dedicated accounts per role

The account of the session setup is the shared one — in practice the superadmin. A spec that has to
prove what a **specific role** may see signs in on demand with a dedicated account instead:

```json
// test-data/private/credentials.local.json (git-ignored, filled from the password manager)
[{ "role": "nurse", "username": "qa.nurse", "password": "<from the password manager>" }]
```

One file per environment (`credentials.local.json`, `credentials.staging.json`); the committed
templates are `test-data/private/credentials.<env>.example.json` and `npm run env:init` copies them.
The file is read from disk at runtime, never imported, so a password cannot reach a report or a
prompt.

```ts
/** The role of this flow. */
const ROLE = 'nurse';

test.describe('his-frontend · auth · role session', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test('a dedicated role account signs in and gets its own session', async ({ hisFrontend }) => {
    skipUnlessRoleConfigured(ROLE); // skips with a reason when the role is not configured
    await hisFrontend.signInAsRole(ROLE);
    // …assert what the role may see
  });
});
```

The role belongs to the flow: every spec declares the role it acts as, and the API clients take it
as an argument (`hisBackendPatient(ROLE)`) — there is no global role constant. A spec that needs
another account declares another role next to it.

`signInAsRole` drops the session of the setup project first (the frontend signs out by clearing
`localStorage`), signs in through the real login form — the backend rejects a plaintext password, so
the browser has to do the RSA encryption — and waits for the signed-in shell.

Rules:

- one account per role, never a personal account; the role name is the key the specs use;
- a role that is missing from the file — or whose password is still the `__FROM_PASSWORD_MANAGER__`
  placeholder — counts as "not configured": the spec skips itself with a reason;
- a malformed file is an **error**, not a silent skip: a broken setup must not look like a green run;
- the file belongs to the environment — a role is provisioned in the target database (an access group
  and its menu) before it is listed here.

The patient-registration (PATREG) and outpatient-registration (OUTREG) clusters use this: they act as
the `admission` role, not as the superadmin of the session setup — see
`docs/patient-registration-testing.md` and `docs/outpatient-registration-testing.md`.

The same file is the fallback for the chart-management roles (`author`, `author2`, `viewer`), so every
dedicated account of the suite lives in one place; the `CHARTMGMT_*` environment variables keep working
and win when both are set.

## Running

```bash
npm run check                                     # typecheck + lint + format
TEST_ENV=staging npx playwright test --project=api-graphql
TEST_ENV=staging npx playwright test --project="web-his-frontend-chromium"
TEST_ENV=staging npx playwright test --project="web-*" --repeat-each=3
```

The web project `web-his-frontend-<browser>` depends on `setup-his-frontend`, which logs in through the
real login page once and stores the browser state in `.auth/his-frontend.json` (git-ignored).

Last verified on staging, with the account configured:

| Run                                                           | Result                                                 |
| ------------------------------------------------------------- | ------------------------------------------------------ |
| `--project=api-graphql --project="web-his-frontend-chromium"` | 24 passed (23 cases + the setup project), 0 failed     |
| the same with `--repeat-each=3`                               | 69 passed, 1 failed — `TC-AUTH-022`, flaky (see below) |

## Findings of this suite

### The session-expired notice is dropped when the redirect happens

`TC-AUTH-022` — a session that expires while the app is running must show the notice `Sesi telah habis`
on the login page (US-D-HISV3-AUTH-02). Observed on staging: the browser is sent back to `/auth/login`
and the token is removed from `localStorage`, but the notice does not always appear.

Measured on staging (`his-frontend` @ `hotfix/change-graph-practitioner-dropdown-ro-051026`):

| Scenario                                                   | Result                          |
| ---------------------------------------------------------- | ------------------------------- |
| Cold browser context, one run after another                | notice missing in 8 of 8 runs   |
| Under Playwright, `--repeat-each=8`, no account configured | notice missing in 3 of 8 runs   |
| Under Playwright, `--repeat-each=8`, account configured    | notice missing in 2 of 8 runs   |
| Full suite, `--repeat-each=3`, account configured          | 69 passed, 1 failed — this case |

The flag itself works: opening `/auth/login` directly with `sessionStorage.SESSION_EXPIRED` set shows the
notice. The notice is lost when the login page is reached through the redirect of the expired session —
`src/utils/gql.ts` publishes the toast while the app reboots at `/auth/login`, and sonner drops a toast
that is published before its toaster has subscribed. Whether the toast wins that race depends on how fast
the login chunk loads, which is why the case is flaky instead of consistently red.

The fix belongs in the application (`his-frontend`), not in this test: the notice has to be published
after the toaster is mounted, or the flag has to be read on the page that survives the redirect. The case
stays in the catalog as `blocked` until the behaviour is corrected — expect it to fail intermittently in
the meantime, which is the signal for the frontend team.

### The REST login endpoint still issues a token without checking the password

`POST /api/v1/auth/login` on his-backend answers with a valid JWT for a hardcoded user, whatever the
credentials are (finding AUTH-01 of `D-HISV3-AUTH`). The GraphQL login is not affected — it compares the
password against the bcrypt hash and answers with one generic message. The REST endpoint is outside the
scope of this epic and is kept in the catalog as `planned` (TC-AUTH-024).
