import '@/test-support/require-disposable-database';

import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { once } from 'node:events';
import type { AddressInfo } from 'node:net';
import test from 'node:test';

import { createApp } from '@/app';
import { AUTH_COOKIE_NAME, signAuthToken } from '@/lib/auth';
import { prisma } from '@/lib/prisma';

const TEST_NOW = new Date('2095-10-05T12:00:00.000Z');
const CACHE_FROM = new Date('2095-07-01T00:00:00.000Z');
const CACHE_TO = new Date('2095-10-06T00:00:00.000Z');
const CACHE_PROVIDERS = [
  'awesomeapi',
  'bcb_sgs_12',
  'coinbase',
  'ibge_sidra_1737_2266',
];

function dayKeys(from: string, to: string) {
  const keys: string[] = [];
  for (
    let at = new Date(`${from}T00:00:00.000Z`);
    at <= new Date(`${to}T00:00:00.000Z`);
    at = new Date(at.getTime() + 86_400_000)
  ) {
    keys.push(at.toISOString().slice(0, 10));
  }
  return keys;
}

function asKey(value: string) {
  return `${value.slice(0, 4)}-${value.slice(4, 6)}-${value.slice(6, 8)}`;
}

function installMarketProviderFake(t: import('node:test').TestContext) {
  const localFetch = globalThis.fetch;
  t.mock.method(
    globalThis,
    'fetch',
    async (input: string | URL | Request, init?: RequestInit) => {
      const url = new URL(input instanceof Request ? input.url : String(input));
      if (url.hostname === '127.0.0.1') return localFetch(input, init);

      if (
        url.hostname === 'economia.awesomeapi.com.br' &&
        url.pathname.includes('/daily/')
      ) {
        const from = asKey(url.searchParams.get('start_date') ?? '');
        const to = asKey(url.searchParams.get('end_date') ?? '');
        return Response.json(
          dayKeys(from, to).map((date) => ({
            bid: '5.123456789123',
            timestamp: String(new Date(`${date}T12:00:00.000Z`).getTime() / 1000),
          })),
        );
      }

      if (
        url.hostname === 'economia.awesomeapi.com.br' &&
        url.pathname.endsWith('/json/last/USD-BRL')
      ) {
        return Response.json({ USDBRL: { bid: '5.25' } });
      }

      if (url.hostname === 'query1.finance.yahoo.com') {
        const from = Number(url.searchParams.get('period1')) * 1000;
        const to = Number(url.searchParams.get('period2')) * 1000;
        const timestamps: number[] = [];
        for (let at = from; at < to; at += 86_400_000) {
          timestamps.push(at / 1000);
        }
        return Response.json({
          chart: {
            result: [
              {
                timestamp: timestamps,
                indicators: { quote: [{ close: timestamps.map(() => 50_000) }] },
              },
            ],
          },
        });
      }

      if (url.hostname === 'api.bcb.gov.br') {
        const seriesId = /bcdata\.sgs\.(\d+)/.exec(url.pathname)?.[1];
        const parseBcbDate = (value: string) => {
          const [day, month, year] = value.split('/');
          return `${year}-${month}-${day}`;
        };
        const from = parseBcbDate(url.searchParams.get('dataInicial') ?? '');
        const to = parseBcbDate(url.searchParams.get('dataFinal') ?? '');
        return Response.json(
          dayKeys(from, to)
            .filter(
              (date) =>
                ![0, 6].includes(
                  new Date(`${date}T00:00:00.000Z`).getUTCDay(),
                ),
            )
            .map((date) => ({
              data: `${date.slice(8, 10)}/${date.slice(5, 7)}/${date.slice(0, 4)}`,
              valor: seriesId === '12' ? '0.10' : '5.25',
            })),
        );
      }

      if (url.hostname === 'apisidra.ibge.gov.br') {
        const [from, to] =
          /\/p\/(\d{6})-(\d{6})/.exec(url.pathname)?.slice(1) ?? [];
        const monthKeys = (start: string, end: string) => {
          const result: string[] = [];
          let year = Number(start.slice(0, 4));
          let month = Number(start.slice(4, 6));
          const endYear = Number(end.slice(0, 4));
          const endMonth = Number(end.slice(4, 6));
          while (year < endYear || (year === endYear && month <= endMonth)) {
            result.push(`${year}${String(month).padStart(2, '0')}`);
            month += 1;
            if (month === 13) {
              year += 1;
              month = 1;
            }
          }
          return result;
        };
        return Response.json([
          { D1C: 'Nível territorial', D2C: 'Mês', V: 'Valor' },
          ...monthKeys(from ?? '209507', to ?? '209510').map((month) => ({
            D1C: month,
            V: '100.0',
          })),
        ]);
      }

      if (
        url.hostname === 'api.coinbase.com' &&
        url.pathname === '/v2/prices/BTC-BRL/spot'
      ) {
        return Response.json({ data: { amount: '260000' } });
      }

      throw new Error(
        `Unexpected external provider request: ${url.origin}${url.pathname}`,
      );
    },
  );
}

async function createUser(name: string) {
  return prisma.user.create({
    data: {
      name,
      email: `${name.toLowerCase().replaceAll(' ', '-')}-${randomUUID()}@example.invalid`,
      passwordHash: 'test-only-not-a-login-hash',
    },
  });
}

async function startApi(userId: string) {
  const app = createApp({ logRequests: false });
  const server = app.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const address = server.address() as AddressInfo;
  const origin = `http://127.0.0.1:${address.port}`;
  const cookie = `${AUTH_COOKIE_NAME}=${signAuthToken(userId, 0)}`;

  return {
    async get(path: string) {
      const response = await fetch(`${origin}${path}`, { headers: { Cookie: cookie } });
      return { response, body: (await response.json()) as unknown };
    },
    async close() {
      await new Promise<void>((resolve, reject) => {
        server.close((error) => (error ? reject(error) : resolve()));
      });
    },
  };
}

async function clearSyntheticMarketCache() {
  await prisma.marketDataCache.deleteMany({
    where: {
      provider: { in: CACHE_PROVIDERS },
      at: { gte: CACHE_FROM, lt: CACHE_TO },
    },
  });
}

async function cleanupUsers(userIds: string[]) {
  const [expenses, entries] = await Promise.all([
    prisma.expense.findMany({
      where: { userId: { in: userIds } },
      select: { id: true },
    }),
    prisma.entry.findMany({
      where: { userId: { in: userIds } },
      select: { id: true },
    }),
  ]);
  const expenseIds = expenses.map(({ id }) => id);
  const entryIds = entries.map(({ id }) => id);

  await prisma.expensePayment.deleteMany({
    where: { expenseId: { in: expenseIds } },
  });
  await prisma.entryReceipt.deleteMany({
    where: { entryId: { in: entryIds } },
  });
  await prisma.salaryHold.deleteMany({ where: { entryId: { in: entryIds } } });
  await prisma.salaryVersion.deleteMany({
    where: { entryId: { in: entryIds } },
  });
  await prisma.expenseSplit.deleteMany({
    where: { expenseId: { in: expenseIds } },
  });
  await prisma.assetTransaction.deleteMany({ where: { userId: { in: userIds } } });
  await prisma.piggyTransaction.deleteMany({ where: { userId: { in: userIds } } });
  await prisma.expense.deleteMany({ where: { userId: { in: userIds } } });
  await prisma.entry.deleteMany({ where: { userId: { in: userIds } } });
  await prisma.piggyBank.deleteMany({ where: { userId: { in: userIds } } });
  await prisma.user.deleteMany({ where: { id: { in: userIds } } });
}

test('Express patrimony summary and history consolidate synthetic ledgers by date and owner', async (t) => {
  t.mock.timers.enable({ apis: ['Date'], now: TEST_NOW });
  installMarketProviderFake(t);
  const users: Awaited<ReturnType<typeof createUser>>[] = [];
  const apis: Array<Awaited<ReturnType<typeof startApi>>> = [];

  try {
    await clearSyntheticMarketCache();
    const ownerA = await createUser('Patrimony summary owner');
    users.push(ownerA);
    const ownerB = await createUser('Patrimony history owner');
    users.push(ownerB);
    await prisma.patrimonySettings.createMany({
      data: [
        {
          userId: ownerA.id,
          baseDate: new Date('2095-10-01T12:00:00.000Z'),
          openingCashBrl: '1000.00',
        },
        {
          userId: ownerB.id,
          baseDate: new Date('2095-10-01T12:00:00.000Z'),
          openingCashBrl: '321.99',
        },
      ],
    });

    await prisma.entry.create({
      data: {
        userId: ownerA.id,
        name: 'Synthetic received income',
        amount: '100.01',
        type: 'freelance',
        frequency: 'unica',
        date: new Date('2095-10-02T12:00:00.000Z'),
        receipts: {
          create: [
            {
              month: '2095-10',
              amount: '100.01',
              receivedAt: new Date('2095-10-02T12:00:00.000Z'),
            },
          ],
        },
      },
    });

    await prisma.expense.create({
      data: {
        userId: ownerA.id,
        name: 'Synthetic paid expense',
        amount: '30.02',
        category: 'outro',
        frequency: 'unica',
        occurredAt: new Date('2095-10-03T12:00:00.000Z'),
        payments: {
          create: [
            {
              month: '2095-10',
              amount: '30.02',
              paidAt: new Date('2095-10-03T12:00:00.000Z'),
            },
          ],
        },
      },
    });

    const usdBuy = await prisma.expense.create({
      data: {
        userId: ownerA.id,
        name: 'Synthetic USD purchase',
        amount: '50.01',
        category: 'investimento',
        frequency: 'unica',
        occurredAt: new Date('2095-10-02T12:00:00.000Z'),
        systemOrigin: 'asset',
      },
    });
    await prisma.assetTransaction.create({
      data: {
        userId: ownerA.id,
        asset: 'USD',
        type: 'BUY',
        quantity: '10.123456789012',
        cashAmountBrl: '50.01',
        date: new Date('2095-10-02T12:00:00.000Z'),
        expenseId: usdBuy.id,
      },
    });

    const btcBuy = await prisma.expense.create({
      data: {
        userId: ownerA.id,
        name: 'Synthetic BTC purchase',
        amount: '10.01',
        category: 'investimento',
        frequency: 'unica',
        occurredAt: new Date('2095-10-02T12:00:00.000Z'),
        systemOrigin: 'asset',
      },
    });
    await prisma.assetTransaction.create({
      data: {
        userId: ownerA.id,
        asset: 'BTC',
        type: 'BUY',
        quantity: '0.000100000000',
        cashAmountBrl: '10.01',
        date: new Date('2095-10-02T12:00:00.000Z'),
        expenseId: btcBuy.id,
      },
    });

    const bank = await prisma.piggyBank.create({
      data: { userId: ownerA.id, name: 'Synthetic reserve' },
    });
    const piggyExpense = await prisma.expense.create({
      data: {
        userId: ownerA.id,
        name: 'Synthetic reserve deposit',
        amount: '25.03',
        category: 'cofrinho',
        frequency: 'unica',
        occurredAt: new Date('2095-10-04T12:00:00.000Z'),
        systemOrigin: 'piggy',
      },
    });
    await prisma.piggyTransaction.create({
      data: {
        userId: ownerA.id,
        piggyBankId: bank.id,
        type: 'deposit',
        amount: '25.03',
        date: new Date('2095-10-04T12:00:00.000Z'),
        expenseId: piggyExpense.id,
      },
    });

    await prisma.expense.create({
      data: {
        userId: ownerB.id,
        name: 'Other owner expense',
        amount: '1.11',
        category: 'outro',
        frequency: 'unica',
        occurredAt: new Date('2095-10-03T12:00:00.000Z'),
        payments: {
          create: [
            {
              month: '2095-10',
              amount: '1.11',
              paidAt: new Date('2095-10-03T12:00:00.000Z'),
            },
          ],
        },
      },
    });

    const apiA = await startApi(ownerA.id);
    apis.push(apiA);
    const apiB = await startApi(ownerB.id);
    apis.push(apiB);

    const summaryResponse = await apiA.get('/patrimony/summary');
    assert.equal(summaryResponse.response.status, 200);
    const summary = summaryResponse.body as {
      settings: { baseDate: string; openingCashBrl: string };
      summary: Record<string, string | null>;
      stale: Record<string, boolean>;
    };
    assert.deepEqual(summary.settings, {
      baseDate: '2095-10-01',
      openingCashBrl: '1000',
    });
    assert.equal(summary.summary.cashBrl, '984.94');
    assert.equal(summary.summary.piggyBrl, '25.03');
    assert.equal(summary.summary.btcBrl, '26');
    assert.equal(summary.summary.usdBrl, '53.148148142313');
    assert.equal(summary.summary.patrimonyBrl, '1089.118148142313');
    assert.deepEqual(summary.stale, { btc: false, usd: false, cdi: false, ipca: false });
    assert.equal('history' in summary, false);

    const historyResponse = await apiA.get(
      '/patrimony/history?from=2095-10-02&to=2095-10-05',
    );
    assert.equal(historyResponse.response.status, 200);
    const history = historyResponse.body as {
      settings: { baseDate: string; openingCashBrl: string };
      summary: Record<string, string | null>;
      history: Array<Record<string, string>>;
    };
    assert.deepEqual(history.history.map(({ date }) => date), [
      '2095-10-02',
      '2095-10-03',
      '2095-10-04',
      '2095-10-05',
    ]);
    assert.equal(history.history[0].cashBrl, '1039.99');
    assert.equal(history.history[1].cashBrl, '1009.97');
    assert.equal(history.history[2].cashBrl, '984.94');
    assert.equal(history.history[2].piggyBrl, '25.03');
    const { date: latestDate, ...latest } = history.history.at(-1)!;
    assert.equal(latestDate, '2095-10-05');
    assert.deepEqual(latest, summary.summary);

    const ownerBSummaryResponse = await apiB.get('/patrimony/summary');
    assert.equal(ownerBSummaryResponse.response.status, 200);
    const ownerBSummary = ownerBSummaryResponse.body as typeof summary;
    assert.deepEqual(ownerBSummary.settings, {
      baseDate: '2095-10-01',
      openingCashBrl: '321.99',
    });
    assert.equal(ownerBSummary.summary.cashBrl, '320.88');
    assert.equal(ownerBSummary.summary.piggyBrl, '0');
    assert.equal(ownerBSummary.summary.btcBrl, '0');
    assert.equal(ownerBSummary.summary.usdBrl, '0');
    assert.equal(ownerBSummary.summary.patrimonyBrl, '320.88');

    const ownerBHistoryResponse = await apiB.get(
      '/patrimony/history?from=2095-10-02&to=2095-10-05',
    );
    assert.equal(ownerBHistoryResponse.response.status, 200);
    const ownerBHistory = ownerBHistoryResponse.body as typeof history;
    assert.deepEqual(ownerBHistory.history.map(({ date }) => date), [
      '2095-10-02',
      '2095-10-03',
      '2095-10-04',
      '2095-10-05',
    ]);
    assert.deepEqual(ownerBHistory.history.map(({ cashBrl }) => cashBrl), [
      '321.99',
      '320.88',
      '320.88',
      '320.88',
    ]);
    assert.equal(ownerBHistory.summary.patrimonyBrl, '320.88');
  } finally {
    await Promise.all(apis.map((api) => api.close()));
    await cleanupUsers(users.map(({ id }) => id));
    await clearSyntheticMarketCache();
  }
});
