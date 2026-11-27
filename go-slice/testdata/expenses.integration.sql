CREATE TEMP TABLE "Expense" (
    "id" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "cardId" TEXT,
    "customTagId" TEXT,
    "name" TEXT NOT NULL,
    "amount" NUMERIC(12, 2) NOT NULL,
    "category" TEXT NOT NULL,
    "frequency" TEXT NOT NULL,
    "dueDay" INTEGER,
    "startsAt" TIMESTAMP(3),
    "endsAt" TIMESTAMP(3),
    "occurredAt" TIMESTAMP(3),
    "paidForMonth" TEXT,
    "paidAt" TIMESTAMP(3),
    "notes" TEXT,
    "isInvoice" BOOLEAN NOT NULL DEFAULT FALSE,
    "billingPeriodStart" TIMESTAMP(3),
    "billingPeriodEnd" TIMESTAMP(3),
    "archivedAt" TIMESTAMP(3),
    "voidedAt" TIMESTAMP(3),
    "systemOrigin" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL,
    "updatedAt" TIMESTAMP(3) NOT NULL
) ON COMMIT DROP;

CREATE TEMP TABLE "CustomTag" (
    "id" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "scope" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "color" TEXT NOT NULL
) ON COMMIT DROP;

CREATE TEMP TABLE "Card" (
    "id" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "name" TEXT NOT NULL
) ON COMMIT DROP;

CREATE TEMP TABLE "ExpenseSplit" (
    "id" TEXT NOT NULL,
    "expenseId" TEXT NOT NULL,
    "kind" TEXT NOT NULL,
    "cardId" TEXT,
    "percent" NUMERIC(5, 2) NOT NULL,
    "amount" NUMERIC(12, 2) NOT NULL
) ON COMMIT DROP;

CREATE TEMP TABLE "ExpensePayment" (
    "id" TEXT NOT NULL,
    "expenseId" TEXT NOT NULL,
    "month" TEXT NOT NULL,
    "amount" NUMERIC(12, 2) NOT NULL,
    "paidAt" TIMESTAMP(3) NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL
) ON COMMIT DROP;

INSERT INTO "CustomTag" ("id", "userId", "scope", "name", "color") VALUES
    ('tag-a-expense', 'synthetic-owner-a', 'expense', 'Moradia', '#112233'),
    ('tag-b-expense', 'synthetic-owner-b', 'expense', 'Privado', '#445566');

INSERT INTO "Card" ("id", "userId", "name") VALUES
	('card-a', 'synthetic-owner-a', 'Azul'),
	('card-b', 'synthetic-owner-a', 'Verde');

INSERT INTO "Expense" (
    "id", "userId", "cardId", "customTagId", "name", "amount", "category", "frequency",
    "dueDay", "startsAt", "endsAt", "occurredAt", "paidForMonth", "paidAt", "notes",
    "isInvoice", "billingPeriodStart", "billingPeriodEnd", "archivedAt", "voidedAt",
    "systemOrigin", "createdAt", "updatedAt"
) VALUES
    ('manual-a-new', 'synthetic-owner-a', NULL, 'tag-a-expense', 'Aluguel dividido', 1234.56, 'outro', 'mensal',
     5, '2026-02-01 00:00:00.000', '2026-12-31 23:59:59.000', NULL, NULL, NULL, 'Conta da casa',
     FALSE, NULL, NULL, NULL, NULL, 'manual', '2026-02-05 06:07:08.009', '2026-02-05 06:07:08.009'),
    ('manual-a-zero', 'synthetic-owner-a', NULL, NULL, 'Despesa zero', 0.00, 'assinatura', 'unica',
     NULL, NULL, NULL, '2026-02-04 00:00:00.000', NULL, NULL, NULL,
     FALSE, NULL, NULL, NULL, NULL, 'manual', '2026-02-04 06:07:08.010', '2026-02-04 06:07:08.010'),
    ('manual-b-private', 'synthetic-owner-b', NULL, 'tag-b-expense', 'De outro titular', 90.11, 'outro', 'mensal',
     9, NULL, NULL, NULL, NULL, NULL, NULL, FALSE, NULL, NULL, NULL, NULL, 'manual',
     '2026-02-08 06:07:08.011', '2026-02-08 06:07:08.011'),
    ('archived-a', 'synthetic-owner-a', NULL, NULL, 'Arquivada', 88.22, 'outro', 'mensal',
     8, NULL, NULL, NULL, NULL, NULL, NULL, FALSE, NULL, NULL, '2026-02-07 00:00:00.000', NULL, 'manual',
     '2026-02-07 06:07:08.012', '2026-02-07 06:07:08.012'),
    ('system-a', 'synthetic-owner-a', NULL, NULL, 'Movimento do sistema', 77.33, 'cofrinho', 'unica',
     NULL, NULL, NULL, NULL, NULL, NULL, NULL, FALSE, NULL, NULL, NULL, NULL, 'piggy',
     '2026-02-06 06:07:08.013', '2026-02-06 06:07:08.013');

INSERT INTO "ExpenseSplit" ("id", "expenseId", "kind", "cardId", "percent", "amount") VALUES
    ('split-a-card', 'manual-a-new', 'card', 'card-a', 60.00, 740.74),
	('split-a-card-b', 'manual-a-new', 'card', 'card-b', 20.00, 246.91),
	('split-a-pix', 'manual-a-new', 'pix', NULL, 20.00, 246.91);

INSERT INTO "ExpensePayment" ("id", "expenseId", "month", "amount", "paidAt", "createdAt") VALUES
	('payment-a-earlier', 'manual-a-new', '2026-01', 5.00, '2026-02-01 10:00:00.000', '2026-02-01 10:00:00.000'),
	('payment-a-first', 'manual-a-new', '2026-02', 123.45, '2026-02-01 10:00:00.000', '2026-02-01 10:01:00.000'),
	('payment-a-second', 'manual-a-new', '2026-03', 0.10, '2026-02-02 10:00:00.000', '2026-02-01 09:00:00.000');
