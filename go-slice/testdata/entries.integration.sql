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

INSERT INTO "CustomTag" ("id", "userId", "scope", "name", "color") VALUES
	('tag-foreign', 'synthetic-owner-b', 'income', 'Foreign income', '#FF0000'),
	('tag-expense', 'synthetic-owner-a', 'expense', 'Expense scope', '#00FF00'),
	('tag-income', 'synthetic-owner-a', 'income', 'Extra', '#AABBCC');

INSERT INTO "Entry" (
	"id", "userId", "customTagId", "name", "notes", "amount", "type", "frequency",
	"receiveDay", "startsAt", "endsAt", "date", "receiptHoldForMonth", "receivedForMonth",
	"receivedAt", "archivedAt", "systemOrigin", "createdAt", "updatedAt"
) VALUES
	('entry-z-salary', 'synthetic-owner-a', 'tag-foreign', 'Salário', NULL, 9999999999.99, 'salario', 'mensal', 5, '2026-01-05 00:00:00.000', NULL, NULL, '2026-12', NULL, NULL, NULL, 'manual', '2026-03-01 10:00:00.007', '2026-03-02 00:00:00.000'),
	('entry-m-wrong-scope', 'synthetic-owner-a', 'tag-expense', 'Tag inválida', 'legacy', 50.25, 'outro', 'mensal', 15, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 'manual', '2026-03-01 10:00:00.007', '2026-03-02 00:00:00.000'),
	('entry-a-tagged', 'synthetic-owner-a', 'tag-income', 'Extra', '', 0.10, 'outro', 'unica', NULL, NULL, NULL, '2026-11-20 12:00:00.000', NULL, NULL, NULL, NULL, 'manual', '2026-03-01 10:00:00.007', '2026-03-02 00:00:00.000'),
	('entry-empty', 'synthetic-owner-a', NULL, 'Outros', NULL, 0.10, 'freelance', 'mensal', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 'manual', '2026-02-28 00:00:00.000', '2026-02-28 00:00:00.000'),
	('entry-foreign-owner', 'synthetic-owner-b', NULL, 'Foreign entry', NULL, 1.00, 'outro', 'unica', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 'manual', '2026-04-01 00:00:00.000', '2026-04-01 00:00:00.000'),
	('entry-archived', 'synthetic-owner-a', NULL, 'Archived entry', NULL, 2.00, 'outro', 'unica', NULL, NULL, NULL, NULL, NULL, NULL, NULL, '2026-04-01 00:00:00.000', 'manual', '2026-04-02 00:00:00.000', '2026-04-02 00:00:00.000'),
	('entry-system', 'synthetic-owner-a', NULL, 'System entry', NULL, 3.00, 'outro', 'unica', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 'piggy', '2026-04-03 00:00:00.000', '2026-04-03 00:00:00.000');

INSERT INTO "SalaryHold" ("id", "entryId", "month", "createdAt") VALUES
	('hold-dec', 'entry-z-salary', '2026-12', '2026-12-01 00:00:00.000'),
	('hold-nov', 'entry-z-salary', '2026-11', '2026-11-01 00:00:00.000');

INSERT INTO "SalaryVersion" ("id", "entryId", "month", "amount", "receiveDay", "createdAt") VALUES
	('version-dec', 'entry-z-salary', '2026-12', 9999999999.99, 5, '2026-12-01 00:00:00.000'),
	('version-nov', 'entry-z-salary', '2026-11', 1234567890.12, 4, '2026-11-01 00:00:00.000');

INSERT INTO "EntryReceipt" ("id", "entryId", "month", "amount", "receivedAt", "createdAt") VALUES
	('receipt-c', 'entry-z-salary', '2026-12', 0.30, '2026-11-05 12:00:00.000', '2026-11-05 12:00:00.000'),
	('receipt-b', 'entry-z-salary', '2026-11', 0.20, '2026-11-05 12:00:00.000', '2026-11-05 12:00:00.000'),
	('receipt-a', 'entry-z-salary', '2026-10', 0.10, '2026-11-05 12:00:00.000', '2026-11-04 12:00:00.000');
