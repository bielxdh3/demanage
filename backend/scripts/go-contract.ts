import assert from 'node:assert/strict';
import { type ChildProcess, spawn } from 'node:child_process';
import { createServer, type Server } from 'node:http';
import { setTimeout as delay } from 'node:timers/promises';

const databaseUrl = process.env.DATABASE_URL;
const goBinary = process.env.GO_CONTRACT_BINARY;
const ownerA = 'parity-owner-a';
const ownerB = 'parity-owner-b';
const timestamp = new Date('2026-01-02T03:04:05.000Z');
const contractNow = '2026-11-01T02:00:00.000Z';
let prisma: (typeof import('../src/lib/prisma'))['prisma'] | undefined;
let signAuthToken: typeof import('../src/lib/auth').signAuthToken | undefined;
let cookieName: typeof import('../src/lib/auth').AUTH_COOKIE_NAME | undefined;

function requireHostedTestInputs() {
  assert.equal(process.env.GITHUB_ACTIONS, 'true', 'contract runs in hosted CI only');
  assert.ok(databaseUrl, 'DATABASE_URL is required');
  const parsed = new URL(databaseUrl);
  assert.ok(['localhost', '127.0.0.1', '::1'].includes(parsed.hostname));
  assert.equal(parsed.pathname, '/demanage_parity_test');
  assert.equal(parsed.searchParams.has('schema'), false);
  assert.ok(goBinary, 'GO_CONTRACT_BINARY is required');
}

async function seedFixtures() {
  const client = prisma;
  assert.ok(client);
  await client.user.createMany({
    data: [
      {
        id: ownerA,
        name: 'Parity owner A',
        email: 'parity-a@example.invalid',
        passwordHash: 'synthetic-only',
      },
      {
        id: ownerB,
        name: 'Parity owner B',
        email: 'parity-b@example.invalid',
        passwordHash: 'synthetic-only',
      },
    ],
  });
  await client.customTag.createMany({
    data: [
      {
        id: 'parity-a-expense-z',
        userId: ownerA,
        scope: 'expense',
        name: 'Zebra',
        color: '#112233',
        createdAt: timestamp,
        updatedAt: timestamp,
      },
      {
        id: 'parity-a-expense-a',
        userId: ownerA,
        scope: 'expense',
        name: 'Alpha',
        color: '#223344',
        createdAt: timestamp,
        updatedAt: timestamp,
      },
      {
        id: 'parity-a-income',
        userId: ownerA,
        scope: 'income',
        name: 'Salary extra',
        color: '#334455',
        createdAt: timestamp,
        updatedAt: timestamp,
      },
      {
        id: 'parity-b-expense',
        userId: ownerB,
        scope: 'expense',
        name: 'Private',
        color: '#445566',
        createdAt: timestamp,
        updatedAt: timestamp,
      },
    ],
  });
}

async function seedFinancialFixtures() {
  const client = prisma;
  assert.ok(client);

  await client.card.createMany({
    data: [
      {
        id: 'parity-a-card-blue',
        userId: ownerA,
        name: 'Azul',
        limit: 2000,
        closingDay: 10,
        lastBillingProcessedAt: null,
        minimumNextClosingOn: null,
        lastInvoicedOn: new Date('2026-10-10T12:00:00.000Z'),
        expiresAt: new Date('2026-11-01T01:59:00.000Z'),
        createdAt: new Date('2026-02-01T00:00:00.000Z'),
        updatedAt: new Date('2026-02-01T00:00:00.000Z'),
      },
      {
        id: 'parity-a-card-green',
        userId: ownerA,
        name: 'Verde',
        limit: null,
        closingDay: 12,
        lastBillingProcessedAt: null,
        minimumNextClosingOn: null,
        lastInvoicedOn: new Date('2026-10-12T12:00:00.000Z'),
        expiresAt: new Date('2026-11-01T03:00:00.000Z'),
        createdAt: new Date('2026-02-02T00:00:00.000Z'),
        updatedAt: new Date('2026-02-02T00:00:00.000Z'),
      },
      {
        id: 'parity-a-card-archived',
        userId: ownerA,
        name: 'Arquivado',
        limit: 500,
        closingDay: 15,
        lastBillingProcessedAt: null,
        minimumNextClosingOn: null,
        archivedAt: new Date('2026-10-20T12:00:00.000Z'),
        createdAt: new Date('2026-03-01T00:00:00.000Z'),
        updatedAt: new Date('2026-10-20T12:00:00.000Z'),
      },
      {
        id: 'parity-b-card-private',
        userId: ownerB,
        name: 'Privado',
        limit: 100,
        closingDay: 31,
        lastBillingProcessedAt: null,
        minimumNextClosingOn: null,
        lastInvoicedOn: new Date('2026-10-30T12:00:00.000Z'),
        createdAt: new Date('2026-01-03T00:00:00.000Z'),
        updatedAt: new Date('2026-10-30T12:00:00.000Z'),
      },
    ],
  });
  await client.expense.createMany({
    data: [
      {
        id: 'parity-a-expense-full',
        userId: ownerA,
        customTagId: 'parity-a-expense-a',
        name: 'Moradia',
        amount: 1234.56,
        category: 'outro',
        frequency: 'mensal',
        dueDay: 5,
        startsAt: new Date('2026-02-01T00:00:00.000Z'),
        endsAt: new Date('2026-12-31T23:59:59.000Z'),
        paidForMonth: '2026-02',
        paidAt: new Date('2026-02-02T10:00:00.000Z'),
        notes: 'fixture with exact splits and payments',
        createdAt: new Date('2026-02-10T12:00:00.000Z'),
        updatedAt: new Date('2026-02-10T12:00:00.000Z'),
      },
      {
        id: 'parity-a-expense-empty-relations',
        userId: ownerA,
        name: 'Avulsa',
        amount: 0,
        category: 'outro',
        frequency: 'unica',
        occurredAt: new Date('2026-01-15T12:00:00.000Z'),
        createdAt: new Date('2026-02-09T12:00:00.000Z'),
        updatedAt: new Date('2026-02-09T12:00:00.000Z'),
      },
      {
        id: 'parity-a-expense-archived-card-charge',
        userId: ownerA,
        cardId: 'parity-a-card-blue',
        name: 'Compromisso arquivado',
        amount: 12.34,
        category: 'outro',
        frequency: 'mensal',
        dueDay: 20,
        startsAt: new Date('2026-09-20T12:00:00.000Z'),
        archivedAt: new Date('2026-10-25T12:00:00.000Z'),
        createdAt: new Date('2026-09-20T12:00:00.000Z'),
        updatedAt: new Date('2026-10-25T12:00:00.000Z'),
      },
      {
        id: 'parity-b-expense-private',
        userId: ownerB,
        customTagId: 'parity-b-expense',
        name: 'Privada',
        amount: 9876.54,
        category: 'outro',
        frequency: 'mensal',
        createdAt: new Date('2026-02-11T12:00:00.000Z'),
        updatedAt: new Date('2026-02-11T12:00:00.000Z'),
      },
      {
        id: 'parity-b-expense-card-one-off',
        userId: ownerB,
        cardId: 'parity-b-card-private',
        name: 'Compra no fechamento SP',
        amount: 50.23,
        category: 'outro',
        frequency: 'unica',
        occurredAt: new Date('2026-10-31T12:00:00.000Z'),
        createdAt: new Date('2026-10-31T13:00:00.000Z'),
        updatedAt: new Date('2026-10-31T13:00:00.000Z'),
      },
      {
        id: 'parity-b-expense-after-sp-close',
        userId: ownerB,
        cardId: 'parity-b-card-private',
        name: 'Parcela após o fechamento SP',
        amount: 7.77,
        category: 'outro',
        frequency: 'mensal',
        dueDay: 1,
        startsAt: new Date('2026-11-01T12:00:00.000Z'),
        createdAt: new Date('2026-10-31T12:00:00.000Z'),
        updatedAt: new Date('2026-10-31T12:00:00.000Z'),
      },
    ],
  });
  await client.expenseSplit.createMany({
    data: [
      {
        id: 'parity-a-split-blue',
        expenseId: 'parity-a-expense-full',
        kind: 'card',
        cardId: 'parity-a-card-blue',
        percent: 60,
        amount: 740.74,
        createdAt: new Date('2026-02-10T12:00:00.000Z'),
        updatedAt: new Date('2026-02-10T12:00:00.000Z'),
      },
      {
        id: 'parity-a-split-green',
        expenseId: 'parity-a-expense-full',
        kind: 'card',
        cardId: 'parity-a-card-green',
        percent: 20,
        amount: 246.91,
        createdAt: new Date('2026-02-10T12:00:00.000Z'),
        updatedAt: new Date('2026-02-10T12:00:00.000Z'),
      },
      {
        id: 'parity-a-split-pix',
        expenseId: 'parity-a-expense-full',
        kind: 'pix',
        percent: 20,
        amount: 246.91,
        createdAt: new Date('2026-02-10T12:00:00.000Z'),
        updatedAt: new Date('2026-02-10T12:00:00.000Z'),
      },
    ],
  });
  await client.expensePayment.createMany({
    data: [
      {
        id: 'parity-a-payment-1',
        expenseId: 'parity-a-expense-full',
        month: '2026-02',
        amount: 5,
        paidAt: new Date('2026-02-02T10:00:00.000Z'),
        createdAt: new Date('2026-02-02T10:00:00.000Z'),
      },
      {
        id: 'parity-a-payment-2',
        expenseId: 'parity-a-expense-full',
        month: '2026-03',
        amount: 123.45,
        paidAt: new Date('2026-03-02T10:00:00.000Z'),
        createdAt: new Date('2026-03-02T10:01:00.000Z'),
      },
      {
        id: 'parity-a-payment-3',
        expenseId: 'parity-a-expense-full',
        month: '2026-04',
        amount: 0.1,
        paidAt: new Date('2026-04-02T10:00:00.000Z'),
        createdAt: new Date('2026-04-02T10:01:00.000Z'),
      },
    ],
  });

  await client.entry.createMany({
    data: [
      {
        id: 'parity-a-entry-salary',
        userId: ownerA,
        name: 'Salário',
        amount: 9999999999.99,
        type: 'salario',
        frequency: 'mensal',
        receiveDay: 5,
        startsAt: new Date('2026-01-05T00:00:00.000Z'),
        receiptHoldForMonth: '2026-12',
        createdAt: new Date('2026-03-01T10:00:00.007Z'),
        updatedAt: new Date('2026-03-01T10:00:00.007Z'),
      },
      {
        id: 'parity-a-entry-tagged',
        userId: ownerA,
        customTagId: 'parity-a-income',
        name: 'Extra',
        notes: '',
        amount: 0.1,
        type: 'outro',
        frequency: 'unica',
        date: new Date('2026-02-20T12:00:00.000Z'),
        createdAt: new Date('2026-02-20T12:00:00.000Z'),
        updatedAt: new Date('2026-02-20T12:00:00.000Z'),
      },
      {
        id: 'parity-b-entry-private',
        userId: ownerB,
        name: 'Privada',
        amount: 3.33,
        type: 'freelance',
        frequency: 'unica',
        date: new Date('2026-02-21T12:00:00.000Z'),
        createdAt: new Date('2026-02-21T12:00:00.000Z'),
        updatedAt: new Date('2026-02-21T12:00:00.000Z'),
      },
    ],
  });
  await client.salaryHold.createMany({
    data: [
      {
        id: 'parity-a-hold-nov',
        entryId: 'parity-a-entry-salary',
        month: '2026-11',
        createdAt: new Date('2026-11-01T00:00:00.000Z'),
      },
      {
        id: 'parity-a-hold-dec',
        entryId: 'parity-a-entry-salary',
        month: '2026-12',
        createdAt: new Date('2026-12-01T00:00:00.000Z'),
      },
    ],
  });
  await client.salaryVersion.createMany({
    data: [
      {
        id: 'parity-a-version-nov',
        entryId: 'parity-a-entry-salary',
        month: '2026-11',
        amount: 1234567890.12,
        receiveDay: 4,
        createdAt: new Date('2026-11-01T00:00:00.000Z'),
      },
      {
        id: 'parity-a-version-dec',
        entryId: 'parity-a-entry-salary',
        month: '2026-12',
        amount: 9999999999.99,
        receiveDay: 5,
        createdAt: new Date('2026-12-01T00:00:00.000Z'),
      },
    ],
  });
  await client.entryReceipt.createMany({
    data: [
      {
        id: 'parity-a-receipt-oct',
        entryId: 'parity-a-entry-salary',
        month: '2026-10',
        amount: 0.1,
        receivedAt: new Date('2026-11-05T12:00:00.000Z'),
        createdAt: new Date('2026-11-04T12:00:00.000Z'),
      },
      {
        id: 'parity-a-receipt-nov',
        entryId: 'parity-a-entry-salary',
        month: '2026-11',
        amount: 0.2,
        receivedAt: new Date('2026-11-06T12:00:00.000Z'),
        createdAt: new Date('2026-11-05T12:00:00.000Z'),
      },
      {
        id: 'parity-a-receipt-dec',
        entryId: 'parity-a-entry-salary',
        month: '2026-12',
        amount: 0.3,
        receivedAt: new Date('2026-12-05T12:00:00.000Z'),
        createdAt: new Date('2026-12-04T12:00:00.000Z'),
      },
    ],
  });

  await client.piggyBank.createMany({
    data: [
      {
        id: 'parity-a-piggy-emergency',
        userId: ownerA,
        name: 'Reserva de emergência',
        goalAmount: 100,
        targetDate: new Date('2027-02-03T12:00:00.000Z'),
        monthlyGoal: 0.1,
        autoDebit: true,
        autoDebitDay: 31,
        isEmergency: true,
        yieldEnabled: true,
        cdiPercent: 100.25,
        interestAccruedThrough: new Date('2026-11-04T12:00:00.000Z'),
        createdAt: new Date('2026-02-03T04:05:06.007Z'),
        updatedAt: new Date('2026-11-04T05:06:07.089Z'),
      },
      {
        id: 'parity-a-piggy-empty',
        userId: ownerA,
        name: 'Sem meta',
        monthlyGoal: 0,
        autoDebitDay: 1,
        createdAt: new Date('2026-03-01T04:05:06.007Z'),
        updatedAt: new Date('2026-03-01T04:05:06.007Z'),
      },
      {
        id: 'parity-a-piggy-archived',
        userId: ownerA,
        name: 'Cofre arquivado',
        goalAmount: null,
        monthlyGoal: 0,
        autoDebit: false,
        autoDebitDay: 1,
        isEmergency: true,
        archivedAt: new Date('2026-10-20T09:08:07.006Z'),
        completedAt: new Date('2026-08-01T00:00:00.000Z'),
        createdAt: new Date('2026-01-02T04:05:06.007Z'),
        updatedAt: new Date('2026-10-20T09:08:07.006Z'),
      },
      {
        id: 'parity-b-piggy-private',
        userId: ownerB,
        name: 'Privado',
        goalAmount: 200,
        monthlyGoal: 5,
        autoDebit: false,
        autoDebitDay: 5,
        createdAt: new Date('2026-04-01T00:00:00.000Z'),
        updatedAt: new Date('2026-04-01T00:00:00.000Z'),
      },
    ],
  });
  await client.piggyTransaction.createMany({
    data: [
      {
        id: 'parity-a-piggy-deposit',
        userId: ownerA,
        piggyBankId: 'parity-a-piggy-emergency',
        type: 'deposit',
        source: 'auto_debit',
        amount: 100.01,
        date: new Date('2026-10-31T12:00:00.000Z'),
        createdAt: new Date('2026-10-31T12:00:00.000Z'),
      },
      {
        id: 'parity-a-piggy-withdraw',
        userId: ownerA,
        piggyBankId: 'parity-a-piggy-emergency',
        type: 'withdraw',
        source: 'manual',
        amount: 0.02,
        date: new Date('2026-10-31T12:00:00.000Z'),
        createdAt: new Date('2026-10-31T12:01:00.000Z'),
      },
      {
        id: 'parity-a-piggy-interest',
        userId: ownerA,
        piggyBankId: 'parity-a-piggy-emergency',
        type: 'interest',
        source: 'yield',
        amount: 0.01,
        date: new Date('2026-10-31T12:00:00.000Z'),
        createdAt: new Date('2026-10-31T12:02:00.000Z'),
      },
      {
        id: 'parity-a-piggy-archived-deposit',
        userId: ownerA,
        piggyBankId: 'parity-a-piggy-archived',
        type: 'deposit',
        amount: 5,
        date: new Date('2026-09-01T12:00:00.000Z'),
        createdAt: new Date('2026-09-01T12:00:00.000Z'),
      },
      {
        id: 'parity-b-piggy-deposit',
        userId: ownerB,
        piggyBankId: 'parity-b-piggy-private',
        type: 'deposit',
        amount: 50,
        date: new Date('2026-10-31T12:00:00.000Z'),
        createdAt: new Date('2026-10-31T12:00:00.000Z'),
      },
    ],
  });
}

async function seedPortfolioFixtures() {
  const client = prisma;
  assert.ok(client);

  await client.patrimonySettings.createMany({
    data: [
      {
        id: 'parity-a-patrimony-settings',
        userId: ownerA,
        baseDate: new Date('2026-02-03T00:00:00.000Z'),
        openingCashBrl: '1234567890123456.78',
        createdAt: timestamp,
        updatedAt: timestamp,
      },
      {
        id: 'parity-b-patrimony-settings',
        userId: ownerB,
        baseDate: new Date('2025-12-31T00:00:00.000Z'),
        openingCashBrl: '0.00',
        createdAt: timestamp,
        updatedAt: timestamp,
      },
    ],
  });

  await client.assetTransaction.createMany({
    data: [
      {
        id: 'parity-a-asset-btc-same-date-old-created',
        userId: ownerA,
        asset: 'BTC',
        type: 'SELL',
        quantity: '0.000000000001',
        cashAmountBrl: '0.10000000',
        feeAmountBrl: '0.00000001',
        feePercent: null,
        costBasisKnown: false,
        date: new Date('2026-11-20T09:30:00.123Z'),
        note: null,
        createdAt: new Date('2026-11-20T09:59:59.999Z'),
        updatedAt: timestamp,
      },
      {
        id: 'parity-a-asset-usd',
        userId: ownerA,
        asset: 'USD',
        type: 'BUY',
        quantity: '1.000000000000',
        cashAmountBrl: '10.00000000',
        feeAmountBrl: '0.00000000',
        feePercent: null,
        costBasisKnown: true,
        date: new Date('2026-11-20T09:30:00.123Z'),
        note: null,
        createdAt: new Date('2026-11-20T10:00:00.000Z'),
        updatedAt: timestamp,
      },
      {
        id: 'parity-b-asset-btc-private',
        userId: ownerB,
        asset: 'BTC',
        type: 'BUY',
        quantity: '5.000000000000',
        cashAmountBrl: '500.00000000',
        feeAmountBrl: '0.00000000',
        feePercent: null,
        costBasisKnown: true,
        date: new Date('2026-11-20T09:30:00.123Z'),
        note: 'owner B only',
        createdAt: new Date('2026-11-20T10:00:00.000Z'),
        updatedAt: timestamp,
      },
      {
        id: 'parity-a-asset-btc-date-new',
        userId: ownerA,
        asset: 'BTC',
        type: 'BUY',
        quantity: '1.234567890123',
        cashAmountBrl: '9999999999.99999999',
        feeAmountBrl: '0.00000000',
        feePercent: null,
        costBasisKnown: true,
        date: new Date('2026-11-21T08:00:00.789Z'),
        note: null,
        createdAt: new Date('2026-11-20T08:00:00.001Z'),
        updatedAt: timestamp,
      },
      {
        id: 'parity-a-asset-btc-same-date-new-created',
        userId: ownerA,
        asset: 'BTC',
        type: 'BUY',
        quantity: '0.000000000001',
        cashAmountBrl: '123.45678901',
        feeAmountBrl: '0.00000001',
        feePercent: '0.00000001',
        costBasisKnown: true,
        date: new Date('2026-11-20T09:30:00.123Z'),
        note: 'precision',
        createdAt: new Date('2026-11-20T10:00:00.456Z'),
        updatedAt: timestamp,
      },
    ],
  });
}

async function seedHistoryFixtures() {
  const client = prisma;
  assert.ok(client);
  const snapshot = new Date('2026-11-25T12:00:00.000Z');
  const older = new Date('2026-11-25T11:59:59.999Z');
  const afterSnapshot = new Date('2026-11-25T12:00:00.001Z');
  const linkedDate = new Date('2026-11-20T00:00:00.000Z');

  await client.expense.createMany({
    data: [
      ...[
        ['parity-history-expense-a', 9999999999.99, snapshot],
        ['parity-history-expense-z', 0.1, snapshot],
        ['parity-history-expense-m', 1.23, snapshot],
        ['parity-history-expense-older', 0.3, older],
        ['parity-history-expense-after', 7.89, afterSnapshot],
      ].map(([id, amount, createdAt]) => ({
        id: id as string,
        userId: ownerA,
        name: id as string,
        amount: amount as number,
        category: 'outro' as const,
        frequency: 'unica' as const,
        occurredAt: createdAt as Date,
        createdAt: createdAt as Date,
        updatedAt: createdAt as Date,
      })),
      {
        id: 'zz-parity-history-expense-generated',
        userId: ownerA,
        name: 'Generated expense excluded from manual history',
        amount: 88.88,
        category: 'outro',
        frequency: 'unica',
        occurredAt: snapshot,
        systemOrigin: 'piggy',
        createdAt: snapshot,
        updatedAt: snapshot,
      },
    ],
  });
  await client.entry.createMany({
    data: [
      ...[
        ['parity-history-entry-a', 9999999999.99, snapshot],
        ['parity-history-entry-z', 0.1, snapshot],
        ['parity-history-entry-m', 2.34, snapshot],
        ['parity-history-entry-older', 0.4, older],
        ['parity-history-entry-after', 7.89, afterSnapshot],
      ].map(([id, amount, createdAt]) => ({
        id: id as string,
        userId: ownerA,
        name: id as string,
        amount: amount as number,
        type: 'outro' as const,
        frequency: 'unica' as const,
        date: createdAt as Date,
        createdAt: createdAt as Date,
        updatedAt: createdAt as Date,
      })),
      {
        id: 'zz-parity-history-entry-generated',
        userId: ownerA,
        name: 'Generated entry excluded from manual history',
        amount: 88.88,
        type: 'outro',
        frequency: 'unica',
        date: snapshot,
        systemOrigin: 'piggy',
        createdAt: snapshot,
        updatedAt: snapshot,
      },
    ],
  });
  await client.piggyBank.createMany({
    data: [
      {
        id: 'parity-a-bank-history',
        userId: ownerA,
        name: 'Owner A history',
        createdAt: linkedDate,
        updatedAt: linkedDate,
      },
      {
        id: 'parity-a-bank-empty',
        userId: ownerA,
        name: 'Owner A empty',
        createdAt: linkedDate,
        updatedAt: linkedDate,
      },
      {
        id: 'parity-b-bank-private',
        userId: ownerB,
        name: 'Owner B private',
        createdAt: linkedDate,
        updatedAt: linkedDate,
      },
    ],
  });
  await client.piggyTransaction.createMany({
    data: [
      {
        id: 'parity-a-piggy-linked-deposit',
        piggyBankId: 'parity-a-bank-history',
        userId: ownerA,
        type: 'deposit',
        source: 'manual',
        amount: 0.1,
        date: linkedDate,
        expenseId: 'parity-a-expense-full',
        createdAt: linkedDate,
      },
      {
        id: 'parity-a-piggy-linked-withdraw',
        piggyBankId: 'parity-a-bank-history',
        userId: ownerA,
        type: 'withdraw',
        source: 'manual',
        amount: 0.2,
        date: new Date('2026-11-21T00:00:00.000Z'),
        entryId: 'parity-a-entry-tagged',
        createdAt: new Date('2026-11-21T00:00:00.000Z'),
      },
      {
        id: 'parity-a-piggy-manual-unlinked',
        piggyBankId: 'parity-a-bank-history',
        userId: ownerA,
        type: 'deposit',
        source: 'manual',
        amount: 0.3,
        date: new Date('2026-11-22T00:00:00.000Z'),
        createdAt: new Date('2026-11-22T00:00:00.000Z'),
      },
      {
        id: 'parity-b-piggy-linked-deposit',
        piggyBankId: 'parity-b-bank-private',
        userId: ownerB,
        type: 'deposit',
        source: 'manual',
        amount: 0.4,
        date: linkedDate,
        expenseId: 'parity-b-expense-private',
        createdAt: linkedDate,
      },
    ],
  });
}

function closeServer(server: Server) {
  return new Promise<void>((resolve, reject) => {
    server.close((error) => error ? reject(error) : resolve());
  });
}

async function waitFor(url: string, process: ChildProcess) {
  const deadline = Date.now() + 20_000;
  while (Date.now() < deadline) {
    if (process.exitCode !== null) {
      throw new Error(`Go server exited with ${process.exitCode}`);
    }
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(1_000) });
      if (response.ok) return;
    } catch {
      // The local server may still be starting.
    }
    await delay(250);
  }
  throw new Error(`Timed out waiting for ${url}`);
}

type ResponseValue = {
  status: number;
  body: unknown;
  headers: { cacheControl: string | null; permissionsPolicy: string | null };
};

async function get(url: string, token?: string): Promise<ResponseValue> {
  assert.ok(cookieName);
  const response = await fetch(url, {
    headers: token ? { Cookie: `${cookieName}=${token}` } : {},
    signal: AbortSignal.timeout(5_000),
  });
  const body = response.status === 204 ? null : await response.json();
  return {
    status: response.status,
    body,
    headers: {
      cacheControl: response.headers.get('cache-control'),
      permissionsPolicy: response.headers.get('permissions-policy'),
    },
  };
}

function normalized(path: string, value: ResponseValue) {
  if (path !== '/health') return value;
  assert.equal(value.status, 200);
  const body = value.body as Record<string, unknown>;
  assert.equal(typeof body.timestamp, 'string');
  assert.ok(Number.isFinite(Date.parse(body.timestamp as string)));
  return { status: value.status, body: { ...body, timestamp: '<timestamp>' } };
}

async function compare(path: string, token?: string) {
  const [expressResponse, goResponse] = await Promise.all([
    get(`${expressUrl}${path}`, token),
    get(`${goUrl}${path}`, token),
  ]);
  assert.deepEqual(normalized(path, expressResponse), normalized(path, goResponse), path);
  assert.deepEqual(goResponse.headers, expressResponse.headers, `${path} headers`);
  assert.equal(expressResponse.headers.cacheControl, 'no-store', `${path} Cache-Control`);
  assert.equal(
    expressResponse.headers.permissionsPolicy,
    'camera=(), geolocation=(), microphone=()',
    `${path} Permissions-Policy`,
  );
  return expressResponse;
}

async function compareAtContractTime(path: string, token?: string) {
  const nativeDate = globalThis.Date;
  const fixedTime = nativeDate.parse(contractNow);
  globalThis.Date = new Proxy(nativeDate, {
    apply: () => new nativeDate(fixedTime).toString(),
    construct: (target, args) => Reflect.construct(target, args.length ? args : [fixedTime]),
    get: (target, property, receiver) =>
      property === 'now' ? () => fixedTime : Reflect.get(target, property, receiver),
  });
  try {
    return await compare(path, token);
  } finally {
    globalThis.Date = nativeDate;
  }
}

let server: Server | undefined;
let goProcess: ChildProcess | undefined;
let expressUrl = '';
const goUrl = 'http://127.0.0.1:8888';

async function runContract() {
  await import('../src/register-aliases');
  const [{ createApp }, auth, prismaModule] = await Promise.all([
    import('../src/app'),
    import('../src/lib/auth'),
    import('../src/lib/prisma'),
  ]);
  prisma = prismaModule.prisma;
  signAuthToken = auth.signAuthToken;
  cookieName = auth.AUTH_COOKIE_NAME;
  await seedFixtures();

  server = createServer(createApp({ logRequests: false }));
  await new Promise<void>((resolve, reject) => {
    server!.once('error', reject);
    server!.listen(0, '127.0.0.1', resolve);
  });
  const address = server.address();
  assert.ok(address && typeof address !== 'string');
  expressUrl = `http://127.0.0.1:${address.port}`;

  goProcess = spawn(goBinary!, [], {
    env: { ...process.env, GO_CONTRACT_NOW: contractNow },
    stdio: 'inherit',
  });
  await Promise.all([
    waitFor(`${expressUrl}/health`, goProcess),
    waitFor(`${goUrl}/health`, goProcess),
  ]);

  assert.ok(signAuthToken);
  const ownerAToken = signAuthToken(ownerA, 0);
  const ownerBToken = signAuthToken(ownerB, 0);

  const emptyExpenses = await compare('/expenses', ownerAToken);
  assert.deepEqual(emptyExpenses.body, []);
  const emptyEntries = await compare('/entries', ownerAToken);
  assert.deepEqual(emptyEntries.body, []);
  const emptyCards = await compareAtContractTime('/cards', ownerAToken);
  assert.deepEqual(emptyCards.body, []);
  const emptyPiggyBanks = await compareAtContractTime('/piggy-banks', ownerAToken);
  assert.deepEqual(emptyPiggyBanks.body, []);
  assert.deepEqual((await compareAtContractTime('/cards', ownerBToken)).body, []);
  assert.deepEqual((await compareAtContractTime('/piggy-banks', ownerBToken)).body, []);
  assert.deepEqual((await compare('/assets/BTC/transactions', ownerAToken)).body, []);
  assert.deepEqual((await compare('/assets/USD/transactions', ownerAToken)).body, []);
  assert.equal((await compare('/patrimony/settings', ownerAToken)).body, null);

  await seedFinancialFixtures();
  await seedPortfolioFixtures();

  const ownerACards = await compareAtContractTime('/cards', ownerAToken);
  const ownerACardRows = ownerACards.body as Array<Record<string, unknown>>;
  assert.deepEqual(ownerACardRows.map(({ id }) => id), [
    'parity-a-card-green',
    'parity-a-card-blue',
  ]);
  const blueCard = ownerACardRows[1];
  assert.equal(blueCard.committed, 753.08);
  assert.equal(blueCard.available, 1246.92);
  assert.equal(blueCard.expired, true);
  assert.equal(ownerACardRows[0].committed, 246.91);
  assert.equal(ownerACardRows[0].available, null);
  assert.deepEqual(
    (await compareAtContractTime('/cards/', ownerAToken)).body,
    ownerACards.body,
  );

  const ownerBCards = await compareAtContractTime('/cards', ownerBToken);
  const ownerBCardRows = ownerBCards.body as Array<Record<string, unknown>>;
  assert.deepEqual(ownerBCardRows.map(({ id }) => id), ['parity-b-card-private']);
  assert.equal(ownerBCardRows[0].committed, 50.23);
  assert.equal(ownerBCardRows[0].available, 49.77);

  const ownerAPiggyBanks = await compareAtContractTime('/piggy-banks', ownerAToken);
  const ownerAPiggyRows = ownerAPiggyBanks.body as Array<Record<string, unknown>>;
  assert.deepEqual(ownerAPiggyRows.map(({ id }) => id), [
    'parity-a-piggy-empty',
    'parity-a-piggy-emergency',
  ]);
  assert.equal(ownerAPiggyRows[0].goalAmount, null);
  assert.equal(ownerAPiggyRows[0].balance, 0);
  assert.equal(ownerAPiggyRows[1].balance, 100);
  assert.equal(ownerAPiggyRows[1].autoDebitPaidThisMonth, true);
  assert.deepEqual(
    (await compareAtContractTime('/piggy-banks/', ownerAToken)).body,
    ownerAPiggyBanks.body,
  );

  const archivedPiggyBanks = await compareAtContractTime(
    '/piggy-banks?includeArchived=true',
    ownerAToken,
  );
  const archivedPiggyRows = archivedPiggyBanks.body as Array<Record<string, unknown>>;
  assert.deepEqual(archivedPiggyRows.map(({ id }) => id), [
    'parity-a-piggy-empty',
    'parity-a-piggy-emergency',
    'parity-a-piggy-archived',
  ]);
  assert.equal(archivedPiggyRows[2].balance, 5);
  assert.deepEqual(
    (await compareAtContractTime('/piggy-banks?includeArchived=TRUE', ownerAToken)).body,
    ownerAPiggyBanks.body,
  );
  assert.equal(
    (await compareAtContractTime(
      '/piggy-banks?includeArchived=true&includeArchived=false',
      ownerAToken,
    )).status,
    400,
  );

  const ownerBPiggyBanks = await compareAtContractTime('/piggy-banks', ownerBToken);
  const ownerBPiggyRows = ownerBPiggyBanks.body as Array<Record<string, unknown>>;
  assert.deepEqual(ownerBPiggyRows.map(({ id }) => id), ['parity-b-piggy-private']);
  assert.equal(ownerBPiggyRows[0].balance, 50);

  const ownerAExpenses = await compare('/expenses', ownerAToken);
  const ownerAExpenseRows = ownerAExpenses.body as Array<Record<string, unknown>>;
  assert.deepEqual(ownerAExpenseRows.map(({ id }) => id), [
    'parity-a-expense-full',
    'parity-a-expense-empty-relations',
  ]);
  assert.equal(ownerAExpenseRows[0].amount, 1234.56);
  assert.deepEqual(ownerAExpenseRows[0].customTag, {
    id: 'parity-a-expense-a',
    name: 'Alpha',
    color: '#223344',
  });
  assert.deepEqual(ownerAExpenseRows[0].splits, [
    {
      id: 'parity-a-split-blue',
      kind: 'card',
      cardId: 'parity-a-card-blue',
      percent: 60,
      amount: 740.74,
      cardName: 'Azul',
    },
    {
      id: 'parity-a-split-green',
      kind: 'card',
      cardId: 'parity-a-card-green',
      percent: 20,
      amount: 246.91,
      cardName: 'Verde',
    },
    {
      id: 'parity-a-split-pix',
      kind: 'pix',
      cardId: null,
      percent: 20,
      amount: 246.91,
      cardName: null,
    },
  ]);
  assert.deepEqual(
    (ownerAExpenseRows[0].payments as Array<Record<string, unknown>>).map(
      ({ id, amount }) => ({ id, amount }),
    ),
    [
      { id: 'parity-a-payment-1', amount: 5 },
      { id: 'parity-a-payment-2', amount: 123.45 },
      { id: 'parity-a-payment-3', amount: 0.1 },
    ],
  );
  assert.deepEqual(ownerAExpenseRows[1].splits, []);
  assert.deepEqual(ownerAExpenseRows[1].payments, []);
  assert.equal(ownerAExpenseRows[1].cardId, null);
  assert.equal(ownerAExpenseRows[1].dueDay, null);

  const ownerBExpenses = await compare('/expenses', ownerBToken);
  assert.deepEqual(
    (ownerBExpenses.body as Array<{ id: string }>).map(({ id }) => id),
    [
      'parity-b-expense-card-one-off',
      'parity-b-expense-after-sp-close',
      'parity-b-expense-private',
    ],
  );

  const ownerAEntries = await compare('/entries', ownerAToken);
  const ownerAEntryRows = ownerAEntries.body as Array<Record<string, unknown>>;
  assert.deepEqual(ownerAEntryRows.map(({ id }) => id), [
    'parity-a-entry-salary',
    'parity-a-entry-tagged',
  ]);
  assert.equal(ownerAEntryRows[0].amount, 9999999999.99);
  assert.deepEqual(
    (ownerAEntryRows[0].salaryVersions as Array<Record<string, unknown>>).map(
      ({ month, amount, receiveDay }) => ({ month, amount, receiveDay }),
    ),
    [
      { month: '2026-11', amount: 1234567890.12, receiveDay: 4 },
      { month: '2026-12', amount: 9999999999.99, receiveDay: 5 },
    ],
  );
  assert.deepEqual(
    (ownerAEntryRows[0].receipts as Array<Record<string, unknown>>).map(
      ({ month, amount }) => ({ month, amount }),
    ),
    [
      { month: '2026-10', amount: 0.1 },
      { month: '2026-11', amount: 0.2 },
      { month: '2026-12', amount: 0.3 },
    ],
  );
  assert.deepEqual(ownerAEntryRows[1].customTag, {
    id: 'parity-a-income',
    name: 'Salary extra',
    color: '#334455',
  });
  assert.equal(ownerAEntryRows[1].notes, '');
  assert.deepEqual(ownerAEntryRows[1].salaryHolds, []);
  assert.deepEqual(ownerAEntryRows[1].salaryVersions, []);
  assert.deepEqual(ownerAEntryRows[1].receipts, []);

  const ownerBEntries = await compare('/entries', ownerBToken);
  assert.deepEqual(
    (ownerBEntries.body as Array<{ id: string }>).map(({ id }) => id),
    ['parity-b-entry-private'],
  );
  assert.equal((await compare('/expenses')).status, 401);
  assert.equal((await compare('/entries')).status, 401);
  assert.equal((await compareAtContractTime('/cards')).status, 401);
  assert.equal((await compareAtContractTime('/piggy-banks')).status, 401);

  const ownerABtcTransactions = await compare('/assets/btc/transactions', ownerAToken);
  assert.deepEqual(ownerABtcTransactions.body, [
    {
      id: 'parity-a-asset-btc-date-new',
      asset: 'BTC',
      type: 'BUY',
      quantity: '1.234567890123',
      cashAmountBrl: '9999999999.99999999',
      feeAmountBrl: '0',
      feePercent: null,
      costBasisKnown: true,
      date: '2026-11-21T08:00:00.789Z',
      note: null,
      expenseId: null,
      entryId: null,
      createdAt: '2026-11-20T08:00:00.001Z',
    },
    {
      id: 'parity-a-asset-btc-same-date-new-created',
      asset: 'BTC',
      type: 'BUY',
      quantity: '0.000000000001',
      cashAmountBrl: '123.45678901',
      feeAmountBrl: '0.00000001',
      feePercent: '0.00000001',
      costBasisKnown: true,
      date: '2026-11-20T09:30:00.123Z',
      note: 'precision',
      expenseId: null,
      entryId: null,
      createdAt: '2026-11-20T10:00:00.456Z',
    },
    {
      id: 'parity-a-asset-btc-same-date-old-created',
      asset: 'BTC',
      type: 'SELL',
      quantity: '0.000000000001',
      cashAmountBrl: '0.1',
      feeAmountBrl: '0.00000001',
      feePercent: null,
      costBasisKnown: false,
      date: '2026-11-20T09:30:00.123Z',
      note: null,
      expenseId: null,
      entryId: null,
      createdAt: '2026-11-20T09:59:59.999Z',
    },
  ]);

  const ownerAUsdTransactions = await compare('/assets/USD/transactions', ownerAToken);
  assert.deepEqual(ownerAUsdTransactions.body, [
    {
      id: 'parity-a-asset-usd',
      asset: 'USD',
      type: 'BUY',
      quantity: '1',
      cashAmountBrl: '10',
      feeAmountBrl: '0',
      feePercent: null,
      costBasisKnown: true,
      date: '2026-11-20T09:30:00.123Z',
      note: null,
      expenseId: null,
      entryId: null,
      createdAt: '2026-11-20T10:00:00.000Z',
    },
  ]);

  const ownerBBtcTransactions = await compare('/assets/BTC/transactions', ownerBToken);
  assert.deepEqual(ownerBBtcTransactions.body, [
    {
      id: 'parity-b-asset-btc-private',
      asset: 'BTC',
      type: 'BUY',
      quantity: '5',
      cashAmountBrl: '500',
      feeAmountBrl: '0',
      feePercent: null,
      costBasisKnown: true,
      date: '2026-11-20T09:30:00.123Z',
      note: 'owner B only',
      expenseId: null,
      entryId: null,
      createdAt: '2026-11-20T10:00:00.000Z',
    },
  ]);

  const ownerAPatrimonySettings = await compare('/patrimony/settings', ownerAToken);
  assert.deepEqual(ownerAPatrimonySettings.body, {
    baseDate: '2026-02-03',
    openingCashBrl: '1234567890123456.78',
  });
  const ownerBPatrimonySettings = await compare('/patrimony/settings', ownerBToken);
  assert.deepEqual(ownerBPatrimonySettings.body, {
    baseDate: '2025-12-31',
    openingCashBrl: '0',
  });

  const invalidAsset = await compare('/assets/ETH/transactions', ownerAToken);
  assert.equal(invalidAsset.status, 400);
  assert.deepEqual(invalidAsset.body, { error: 'Ativo inválido' });
  assert.equal((await compare('/assets/BTC/transactions')).status, 401);
  assert.equal((await compare('/patrimony/settings')).status, 401);

  const client = prisma;
  assert.ok(client);
  await client.user.update({ where: { id: ownerA }, data: { sessionVersion: 1 } });
  try {
    assert.equal((await compare('/assets/BTC/transactions', ownerAToken)).status, 401);
    assert.equal((await compare('/patrimony/settings', ownerAToken)).status, 401);
  } finally {
    await client.user.update({ where: { id: ownerA }, data: { sessionVersion: 0 } });
  }

  await compare('/health');
  const ready = await compare('/ready');
  assert.equal(ready.status, 200);
  assert.deepEqual(ready.body, {
    status: 'ok',
    service: 'demanage-backend',
    db: 'ready',
  });

  const expense = await compare('/custom-tags?scope=expense', ownerAToken);
  assert.equal(expense.status, 200);
  assert.deepEqual(
    (expense.body as Array<{ id: string }>).map(({ id }) => id),
    ['parity-a-expense-a', 'parity-a-expense-z'],
  );
  const income = await compare('/custom-tags?scope=income', ownerAToken);
  assert.equal(income.status, 200);
  assert.deepEqual((income.body as Array<{ id: string }>).map(({ id }) => id), [
    'parity-a-income',
  ]);
  const otherOwner = await compare('/custom-tags?scope=expense', ownerBToken);
  assert.equal(otherOwner.status, 200);
  assert.deepEqual(
    (otherOwner.body as Array<{ id: string }>).map(({ id }) => id),
    ['parity-b-expense'],
  );
  assert.equal(
    (otherOwner.body as Array<{ id: string }>).some(({ id }) =>
      id.startsWith('parity-a-'),
    ),
    false,
  );

  const unauthorized = await compare('/custom-tags?scope=expense');
  assert.equal(unauthorized.status, 401);
  assert.equal((await compare('/custom-tags', ownerAToken)).status, 400);
  assert.equal((await compare('/custom-tags?scope=unknown', ownerAToken)).status, 400);

  if (process.argv.includes('--load')) await compareLoad(ownerAToken);
  if (process.argv.includes('--qualify')) await compareQualification(ownerAToken);

  await seedHistoryFixtures();
  await compareHistoryEndpoints(ownerAToken, ownerBToken);

  console.log('Express and Go contract cases passed.');
}

async function compareHistoryEndpoints(ownerAToken: string, ownerBToken: string) {
  const cashMovements = await compare('/piggy-banks/cash-movements', ownerAToken);
  assert.deepEqual(cashMovements.body, [
    { type: 'deposit', amount: 0.1, date: '2026-11-20' },
    { type: 'withdraw', amount: 0.2, date: '2026-11-21' },
  ]);

  const transactions = await compare(
    '/piggy-banks/parity-a-bank-history/transactions',
    ownerAToken,
  );
  const transactionRows = transactions.body as Array<Record<string, unknown>>;
  assert.deepEqual(
    transactionRows.map(({ id, source, amount, expenseId, entryId }) => ({
      id,
      source,
      amount,
      expenseId,
      entryId,
    })),
    [
      {
        id: 'parity-a-piggy-manual-unlinked',
        source: 'manual',
        amount: 0.3,
        expenseId: null,
        entryId: null,
      },
      {
        id: 'parity-a-piggy-linked-withdraw',
        source: 'manual',
        amount: 0.2,
        expenseId: null,
        entryId: 'parity-a-entry-tagged',
      },
      {
        id: 'parity-a-piggy-linked-deposit',
        source: 'manual',
        amount: 0.1,
        expenseId: 'parity-a-expense-full',
        entryId: null,
      },
    ],
  );

  const emptyBank = await compare(
    '/piggy-banks/parity-a-bank-empty/transactions',
    ownerAToken,
  );
  assert.deepEqual(emptyBank.body, []);
  assert.equal(
    (await compare('/piggy-banks/parity-b-bank-private/transactions', ownerAToken)).status,
    404,
  );
  assert.equal(
    (await compare('/piggy-banks/parity-missing-bank/transactions', ownerAToken)).status,
    404,
  );
  const ownerBCashMovements = await compare('/piggy-banks/cash-movements', ownerBToken);
  assert.deepEqual(ownerBCashMovements.body, [
    { type: 'deposit', amount: 0.4, date: '2026-11-20' },
  ]);

  for (const path of [
    '/piggy-banks/cash-movements',
    '/piggy-banks/parity-a-bank-history/transactions',
    '/financial-history',
  ]) {
    assert.equal((await compare(path)).status, 401, `${path} requires auth`);
  }

  const snapshot = '2026-11-25T12:00:00.000Z';
  const snapshotQuery = `snapshot=${encodeURIComponent(snapshot)}&limit=2`;
  const firstPage = await compare(`/financial-history?${snapshotQuery}`, ownerAToken);
  const firstBody = firstPage.body as Record<string, unknown>;
  assert.equal(firstBody.snapshot, snapshot);
  assert.deepEqual(
    (firstBody.expenses as Array<Record<string, unknown>>).map(({ id, amount, createdAt }) => ({
      id,
      amount,
      createdAt,
    })),
    [
      { id: 'parity-history-expense-z', amount: 0.1, createdAt: snapshot },
      { id: 'parity-history-expense-m', amount: 1.23, createdAt: snapshot },
    ],
  );
  assert.deepEqual(
    (firstBody.incomes as Array<Record<string, unknown>>).map(({ id, amount, createdAt }) => ({
      id,
      amount,
      createdAt,
    })),
    [
      { id: 'parity-history-entry-z', amount: 0.1, createdAt: snapshot },
      { id: 'parity-history-entry-m', amount: 2.34, createdAt: snapshot },
    ],
  );
  assert.equal(typeof firstBody.nextExpenseCursor, 'string');
  assert.equal(typeof firstBody.nextIncomeCursor, 'string');
  const expenseCursor = firstBody.nextExpenseCursor as string;
  const incomeCursor = firstBody.nextIncomeCursor as string;

  const nextPage = await compare(
    `/financial-history?${snapshotQuery}&expenseCursor=${expenseCursor}&incomeCursor=${incomeCursor}`,
    ownerAToken,
  );
  const nextBody = nextPage.body as Record<string, unknown>;
  assert.deepEqual(
    (nextBody.expenses as Array<Record<string, unknown>>).map(({ id, amount }) => ({ id, amount })),
    [
      { id: 'parity-history-expense-a', amount: 9999999999.99 },
      { id: 'parity-history-expense-older', amount: 0.3 },
    ],
  );
  assert.deepEqual(
    (nextBody.incomes as Array<Record<string, unknown>>).map(({ id, amount }) => ({ id, amount })),
    [
      { id: 'parity-history-entry-a', amount: 9999999999.99 },
      { id: 'parity-history-entry-older', amount: 0.4 },
    ],
  );
  const returnedIDs = [
    ...(firstBody.expenses as Array<{ id: string }>).map(({ id }) => id),
    ...(firstBody.incomes as Array<{ id: string }>).map(({ id }) => id),
    ...(nextBody.expenses as Array<{ id: string }>).map(({ id }) => id),
    ...(nextBody.incomes as Array<{ id: string }>).map(({ id }) => id),
  ];
  assert.equal(returnedIDs.includes('parity-history-expense-after'), false);
  assert.equal(returnedIDs.includes('parity-history-entry-after'), false);
  assert.equal(returnedIDs.includes('zz-parity-history-expense-generated'), false);
  assert.equal(returnedIDs.includes('zz-parity-history-entry-generated'), false);

  const ended = await compare(
    `/financial-history?${snapshotQuery}&expenseCursor=end&incomeCursor=end`,
    ownerAToken,
  );
  assert.deepEqual(ended.body, {
    snapshot,
    nextExpenseCursor: null,
    nextIncomeCursor: null,
    expenses: [],
    incomes: [],
  });
}

async function compareLoad(token: string) {
  const concurrency = 10;
  const requestsPerImplementation = 100;
  const measure = async (baseUrl: string) => {
    const durations: number[] = [];
    let errors = 0;
    let next = 0;
    await Promise.all(Array.from({ length: concurrency }, async () => {
      while (next < requestsPerImplementation) {
        const index = next++;
        const started = performance.now();
        try {
          const response = await get(`${baseUrl}/custom-tags?scope=expense`, token);
          assert.equal(response.status, 200);
          assert.deepEqual(
            (response.body as Array<{ id: string }>).map(({ id }) => id),
            ['parity-a-expense-a', 'parity-a-expense-z'],
          );
          durations[index] = performance.now() - started;
        } catch {
          errors++;
        }
      }
    }));
    const sorted = durations.toSorted((a, b) => a - b);
    const percentile = (p: number) => sorted[Math.ceil(p * sorted.length) - 1] ?? 0;
    return { p50Ms: percentile(0.5), p95Ms: percentile(0.95), errors };
  };

  const [express, go] = await Promise.all([measure(expressUrl), measure(goUrl)]);
  console.log(JSON.stringify({ load: { concurrency, requestsPerImplementation, express, go } }));
  assert.equal(express.errors, 0);
  assert.equal(go.errors, 0);
}

async function compareQualification(token: string) {
  assert.equal(process.env.GITHUB_ACTIONS, 'true', 'qualification runs in hosted CI only');
  const concurrency = 10;
  const warmupRequestsPerImplementation = 100;
  const requestsPerImplementation = 10_000;
  const batches = 10;
  const requestsPerBatch = requestsPerImplementation / batches;
  const routes = [
    {
      path: '/custom-tags?scope=expense',
      expectedIDs: ['parity-a-expense-a', 'parity-a-expense-z'],
    },
    {
      path: '/expenses',
      expectedIDs: ['parity-a-expense-full', 'parity-a-expense-empty-relations'],
    },
    {
      path: '/entries',
      expectedIDs: ['parity-a-entry-salary', 'parity-a-entry-tagged'],
    },
  ];

  const measure = async (
    baseUrl: string,
    path: string,
    expectedIDs: string[],
    requestCount: number,
  ) => {
    const durations: number[] = [];
    let errors = 0;
    let next = 0;
    await Promise.all(Array.from({ length: concurrency }, async () => {
      while (next < requestCount) {
        const index = next++;
        const started = performance.now();
        try {
          const response = await get(`${baseUrl}${path}`, token);
          assert.equal(response.status, 200, `${path} status`);
          assert.deepEqual(
            (response.body as Array<{ id: string }>).map(({ id }) => id),
            expectedIDs,
            `${path} IDs`,
          );
        } catch {
          errors++;
        } finally {
          durations[index] = performance.now() - started;
        }
      }
    }));
    return { durations, errors };
  };

  const percentile = (durations: number[], p: number) => {
    const sorted = durations.toSorted((a, b) => a - b);
    return sorted[Math.ceil(p * sorted.length) - 1] ?? 0;
  };

  for (const { path, expectedIDs } of routes) {
    const expressWarmup = await measure(
      expressUrl,
      path,
      expectedIDs,
      warmupRequestsPerImplementation,
    );
    const goWarmup = await measure(
      goUrl,
      path,
      expectedIDs,
      warmupRequestsPerImplementation,
    );
    const expressDurations: number[] = [];
    const goDurations: number[] = [];
    let expressErrors = expressWarmup.errors;
    let goErrors = goWarmup.errors;

    for (let batch = 0; batch < batches; batch++) {
      const first = batch % 2 === 0 ? 'express' : 'go';
      const firstResult = await measure(
        first === 'express' ? expressUrl : goUrl,
        path,
        expectedIDs,
        requestsPerBatch,
      );
      const secondResult = await measure(
        first === 'express' ? goUrl : expressUrl,
        path,
        expectedIDs,
        requestsPerBatch,
      );
      if (first === 'express') {
        expressDurations.push(...firstResult.durations);
        goDurations.push(...secondResult.durations);
        expressErrors += firstResult.errors;
        goErrors += secondResult.errors;
      } else {
        goDurations.push(...firstResult.durations);
        expressDurations.push(...secondResult.durations);
        goErrors += firstResult.errors;
        expressErrors += secondResult.errors;
      }
    }

    const result = {
      qualification: {
        route: path,
        runner: {
          os: process.env.RUNNER_OS ?? 'unknown',
          arch: process.env.RUNNER_ARCH ?? 'unknown',
        },
        concurrency,
        warmupRequestsPerImplementation,
        requestsPerImplementation,
        requestsPerBatch,
        batches,
        order: 'alternating express/go then go/express; batches sequential',
        limit: 'Go p95 <= 1.10 * Express p95; zero errors for both',
        express: {
          p50Ms: percentile(expressDurations, 0.5),
          p95Ms: percentile(expressDurations, 0.95),
          warmupSamples: expressWarmup.durations.length,
          measuredSamples: expressDurations.length,
          errors: expressErrors,
        },
        go: {
          p50Ms: percentile(goDurations, 0.5),
          p95Ms: percentile(goDurations, 0.95),
          warmupSamples: goWarmup.durations.length,
          measuredSamples: goDurations.length,
          errors: goErrors,
        },
      },
    };
    console.log(JSON.stringify(result));
    assert.equal(expressErrors, 0, `${path}: Express qualification requests must not fail`);
    assert.equal(goErrors, 0, `${path}: Go qualification requests must not fail`);
    assert.equal(
      expressWarmup.durations.length,
      warmupRequestsPerImplementation,
      `${path}: Express warmup sample count`,
    );
    assert.equal(
      goWarmup.durations.length,
      warmupRequestsPerImplementation,
      `${path}: Go warmup sample count`,
    );
    assert.equal(
      expressDurations.length,
      requestsPerImplementation,
      `${path}: Express measured sample count`,
    );
    assert.equal(
      goDurations.length,
      requestsPerImplementation,
      `${path}: Go measured sample count`,
    );
    assert.ok(
      result.qualification.go.p95Ms <= result.qualification.express.p95Ms * 1.1,
      `${path}: Go p95 must be at most 1.10 times Express p95`,
    );
  }
}

async function main() {
  requireHostedTestInputs();
  try {
    await runContract();
  } finally {
    goProcess?.kill();
    if (server?.listening) await closeServer(server).catch(() => undefined);
    await prisma?.$disconnect();
  }
}

void main().catch((error: unknown) => {
  console.error(error);
  process.exitCode = 1;
});
