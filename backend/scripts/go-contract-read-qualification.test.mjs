import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import {
  createReadQualificationRoutes,
  readQualificationEnabled,
} from './go-contract-read-qualification.ts';

test('read qualification is explicit and covers the seven additional #163 GET contracts', () => {
  assert.equal(readQualificationEnabled([]), false);
  assert.equal(readQualificationEnabled(['--qualify']), false);
  assert.equal(readQualificationEnabled(['--qualify-reads']), true);

  const routes = createReadQualificationRoutes('parity-read-qualification-bank');
  assert.deepEqual(routes.map(({ path }) => path), [
    '/cards',
    '/piggy-banks',
    '/financial-history?snapshot=2026-10-25T12%3A00%3A00.000Z&limit=2',
    '/piggy-banks/cash-movements',
    '/piggy-banks/parity-read-qualification-bank/transactions',
    '/assets/BTC/transactions',
    '/patrimony/settings',
  ]);
  assert.deepEqual(routes.map(({ contractTime }) => contractTime ?? false), [
    true,
    true,
    false,
    false,
    false,
    false,
    false,
  ]);

  const bodies = [
    [{ id: 'parity-read-qualification-card' }],
    [{ id: 'parity-read-qualification-bank' }],
    {
      snapshot: '2026-10-25T12:00:00.000Z',
      expenses: [{ id: 'parity-read-qualification-expense' }],
      incomes: [{ id: 'parity-read-qualification-entry' }],
    },
    [{ type: 'deposit', amount: 3.21, date: '2026-10-20' }],
    [{ id: 'parity-read-qualification-transaction' }],
    [{ id: 'parity-read-qualification-asset-btc' }],
    { baseDate: '2026-02-03', openingCashBrl: '1234.56' },
  ];

  for (const [index, route] of routes.entries()) {
    assert.doesNotThrow(() => route.assertBody(bodies[index]));
    assert.throws(() => route.assertBody(null));
  }
});

test('the default CI qualification keeps the additional read mode opt-in', () => {
  const backendPackage = JSON.parse(
    readFileSync(new URL('../package.json', import.meta.url), 'utf8'),
  );

  assert.equal(
    backendPackage.scripts['test:go-contract:qualify'],
    'tsx scripts/go-contract.ts --load --qualify',
  );
  assert.equal(
    backendPackage.scripts['test:go-contract:qualify:reads'],
    'tsx scripts/go-contract.ts --load --qualify --qualify-reads',
  );
});
