## Test Auth Endpoint Specification

### 1. Dev-Only Test Login Endpoint

Create an API endpoint at the EXACT path `/api/auth/test-login`.
This name is standardized — the automated test pipeline expects it at this exact path.

The endpoint:
- Accepts POST with JSON body: `{ "email": "user@example.com" }`
- Looks up user by email and authenticates using the project's normal auth mechanism
  (session, JWT, etc.) — no password required
- Returns JSON response: `{ "token": "...", "user": { "id": "...", "email": "..." } }`
  The token MUST be at the top level of the response
- Also sets any cookies/session headers the browser needs (for Playwright)
- MUST ONLY work in development/test mode — disabled/404 in production
  (use env vars, build flags, or stack-appropriate guards)

### 2. Playwright Auth Setup (frontend repos only)

- Create setup file: `tests/e2e/auth.setup.ts` (runs BEFORE all tests)
- Setup hits `/api/auth/test-login` with `TEST_USER_EMAIL` from env
- Saves authenticated browser state to `tests/e2e/.auth/user.json`
- Update `playwright.config.ts`:
  - Add "setup" project that runs auth setup first
  - All other test projects depend on "setup"
  - Configure `storageState` so every test starts authenticated

### 3. Test Credentials via Environment

- Test user email from `TEST_USER_EMAIL` env var
- Store in `.env.test` (NOT `.env`)
- Add `.env.test` to `.gitignore`
- Document how to create a test user (seed script, manual, or auto-creation)

### 4. Documentation in CLAUDE.md (REQUIRED — exact format)

Add this EXACT section to the repo's CLAUDE.md:

## Test Auth

**Endpoint**: `POST /api/auth/test-login`
**Body**: `{ "email": "..." }`
**Response**: `{ "token": "...", "user": { "id": "...", "email": "..." } }`
**Guard**: {describe how it's disabled in production}
**Env vars**: `TEST_USER_EMAIL`

### Who uses this
- **API test agents**: call the endpoint via curl, extract the token
  from the response, use it as `Authorization: Bearer {token}` in
  subsequent requests
- **E2E test agents (Playwright)**: call the endpoint in the browser
  context, save the authenticated state (cookies/session) to
  `tests/e2e/.auth/user.json`, reuse across all test specs

### How it works
{Brief explanation of the auth flow: endpoint receives email →
authenticates user → returns token in body + sets session cookies →
API tests use the token, Playwright tests use the browser state}

### Playwright setup (if frontend exists)
- **Storage state**: `tests/e2e/.auth/user.json`
- **Setup file**: `tests/e2e/auth.setup.ts`
- Setup project runs first → all test projects depend on it

### Creating test users
{How to create a test user — seed command, manual step, etc.}

### Safety
This endpoint is dev/test ONLY. It does NOT exist in production.
{Explain the mechanism that prevents it from running in prod.}
