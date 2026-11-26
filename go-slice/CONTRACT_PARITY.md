# Express and Go contract comparison

The hosted `express-go-contract` job validates the exact loopback DSN and `demanage_parity_test` database name before invoking Prisma or importing the Express application. It then compares the real Express `createApp()` application with the Go HTTP server against that disposable PostgreSQL database after applying the checked-in Prisma migrations. Its synthetic users and tags use fixed, namespaced IDs and `.invalid` email addresses. No production or developer database is read or modified.

The contract checks `/health`, successful `/ready`, authenticated expense and income tag reads, alphabetical ordering, owner isolation, missing authentication, missing scope, and invalid scope. It compares HTTP status and JSON; `/health.timestamp` is checked for a valid date and then normalized. Fixture IDs and timestamps are deterministic, so custom-tag JSON remains directly comparable.

HTTP headers are outside this comparison because they include framework and middleware behavior. Express adds `Cache-Control: no-store` and `Permissions-Policy`; those application middleware headers are not yet mirrored by this Go slice. Go's focused tests separately check the Express-style JSON content type, ETag, and `X-Powered-By` response behavior. This contract job does not relax any existing CI gate.

The dedicated hosted workflow runs `pnpm test:go-contract:load` after building the Go binary and applying migrations to `demanage_parity_test`. That command includes all contract cases, then runs 100 synthetic requests per implementation at concurrency 10 and reports p50, p95, and error totals. These are comparative measurements, not a performance claim, until a hosted run is available. `pnpm test:go-contract` runs only the parity cases.
