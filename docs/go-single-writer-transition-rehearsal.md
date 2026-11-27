# Synthetic single-writer transition rehearsal

This runbook documents a test-only rehearsal for the `custom-tags` domain. It models one active writer identity at a time, a fail-closed drain, a switch to Go, rollback to Express, and exact synthetic reconciliation. It changes no route, schema, workflow, or production behavior.

## What this checkout proves

| Area | Verified local behavior | What remains unproven |
| --- | --- | --- |
| Schema | [`CustomTag`](../backend/prisma/schema.prisma) stores owner, scope, name, color, and timestamps. Expenses and entries reference tags; deleting a tag sets those references to null. The Prisma migrations define the per-owner/scope/name uniqueness rules, including the normalized income-name index. | No transition fence or writer identity is represented in the schema. |
| Express | [`custom-tags.ts`](../backend/src/routes/custom-tags.ts) mounts authenticated GET, POST, and DELETE handlers; [`custom-tags.ts`](../backend/src/services/custom-tags.ts) reads, creates, and deletes tags. | This does not establish which deployment instances or external callers can reach the handlers. |
| Go | [`custom_tags.go`](../go-slice/custom_tags.go) registers GET only. [`main.go`](../go-slice/main.go) binds the Go process to `127.0.0.1:8888`. | There is no Go custom-tag write route or deployed writer fence in this checkout. |
| Production route config | [`docker-compose.prod.yml`](../docker-compose.prod.yml) points the frontend at the `backend` service on port 8888; that service is the Node backend. The existing [incremental migration guidance](go-incremental-migration.md) says production routing remains unchanged. | This local source/config inspection is not a live deployment audit. |
| Additional local tag creation | [`finance-tags.ts`](../backend/src/services/finance-tags.ts) can create an income tag inside a transaction; [`finance-operation.ts`](../backend/src/services/finance-operation.ts) calls it. This path must be included in any writer inventory. | The complete deployed inventory of HTTP routes, MCP operations, jobs, workers, scripts, direct database clients, and other processes has not been established. |

Custom tags contain no monetary amount, but they are referenced by financial rows. Treat the domain as lower impact than ledger writes, not as impact-free.

## Run the synthetic rehearsal

From `go-slice`, with the repository's Go version and dependencies already available:

```powershell
$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'off'
go test -count=1 -run '^TestCustomTag(Transition|WriterGate|Rollback)' .
```

The test-only gate starts with Express as the `custom-tags` writer. It rejects new acquisitions while draining, waits for the in-flight Express lease, then admits only Go. Rollback repeats the drain in the other direction. The in-memory store uses literal synthetic owners, scopes, names, colors, and operation IDs; reconciliation compares the complete expected tag set. An unknown response is modeled after the synthetic write has reached the store, so the rollback assertions catch automatic replay of both committed and uncertain operations.

## Required gates before any real cutover

1. **Inventory every writer for the domain.** Record each HTTP route and caller, the finance/MCP operation path, scheduled jobs and workers, maintenance scripts, administrative tools, database triggers/functions, and direct database clients. Include the paths already found in Express and `finance-tags.ts`. Confirm the inventory against deployed routing and runtime configuration; this checkout search alone is not complete evidence.
2. **Prove one writer identity per domain.** Define how every instance and non-HTTP path is fenced. The authority must cover multiple processes, replicas, restarts, and stale in-flight requests. The in-process test gate is only a toy model and does not prove deployed fencing.
3. **Prove contract and ownership parity.** Before Go writes are enabled, cover authenticated create/delete behavior, owner isolation, scope validation, duplicate-name rules, referenced-row effects, and error behavior. Keep Express and Go from writing the same domain concurrently; do not dual-write. Preserve the existing [synthetic performance gate](go-incremental-migration.md#synthetic-performance-acceptance): zero errors over 10,000 requests and Go p95 latency no more than 10% above the Express baseline.
4. **Cut over fail-closed.** Stop admitting mutations for the domain, reject requests during the drain, and wait for every old-writer operation to finish before granting the new identity. If fencing or drain status is unavailable, keep writes closed and abort the switch. Do not queue writes for later replay.
5. **Reconcile before resuming.** Compare exact tag IDs, owners, scopes, names, colors, and expected operation counts against the authoritative database and the change record. Investigate every mismatch. A timeout or lost response is an unknown outcome: determine its state from authoritative evidence; never blindly resend it.
6. **Rollback without replay.** Close Go admission, drain its in-flight work, then restore Express as the sole writer. Do not replay unknown or committed mutations. Reconcile the shared state before reopening writes; if state or ownership cannot be established, leave writes closed.

Any missing writer, stale writer, failed drain, or unreconciled outcome blocks cutover and rollback completion. A successful in-memory rehearsal is not authorization or evidence that a deployed transition is safe.

## Validation boundary

The Go rehearsal is test-only and in-memory. It does not exercise HTTP routing, PostgreSQL, a proxy or load balancer, multiple processes, workers, retries from clients, real idempotency, process crashes, production data, or a deployed fence. Passing it does not satisfy the route-parity or performance gates above.
