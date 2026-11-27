import assert from 'node:assert/strict';
import test from 'node:test';

import {
  createReadQualificationRoutes,
  readQualificationEnabled,
} from './go-contract-read-qualification.ts';

test('read qualification is explicit and covers the seven #163 GET routes', () => {
  assert.equal(readQualificationEnabled([]), false);
  assert.equal(readQualificationEnabled(['--qualify-reads']), true);

  const routes = createReadQualificationRoutes('parity-read-qualification-bank');
  assert.deepEqual(routes.map(({ path }) => path), [
    '/expenses',
    '/entries',
    '/cards',
    '/piggy-banks',
    '/financial-history?snapshot=2026-10-25T12%3A00%3A00.000Z&limit=2',
    '/piggy-banks/cash-movements',
    '/piggy-banks/parity-read-qualification-bank/transactions',
  ]);
  assert.deepEqual(routes.map(({ contractTime }) => contractTime ?? false), [
    false,
    false,
    true,
    true,
    false,
    false,
    false,
  ]);

  const bodies = [
    [{ id: 'parity-read-qualification-expense' }],
    [{ id: 'parity-read-qualification-entry' }],
    [{ id: 'parity-read-qualification-card' }],
    [{ id: 'parity-read-qualification-bank' }],
    {
      snapshot: '2026-10-25T12:00:00.000Z',
      expenses: [{ id: 'parity-read-qualification-expense' }],
      incomes: [{ id: 'parity-read-qualification-entry' }],
    },
    [{ type: 'deposit', amount: 3.21, date: '2026-10-20' }],
    [{ id: 'parity-read-qualification-transaction' }],
  ];

  for (const [index, route] of routes.entries()) {
    assert.doesNotThrow(() => route.assertBody(bodies[index]));
    assert.throws(() => route.assertBody(null));
  }
});
