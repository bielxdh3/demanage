CREATE TEMP TABLE "Expense" (
	"id" TEXT NOT NULL PRIMARY KEY,
	"userId" TEXT NOT NULL,
	"cardId" TEXT,
	"customTagId" TEXT,
	"name" TEXT NOT NULL,
	"amount" DECIMAL(12,2) NOT NULL,
	"category" TEXT NOT NULL,
	"frequency" TEXT NOT NULL,
	"dueDay" INTEGER,
	"startsAt" TIMESTAMP(3),
	"endsAt" TIMESTAMP(3),
	"occurredAt" TIMESTAMP(3),
	"paidForMonth" TEXT,
	"paidAt" TIMESTAMP(3),
	"notes" TEXT,
	"isInvoice" BOOLEAN NOT NULL,
	"billingPeriodStart" TIMESTAMP(3),
	"billingPeriodEnd" TIMESTAMP(3),
	"archivedAt" TIMESTAMP(3),
	"voidedAt" TIMESTAMP(3),
	"systemOrigin" TEXT NOT NULL,
	"createdAt" TIMESTAMP(3) NOT NULL,
	"updatedAt" TIMESTAMP(3) NOT NULL
);

CREATE TEMP TABLE "ExpenseSplit" (
	"id" TEXT NOT NULL PRIMARY KEY,
	"expenseId" TEXT NOT NULL,
	"kind" TEXT NOT NULL,
	"cardId" TEXT,
	"percent" DECIMAL(5,2) NOT NULL,
	"amount" DECIMAL(12,2) NOT NULL
);

CREATE TEMP TABLE "ExpensePayment" (
	"id" TEXT NOT NULL PRIMARY KEY,
	"expenseId" TEXT NOT NULL,
	"month" TEXT NOT NULL,
	"amount" DECIMAL(12,2) NOT NULL,
	"paidAt" TIMESTAMP(3) NOT NULL,
	"createdAt" TIMESTAMP(3) NOT NULL
);

CREATE TEMP TABLE "Entry" (
	"id" TEXT NOT NULL PRIMARY KEY,
	"userId" TEXT NOT NULL,
	"customTagId" TEXT,
	"name" TEXT NOT NULL,
	"notes" TEXT,
	"amount" DECIMAL(12,2) NOT NULL,
	"type" TEXT NOT NULL,
	"frequency" TEXT NOT NULL,
	"receiveDay" INTEGER,
	"startsAt" TIMESTAMP(3),
	"endsAt" TIMESTAMP(3),
	"date" TIMESTAMP(3),
	"receiptHoldForMonth" TEXT,
	"receivedForMonth" TEXT,
	"receivedAt" TIMESTAMP(3),
	"archivedAt" TIMESTAMP(3),
	"systemOrigin" TEXT NOT NULL,
	"createdAt" TIMESTAMP(3) NOT NULL,
	"updatedAt" TIMESTAMP(3) NOT NULL
);

CREATE TEMP TABLE "Card" (
	"id" TEXT NOT NULL PRIMARY KEY,
	"userId" TEXT NOT NULL,
	"name" TEXT NOT NULL
);

CREATE TEMP TABLE "CustomTag" (
	"id" TEXT NOT NULL PRIMARY KEY,
	"userId" TEXT NOT NULL,
	"scope" TEXT NOT NULL,
	"name" TEXT NOT NULL,
	"color" TEXT NOT NULL
);

CREATE TEMP TABLE "EntryReceipt" (
	"id" TEXT NOT NULL PRIMARY KEY,
	"entryId" TEXT NOT NULL,
	"month" TEXT NOT NULL,
	"amount" DECIMAL(12,2) NOT NULL,
	"receivedAt" TIMESTAMP(3) NOT NULL,
	"createdAt" TIMESTAMP(3) NOT NULL
);

CREATE TEMP TABLE "SalaryVersion" (
	"id" TEXT NOT NULL PRIMARY KEY,
	"entryId" TEXT NOT NULL,
	"month" TEXT NOT NULL,
	"amount" DECIMAL(12,2) NOT NULL,
	"receiveDay" INTEGER NOT NULL,
	"createdAt" TIMESTAMP(3) NOT NULL
);

CREATE TEMP TABLE "SalaryHold" (
	"id" TEXT NOT NULL PRIMARY KEY,
	"entryId" TEXT NOT NULL,
	"month" TEXT NOT NULL,
	"createdAt" TIMESTAMP(3) NOT NULL
);

INSERT INTO "Card" ("id", "userId", "name") VALUES
	('card-owner-a', 'synthetic-owner-a', 'Owner card'),
	('card-owner-b', 'synthetic-owner-b', 'Foreign card');

INSERT INTO "CustomTag" ("id", "userId", "scope", "name", "color") VALUES
	('tag-expense-a', 'synthetic-owner-a', 'expense', 'Mercado', '#AABBCC'),
	('tag-income-a', 'synthetic-owner-a', 'income', 'Extra', '#BBCCDD'),
	('tag-wrong-expense-scope', 'synthetic-owner-a', 'income', 'Income only', '#CCDDEE'),
	('tag-wrong-income-scope', 'synthetic-owner-a', 'expense', 'Expense only', '#DDEEFF'),
	('tag-foreign', 'synthetic-owner-b', 'expense', 'Foreign tag', '#FF0000');

INSERT INTO "Expense" (
	"id", "userId", "cardId", "customTagId", "name", "amount", "category", "frequency",
	"dueDay", "startsAt", "endsAt", "occurredAt", "paidForMonth", "paidAt", "notes", "isInvoice",
	"billingPeriodStart", "billingPeriodEnd", "archivedAt", "voidedAt", "systemOrigin", "createdAt", "updatedAt"
) VALUES
	('expense-z-archived', 'synthetic-owner-a', NULL, 'tag-foreign', 'Archived manual expense', 9999999999.99, 'outro', 'unica', NULL, NULL, NULL, '2026-11-05 12:00:00.000', NULL, NULL, NULL, FALSE, NULL, NULL, '2026-11-08 00:00:00.000', NULL, 'manual', '2026-11-05 12:00:00.000', '2026-11-08 00:00:00.000'),
	('expense-m-tie', 'synthetic-owner-a', NULL, 'tag-wrong-expense-scope', 'Middle tie', 0.10, 'outro', 'unica', NULL, NULL, NULL, '2026-11-05 12:00:00.000', NULL, NULL, NULL, FALSE, NULL, NULL, NULL, NULL, 'manual', '2026-11-05 12:00:00.000', '2026-11-05 12:00:00.000'),
	('expense-a-tie', 'synthetic-owner-a', NULL, 'tag-expense-a', 'Earlier tie', 1.25, 'outro', 'unica', NULL, NULL, NULL, '2026-11-05 12:00:00.000', NULL, NULL, NULL, FALSE, NULL, NULL, NULL, NULL, 'manual', '2026-11-05 12:00:00.000', '2026-11-05 12:00:00.000'),
	('expense-older', 'synthetic-owner-a', NULL, 'tag-expense-a', 'Older manual', 2.00, 'outro', 'unica', NULL, NULL, NULL, '2026-11-04 12:00:00.000', NULL, NULL, NULL, FALSE, NULL, NULL, NULL, NULL, 'manual', '2026-11-04 12:00:00.000', '2026-11-04 12:00:00.000'),
	('expense-voided', 'synthetic-owner-a', NULL, NULL, 'Voided manual', 85.00, 'outro', 'unica', NULL, NULL, NULL, '2026-11-09 12:00:00.000', NULL, NULL, NULL, FALSE, NULL, NULL, '2026-11-09 13:00:00.000', '2026-11-09 13:00:00.000', 'manual', '2026-11-09 12:00:00.000', '2026-11-09 13:00:00.000'),
	('expense-system', 'synthetic-owner-a', NULL, NULL, 'System asset expense', 100.00, 'investimento', 'unica', NULL, NULL, NULL, '2026-11-09 12:00:00.000', NULL, NULL, NULL, FALSE, NULL, NULL, NULL, NULL, 'asset', '2026-11-09 12:00:00.000', '2026-11-09 12:00:00.000'),
	('expense-foreign', 'synthetic-owner-b', NULL, NULL, 'Other owner expense', 200.00, 'outro', 'unica', NULL, NULL, NULL, '2026-11-09 12:00:00.000', NULL, NULL, NULL, FALSE, NULL, NULL, NULL, NULL, 'manual', '2026-11-09 12:00:00.000', '2026-11-09 12:00:00.000'),
	('expense-after-snapshot', 'synthetic-owner-a', NULL, NULL, 'After snapshot', 300.00, 'outro', 'unica', NULL, NULL, NULL, '2026-11-11 12:00:00.000', NULL, NULL, NULL, FALSE, NULL, NULL, NULL, NULL, 'manual', '2026-11-11 12:00:00.000', '2026-11-11 12:00:00.000');

INSERT INTO "ExpenseSplit" ("id", "expenseId", "kind", "cardId", "percent", "amount") VALUES
	('split-card-owner', 'expense-z-archived', 'card', 'card-owner-a', 60.00, 6.00),
	('split-card-foreign', 'expense-z-archived', 'card', 'card-owner-b', 40.00, 4.00),
	('split-pix', 'expense-z-archived', 'pix', NULL, 0.00, 0.00);

INSERT INTO "ExpensePayment" ("id", "expenseId", "month", "amount", "paidAt", "createdAt") VALUES
	('payment-second', 'expense-z-archived', '2026-11', 0.20, '2026-11-06 10:00:00.000', '2026-11-07 10:00:00.000'),
	('payment-first', 'expense-z-archived', '2026-10', 0.10, '2026-11-06 10:00:00.000', '2026-11-06 10:00:00.000');

INSERT INTO "Entry" (
	"id", "userId", "customTagId", "name", "notes", "amount", "type", "frequency", "receiveDay",
	"startsAt", "endsAt", "date", "receiptHoldForMonth", "receivedForMonth", "receivedAt", "archivedAt",
	"systemOrigin", "createdAt", "updatedAt"
) VALUES
	('entry-z-archived', 'synthetic-owner-a', 'tag-foreign', 'Archived salary', NULL, 9999999999.99, 'salario', 'mensal', 5, '2026-01-05 00:00:00.000', NULL, NULL, '2026-12', NULL, NULL, '2026-11-08 00:00:00.000', 'manual', '2026-11-05 12:00:00.000', '2026-11-08 00:00:00.000'),
	('entry-m-tie', 'synthetic-owner-a', 'tag-wrong-income-scope', 'Middle income tie', 'notes', 0.10, 'outro', 'unica', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 'manual', '2026-11-05 12:00:00.000', '2026-11-05 12:00:00.000'),
	('entry-a-tie', 'synthetic-owner-a', 'tag-income-a', 'Earlier income tie', '', 1.25, 'outro', 'unica', NULL, NULL, NULL, '2026-11-05 12:00:00.000', NULL, NULL, NULL, NULL, 'manual', '2026-11-05 12:00:00.000', '2026-11-05 12:00:00.000'),
	('entry-older', 'synthetic-owner-a', NULL, 'Older income', NULL, 2.00, 'outro', 'unica', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 'manual', '2026-11-04 12:00:00.000', '2026-11-04 12:00:00.000'),
	('entry-foreign', 'synthetic-owner-b', NULL, 'Other owner income', NULL, 200.00, 'outro', 'unica', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 'manual', '2026-11-09 12:00:00.000', '2026-11-09 12:00:00.000'),
	('entry-system', 'synthetic-owner-a', NULL, 'System income', NULL, 100.00, 'outro', 'unica', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 'piggy', '2026-11-09 12:00:00.000', '2026-11-09 12:00:00.000'),
	('entry-after-snapshot', 'synthetic-owner-a', NULL, 'After snapshot income', NULL, 300.00, 'outro', 'unica', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 'manual', '2026-11-11 12:00:00.000', '2026-11-11 12:00:00.000');

INSERT INTO "SalaryHold" ("id", "entryId", "month", "createdAt") VALUES
	('hold-dec', 'entry-z-archived', '2026-12', '2026-12-01 00:00:00.000'),
	('hold-nov', 'entry-z-archived', '2026-11', '2026-11-01 00:00:00.000');

INSERT INTO "SalaryVersion" ("id", "entryId", "month", "amount", "receiveDay", "createdAt") VALUES
	('version-dec', 'entry-z-archived', '2026-12', 9999999999.99, 5, '2026-12-01 00:00:00.000'),
	('version-nov', 'entry-z-archived', '2026-11', 1234567890.12, 4, '2026-11-01 00:00:00.000');

INSERT INTO "EntryReceipt" ("id", "entryId", "month", "amount", "receivedAt", "createdAt") VALUES
	('receipt-second', 'entry-z-archived', '2026-11', 0.20, '2026-11-08 12:00:00.000', '2026-11-08 12:00:00.000'),
	('receipt-first', 'entry-z-archived', '2026-10', 0.10, '2026-11-08 12:00:00.000', '2026-11-07 12:00:00.000');
