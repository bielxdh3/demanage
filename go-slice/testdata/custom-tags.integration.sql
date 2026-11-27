-- Route-only test schema derived from:
--   backend/prisma/migrations/20260804160000_custom_tags_and_weekly/migration.sql
--   backend/prisma/migrations/20260811155000_add_offline_recovery_code/migration.sql
-- Only columns used by the Go session and custom-tag reads are included.
CREATE TYPE "CustomTagScope" AS ENUM ('expense', 'income');

CREATE TABLE "User" (
    "id" TEXT NOT NULL,
    "sessionVersion" INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT "User_pkey" PRIMARY KEY ("id")
);

CREATE TABLE "CustomTag" (
    "id" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "scope" "CustomTagScope" NOT NULL,
    "name" TEXT NOT NULL,
    "color" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,
    CONSTRAINT "CustomTag_pkey" PRIMARY KEY ("id"),
    CONSTRAINT "CustomTag_userId_fkey" FOREIGN KEY ("userId")
        REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT "CustomTag_userId_scope_name_key" UNIQUE ("userId", "scope", "name")
);

CREATE INDEX "CustomTag_userId_scope_idx" ON "CustomTag"("userId", "scope");
