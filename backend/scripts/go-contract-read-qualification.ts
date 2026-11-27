import assert from 'node:assert/strict';

export const readQualificationSnapshot = '2026-10-25T12:00:00.000Z';

export type ReadQualificationRoute = {
  path: string;
  assertBody: (body: unknown) => void;
  contractTime?: boolean;
};

function assertIDs(body: unknown, expectedIDs: string[]) {
  assert.ok(Array.isArray(body));
  assert.deepEqual(
    body.map((row: { id: string }) => row.id),
    expectedIDs,
  );
}

export function readQualificationEnabled(args: string[] = process.argv) {
  return args.includes('--qualify-reads');
}

export function createReadQualificationRoutes(bankID: string): ReadQualificationRoute[] {
  const expenseID = 'parity-read-qualification-expense';
  const entryID = 'parity-read-qualification-entry';

  return [
    {
      path: '/expenses',
      assertBody: (body) => assertIDs(body, [expenseID]),
    },
    {
      path: '/entries',
      assertBody: (body) => assertIDs(body, [entryID]),
    },
    {
      path: '/cards',
      assertBody: (body) => assertIDs(body, ['parity-read-qualification-card']),
      contractTime: true,
    },
    {
      path: '/piggy-banks',
      assertBody: (body) => assertIDs(body, [bankID]),
      contractTime: true,
    },
    {
      path: `/financial-history?snapshot=${encodeURIComponent(readQualificationSnapshot)}&limit=2`,
      assertBody: (body) => {
        assert.ok(body && typeof body === 'object');
        const page = body as {
          snapshot: string;
          expenses: Array<{ id: string }>;
          incomes: Array<{ id: string }>;
        };
        assert.equal(page.snapshot, readQualificationSnapshot);
        assertIDs(page.expenses, [expenseID]);
        assertIDs(page.incomes, [entryID]);
      },
    },
    {
      path: '/piggy-banks/cash-movements',
      assertBody: (body) =>
        assert.deepEqual(body, [{ type: 'deposit', amount: 3.21, date: '2026-10-20' }]),
    },
    {
      path: `/piggy-banks/${bankID}/transactions`,
      assertBody: (body) =>
        assertIDs(body, ['parity-read-qualification-transaction']),
    },
  ];
}
