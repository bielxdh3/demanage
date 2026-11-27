CREATE TEMP TABLE "__asset_transaction_projection_anchor" ("id" INTEGER);

CREATE TYPE "Asset" AS ENUM ('BTC', 'USD');
CREATE TYPE "AssetTransactionType" AS ENUM ('BUY', 'SELL', 'MANUAL_ADJUSTMENT');

CREATE TEMP TABLE "AssetTransaction" (
	"id" TEXT NOT NULL PRIMARY KEY,
	"userId" TEXT NOT NULL,
	"asset" "Asset" NOT NULL,
	"type" "AssetTransactionType" NOT NULL,
	"quantity" DECIMAL(30,12) NOT NULL,
	"cashAmountBrl" DECIMAL(18,8) NOT NULL,
	"feeAmountBrl" DECIMAL(18,8) NOT NULL,
	"feePercent" DECIMAL(12,8),
	"costBasisKnown" BOOLEAN NOT NULL,
	"date" TIMESTAMP(3) NOT NULL,
	"note" TEXT,
	"expenseId" TEXT,
	"entryId" TEXT,
	"createdAt" TIMESTAMP(3) NOT NULL
);

INSERT INTO "AssetTransaction" (
	"id", "userId", "asset", "type", "quantity", "cashAmountBrl", "feeAmountBrl",
	"feePercent", "costBasisKnown", "date", "note", "expenseId", "entryId", "createdAt"
) VALUES
	('tx-btc-same-date-old-created', 'synthetic-owner-a', 'BTC', 'SELL', 0.000000000001, 0.10000000, 0.00000001, NULL, FALSE, '2026-11-20 09:30:00.123', NULL, NULL, 'entry-a', '2026-11-20 09:59:59.999'),
	('tx-usd-owner-a', 'synthetic-owner-a', 'USD', 'BUY', 1.000000000000, 10.00000000, 0.00000000, NULL, TRUE, '2026-11-20 09:30:00.123', NULL, NULL, NULL, '2026-11-20 10:00:00.000'),
	('tx-btc-owner-b', 'synthetic-owner-b', 'BTC', 'BUY', 5.000000000000, 500.00000000, 0.00000000, NULL, TRUE, '2026-11-20 09:30:00.123', NULL, NULL, NULL, '2026-11-20 10:00:00.000'),
	('tx-btc-date-new', 'synthetic-owner-a', 'BTC', 'BUY', 1.234567890123, 9999999999.99999999, 0.00000000, NULL, TRUE, '2026-11-21 08:00:00.789', NULL, NULL, NULL, '2026-11-20 08:00:00.001'),
	('tx-btc-same-date-new-created', 'synthetic-owner-a', 'BTC', 'BUY', 0.000000000001, 123.45678901, 0.00000001, 0.00000001, TRUE, '2026-11-20 09:30:00.123', 'precision', 'expense-a', NULL, '2026-11-20 10:00:00.456');
