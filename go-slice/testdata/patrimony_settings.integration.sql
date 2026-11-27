CREATE TEMP TABLE "PatrimonySettings" (
	"id" TEXT NOT NULL PRIMARY KEY,
	"userId" TEXT NOT NULL UNIQUE,
	"baseDate" TIMESTAMP(3) NOT NULL,
	"openingCashBrl" DECIMAL(18,2) NOT NULL,
	"createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
	"updatedAt" TIMESTAMP(3) NOT NULL
);

INSERT INTO "PatrimonySettings" ("id", "userId", "baseDate", "openingCashBrl", "updatedAt") VALUES
	('settings-owner-a', 'synthetic-owner-a', '2026-02-03 00:00:00.000', 1234567890123456.78, '2026-02-04 05:06:07.089'),
	('settings-owner-b', 'synthetic-owner-b', '2026-12-31 00:00:00.000', 0.00, '2026-12-31 23:59:59.999');
