# Go incremental migration

The Go service starts as a separate, database-free liveness and database-aware readiness slice, bound to `127.0.0.1:8888`. Express remains the only production API and routing stays unchanged. CI exercises this Go code independently; this slice does not build, publish, or deploy a Go image.

## Route migration order

1. **Health:** keep `/health` process-only and `/ready` dependent on a real PostgreSQL `SELECT 1`. Missing or invalid `DATABASE_URL`, connection failures, and timeouts make readiness return 503. Never report unconfigured readiness as healthy or expose connection details.
2. **Authenticated reads:** migrate one route at a time after authentication/session parity, authorization ownership checks, response shape, and error behavior have contract tests against Express.
3. **Single-writer writes:** migrate writes only after the same parity suite exists and traffic ownership guarantees that Express and Go cannot write the same domain concurrently. Route one write path to one implementation at a time; no dual writes.

## Current authenticated read slice

`GET /custom-tags?scope=expense|income` uses the existing `demanage-token` cookie and verifies HS256 JWTs with the explicitly configured `JWT_SECRET`. It checks the user's current `sessionVersion` in PostgreSQL before parsing the query, matching Express's default of zero for an omitted token claim. The scope must occur once and match exactly. The read is constrained by the verified `userId` and scope, ordered by `name ASC`, and returns the complete Prisma `CustomTag` shape (`id`, `userId`, `scope`, `name`, `color`, `createdAt`, `updatedAt`). Database failures return a generic 500 body. The Go synthetic tests cover this contract; they do not replace a real PostgreSQL integration run.

The Go service shares one bounded `pgxpool` for its lifetime (`MaxConns=4`). `/ready` still performs a real `SELECT 1` under a two-second request timeout and fails closed if its database configuration or connection is unavailable. Each custom-tag database lookup retains a three-second timeout. The Go CI job runs a separate, mandatory integration test against an ephemeral PostgreSQL service, with fixed synthetic users and tags. Its route-only fixture is derived from the custom-tag and session-version migrations, but includes only the columns those reads use; it is not the complete Prisma schema.

## Data and contract authority

- `backend/prisma/schema.prisma` and its migrations remain the database schema authority. Go must not introduce independent schema changes or migrations.
- Preserve monetary values exactly. Do not pass financial amounts through `float32` or `float64`; use decimal strings or integer minor units at API boundaries and a decimal-capable representation for database arithmetic. Match Prisma decimal precision and rounding only after parity tests establish the current behavior.
- Every migrated route needs synthetic Go contract tests for status, stable headers, body fields, errors, and relevant authorization behavior, plus parity fixtures captured from the Express handler. Volatile fields (timestamps, generated ETags) should be shape-checked or normalized, not frozen.
- Express's weak ETag hashes the response bytes with SHA-1, base64-encodes it, and truncates the digest to 27 characters. The Go health handler preserves this form and honors matching `If-None-Match` on GET/HEAD with a bodyless 304.
- Health contract tests use injected synthetic probes. The separately tagged PostgreSQL integration test runs only in GitHub Actions with a dedicated test DSN, never `DATABASE_URL`; it is not a full Express differential suite.

## Validation handoff

Go was unavailable in the Windows PATH during implementation. The bootstrap module manifests were generated and verified in hosted run [45903566480](https://github.com/bielxdh3/demanage/actions/runs/45903566480); the current workflow requires `go mod tidy -diff`, `go mod verify`, formatting validation, synthetic tests, the tagged ephemeral-PostgreSQL integration test, and `go vet -mod=readonly ./...`. The integration fixture proves the selected SQL and route behavior against a route-only schema; it does not prove full Express differential parity or production performance. Those require separate differential fixtures and the benchmark gate below.

## Synthetic performance acceptance

Before routing production traffic to a migrated route, run a repeatable synthetic benchmark on the same CI runner and compare it with the Express baseline using identical fixtures and concurrency. Require no contract regressions, no errors over 10,000 requests, and Go p95 latency no more than 10% above the Express p95. Record runner, command, and results; these numbers are a routing gate, not a claim that local Windows performance is representative.

## Rollback

For this initial slice, production routing remains unchanged, so rollback is to stop evaluating the Go service and revert its isolated source, workflow, and documentation change. For later routes, switch the affected route back to Express as the sole writer/read handler, verify its existing contract suite, then remove Go routing only after traffic has returned. Do not roll back or edit Prisma schema as part of a Go route rollback.
