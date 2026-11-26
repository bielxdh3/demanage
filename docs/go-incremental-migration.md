# Go incremental migration

The Go service starts as a separate, database-free liveness and database-aware readiness slice, bound to `127.0.0.1:8888`. Express remains the only production API and routing stays unchanged. CI exercises this Go code independently; this slice does not build, publish, or deploy a Go image.

## Route migration order

1. **Health:** keep `/health` process-only and `/ready` dependent on a real PostgreSQL `SELECT 1`. Missing or invalid `DATABASE_URL`, connection failures, and timeouts make readiness return 503. Never report unconfigured readiness as healthy or expose connection details.
2. **Authenticated reads:** migrate one route at a time after authentication/session parity, authorization ownership checks, response shape, and error behavior have contract tests against Express.
3. **Single-writer writes:** migrate writes only after the same parity suite exists and traffic ownership guarantees that Express and Go cannot write the same domain concurrently. Route one write path to one implementation at a time; no dual writes.

## Data and contract authority

- `backend/prisma/schema.prisma` and its migrations remain the database schema authority. Go must not introduce independent schema changes or migrations.
- Preserve monetary values exactly. Do not pass financial amounts through `float32` or `float64`; use decimal strings or integer minor units at API boundaries and a decimal-capable representation for database arithmetic. Match Prisma decimal precision and rounding only after parity tests establish the current behavior.
- Every migrated route needs synthetic Go contract tests for status, stable headers, body fields, errors, and relevant authorization behavior, plus parity fixtures captured from the Express handler. Volatile fields (timestamps, generated ETags) should be shape-checked or normalized, not frozen.
- Express's weak ETag hashes the response bytes with SHA-1, base64-encodes it, and truncates the digest to 27 characters. The Go health handler preserves this form and honors matching `If-None-Match` on GET/HEAD with a bodyless 304.
- Database connectivity tests use injected synthetic probes. Do not use a real database or credentials in the health contract tests.

## Validation handoff

Go was unavailable in the Windows PATH for this slice, and dependency downloads are intentionally left to hosted CI. From `go-slice/`, the hosted runner runs `go mod tidy`, then `test -z "$(gofmt -l .)"`, `go test ./...`, and `go vet ./...`. It uploads the generated public `go.mod` and `go.sum` as `go-module-files-bootstrap` so the owner can commit them before switching CI to readonly dependency validation. No checksums are fabricated or committed by this preparation.

## Synthetic performance acceptance

Before routing production traffic to a migrated route, run a repeatable synthetic benchmark on the same CI runner and compare it with the Express baseline using identical fixtures and concurrency. Require no contract regressions, no errors over 10,000 requests, and Go p95 latency no more than 10% above the Express p95. Record runner, command, and results; these numbers are a routing gate, not a claim that local Windows performance is representative.

## Rollback

For this initial slice, production routing remains unchanged, so rollback is to stop evaluating the Go service and revert its isolated source, workflow, and documentation change. For later routes, switch the affected route back to Express as the sole writer/read handler, verify its existing contract suite, then remove Go routing only after traffic has returned. Do not roll back or edit Prisma schema as part of a Go route rollback.
