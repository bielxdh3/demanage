import assert from 'node:assert/strict';
import test from 'node:test';

import {
  incomeContributionThisMonth,
  isIncomeAutoReceivedThisMonth,
  isIncomeReceivedThisMonth,
} from '@/lib/income-schedule';
import type { Income } from '@/types/finance';

const now = new Date('2026-10-14T15:00:00.000Z');

function income(overrides: Partial<Income> = {}): Income {
  return {
    id: 'income-1',
    name: 'Presente',
    type: 'outro',
    frequency: 'unica',
    amount: 700,
    date: '2026-10-09',
    ...overrides,
  };
}

test('one-off incomes waiting for confirmation are not credited by their scheduled date', () => {
  const pending = income();
  assert.equal(isIncomeAutoReceivedThisMonth(pending, now), false);
  assert.equal(isIncomeReceivedThisMonth(pending, now), false);
  assert.equal(incomeContributionThisMonth(pending, now), 0);
});

test('confirming a one-off income credits the receipt snapshot, not the edited template amount', () => {
  const received = income({
    amount: 1200,
    receipts: [
      {
        month: '2026-10',
        amount: 700,
        receivedAt: '2026-10-12T15:00:00.000Z',
      },
    ],
  });
  assert.equal(isIncomeReceivedThisMonth(received, now), true);
  assert.equal(incomeContributionThisMonth(received, now), 700);
});

test('one-off receipt counts in the month it was actually received, not its scheduled month', () => {
  const received = income({
    date: '2026-09-09',
    receipts: [
      {
        month: '2026-09',
        amount: 234.87,
        receivedAt: '2026-10-10T15:00:00.000Z',
      },
    ],
  });
  assert.equal(incomeContributionThisMonth(received, now), 234.87);
  assert.equal(
    incomeContributionThisMonth(received, new Date('2026-09-30T15:00:00Z')),
    0,
  );
});

test('previous-month and future receipt timestamps do not inflate the current balance', () => {
  const received = income({
    receipts: [
      {
        month: '2026-09',
        amount: 50,
        receivedAt: '2026-09-28T15:00:00.000Z',
      },
      {
        month: '2026-10',
        amount: 90,
        receivedAt: '2026-10-20T15:00:00.000Z',
      },
    ],
  });
  assert.equal(isIncomeReceivedThisMonth(received, now), false);
  assert.equal(incomeContributionThisMonth(received, now), 0);
});

test('one-off receipt uses the São Paulo calendar across midnight UTC', () => {
  const endOfOctober = new Date('2026-11-01T00:30:00.000Z');
  const received = income({
    receipts: [
      {
        month: '2026-10',
        amount: 42,
        receivedAt: '2026-11-01T00:15:00.000Z',
      },
    ],
  });
  assert.equal(incomeContributionThisMonth(received, endOfOctober), 42);
  assert.equal(
    incomeContributionThisMonth(received, new Date('2026-11-01T15:00:00Z')),
    0,
  );
});

test('monthly salary still credits automatically on schedule and respects a manual hold', () => {
  const salary = income({
    type: 'salario',
    frequency: 'mensal',
    amount: 1680,
    receiveDay: 5,
    startsAt: '2026-01-05',
  });
  assert.equal(incomeContributionThisMonth(salary, now), 1680);
  assert.equal(
    incomeContributionThisMonth(
      { ...salary, receiptHoldForMonth: '2026-10' },
      now,
    ),
    0,
  );
});
