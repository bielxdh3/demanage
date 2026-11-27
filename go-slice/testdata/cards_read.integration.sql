CREATE TEMP TABLE "Card" (
  "id" text PRIMARY KEY,
  "userId" text NOT NULL,
  "name" text NOT NULL,
  "limit" numeric(12, 2),
  "closingDay" integer NOT NULL,
  "pendingClosingDay" integer,
  "archivedAt" timestamp(3),
  "expiresAt" timestamp(3),
  "lastInvoicedOn" timestamp(3),
  "lastBillingProcessedAt" timestamp(3),
  "minimumNextClosingOn" timestamp(3),
  "createdAt" timestamp(3) NOT NULL,
  "updatedAt" timestamp(3) NOT NULL
);
CREATE TEMP TABLE "Expense" (
  "id" text PRIMARY KEY,
  "userId" text NOT NULL,
  "cardId" text,
  "amount" numeric(12, 2) NOT NULL,
  "isInvoice" boolean NOT NULL,
  "frequency" text NOT NULL,
  "dueDay" integer,
  "startsAt" timestamp(3),
  "endsAt" timestamp(3),
  "occurredAt" timestamp(3),
  "archivedAt" timestamp(3),
  "systemOrigin" text NOT NULL,
  "createdAt" timestamp(3) NOT NULL
);
CREATE TEMP TABLE "ExpenseSplit" (
  "id" text PRIMARY KEY,
  "expenseId" text NOT NULL,
  "kind" text NOT NULL,
  "cardId" text,
  "amount" numeric(12, 2) NOT NULL
);
CREATE TEMP TABLE "ExpenseObligationVersion" (
  "id" text PRIMARY KEY,
  "expenseId" text NOT NULL,
  "sequence" integer NOT NULL,
  "effectiveMonth" text NOT NULL,
  "amount" numeric(12, 2) NOT NULL,
  "cardId" text,
  "splits" jsonb NOT NULL
);
INSERT INTO "Card" (
  "id", "userId", "name", "limit", "closingDay", "pendingClosingDay",
  "archivedAt", "expiresAt", "lastInvoicedOn", "lastBillingProcessedAt",
  "createdAt", "updatedAt"
) VALUES
  ('card-a-old', 'synthetic-owner-a', 'Old', 200.00, 5, 8, NULL, '2026-06-05 12:00:00', '2026-05-05 12:00:00', '2026-05-06 12:00:00', '2026-01-01 12:00:00', '2026-01-02 12:00:00'),
  ('card-a-new', 'synthetic-owner-a', 'New', NULL, 10, NULL, NULL, NULL, NULL, NULL, '2026-02-01 12:00:00', '2026-02-02 12:00:00'),
  ('card-a-archived', 'synthetic-owner-a', 'Archived', 500.00, 10, NULL, '2026-06-04 12:00:00', NULL, NULL, NULL, '2026-06-03 12:00:00', '2026-06-04 12:00:00'),
  ('card-b', 'synthetic-owner-b', 'Other owner', 100.00, 5, NULL, NULL, NULL, '2026-05-05 12:00:00', '2026-05-06 12:00:00', '2026-03-01 12:00:00', '2026-03-02 12:00:00');
INSERT INTO "Expense" (
  "id", "userId", "cardId", "amount", "isInvoice", "frequency", "dueDay",
  "startsAt", "occurredAt", "systemOrigin", "createdAt"
) VALUES
  ('expense-a-monthly', 'synthetic-owner-a', 'card-a-old', 20.00, false, 'mensal', 10, '2026-05-10 00:00:00', NULL, 'manual', '2026-05-01 12:00:00'),
  ('expense-a-split', 'synthetic-owner-a', NULL, 3.00, false, 'unica', NULL, NULL, '2026-06-06 02:30:00', 'manual', '2026-06-06 02:30:00'),
  ('expense-a-late', 'synthetic-owner-a', 'card-a-old', 1.23, false, 'unica', NULL, NULL, '2026-05-05 12:00:00', 'manual', '2026-05-08 12:00:00'),
  ('expense-b-card-a', 'synthetic-owner-b', 'card-a-old', 999.99, false, 'unica', NULL, NULL, '2026-06-01 12:00:00', 'manual', '2026-06-01 12:00:00');
INSERT INTO "ExpenseSplit" ("id", "expenseId", "kind", "cardId", "amount") VALUES
  ('split-a-card', 'expense-a-split', 'card', 'card-a-old', 2.34),
  ('split-a-pix', 'expense-a-split', 'pix', NULL, 0.66);
