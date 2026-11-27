CREATE TEMP TABLE "PiggyBank" (
	"id" TEXT NOT NULL PRIMARY KEY,
	"userId" TEXT NOT NULL,
	"archivedAt" TIMESTAMP(3)
);

CREATE TEMP TABLE "PiggyTransaction" (
	"id" TEXT NOT NULL PRIMARY KEY,
	"piggyBankId" TEXT NOT NULL,
	"userId" TEXT NOT NULL,
	"type" TEXT NOT NULL,
	"source" TEXT NOT NULL,
	"amount" NUMERIC(12,2) NOT NULL,
	"date" TIMESTAMP(3) NOT NULL,
	"expenseId" TEXT,
	"entryId" TEXT,
	"note" TEXT,
	"correctionOrigin" TEXT,
	"cdiRate" NUMERIC(12,8),
	"cdiPercent" NUMERIC(8,4),
	"baseBalance" NUMERIC(18,8),
	"resultingBalance" NUMERIC(18,8),
	"interestKey" TEXT UNIQUE,
	"createdAt" TIMESTAMP(3) NOT NULL
);

INSERT INTO "PiggyBank" ("id", "userId", "archivedAt") VALUES
	('bank-owner-a', 'synthetic-owner-a', NULL),
	('bank-owner-a-archived', 'synthetic-owner-a', '2026-04-01 00:00:00.000'),
	('bank-owner-a-empty', 'synthetic-owner-a', NULL),
	('bank-foreign', 'synthetic-owner-b', NULL);

INSERT INTO "PiggyTransaction" (
	"id", "piggyBankId", "userId", "type", "source", "amount", "date",
	"expenseId", "entryId", "note", "correctionOrigin", "cdiRate", "cdiPercent",
	"baseBalance", "resultingBalance", "interestKey", "createdAt"
) VALUES
	('tx-deposit-early', 'bank-owner-a', 'synthetic-owner-a', 'deposit', 'manual', 9999999999.99, '2026-05-01 12:00:00.000', 'expense-early', NULL, 'early deposit', NULL, NULL, NULL, NULL, NULL, NULL, '2026-05-01 08:00:00.000'),
	('tx-withdraw-linked', 'bank-owner-a', 'synthetic-owner-a', 'withdraw', 'manual', 0.10, '2026-05-01 12:00:00.000', NULL, 'entry-a', 'linked withdrawal', NULL, NULL, NULL, NULL, NULL, NULL, '2026-05-01 09:00:00.000'),
	('tx-wrong-link', 'bank-owner-a', 'synthetic-owner-a', 'withdraw', 'manual', 3.45, '2026-05-02 12:00:00.000', 'expense-wrong', NULL, 'withdrawal without entry', NULL, NULL, NULL, NULL, NULL, NULL, '2026-05-02 09:00:00.000'),
	('tx-unlinked-withdraw', 'bank-owner-a', 'synthetic-owner-a', 'withdraw', 'manual', 4.56, '2026-05-02 12:00:00.000', NULL, NULL, 'unlinked withdrawal', NULL, NULL, NULL, NULL, NULL, NULL, '2026-05-02 10:00:00.000'),
	('tx-unlinked-deposit', 'bank-owner-a', 'synthetic-owner-a', 'deposit', 'manual', 12.34, '2026-05-02 12:00:00.000', NULL, NULL, 'unlinked deposit', NULL, NULL, NULL, NULL, NULL, NULL, '2026-05-02 11:00:00.000'),
	('tx-deposit-late', 'bank-owner-a', 'synthetic-owner-a', 'deposit', 'manual', 0.20, '2026-05-02 12:00:00.000', 'expense-late', NULL, '', NULL, NULL, NULL, NULL, NULL, NULL, '2026-05-02 12:00:00.000'),
	('tx-interest', 'bank-owner-a', 'synthetic-owner-a', 'interest', 'yield', 0.01, '2026-05-03 12:00:00.000', NULL, NULL, NULL, NULL, 0.00123456, 125.5000, 10.00345678, 10.00901234, 'interest-fixture-key', '2026-05-03 12:01:02.123'),
	('tx-foreign', 'bank-foreign', 'synthetic-owner-b', 'deposit', 'manual', 88.88, '2026-05-04 12:00:00.000', 'expense-foreign', NULL, 'foreign owner', NULL, NULL, NULL, NULL, NULL, NULL, '2026-05-04 08:00:00.000'),
	('tx-archived', 'bank-owner-a-archived', 'synthetic-owner-a', 'deposit', 'manual', 1.00, '2026-05-05 12:00:00.000', NULL, NULL, 'archived bank', NULL, NULL, NULL, NULL, NULL, NULL, '2026-05-05 08:00:00.000');
