/* eslint-disable simple-import-sort/imports -- isolate Prisma from inherited DATABASE_URL */
import '@/test-support/offline-prisma-env';

import assert from 'node:assert/strict';
import { once } from 'node:events';
import type { AddressInfo } from 'node:net';
import test from 'node:test';
import cookieParser from 'cookie-parser';
import express from 'express';

import { Prisma, type AssetTransaction, type User } from '@/generated/prisma/client';
import { errorHandler, notFoundHandler } from '@/http/errors';
import { AUTH_COOKIE_NAME, signAuthToken } from '@/lib/auth';
import { prisma } from '@/lib/prisma';

import assetsRoutes from './assets';

const OWNER_ID = 'asset-summary-owner';
const OTHER_OWNER_ID = 'asset-summary-other-owner';
const AS_OF = new Date('2026-01-10T12:00:00.000Z');

function user(id: string): User {
  return {
    id,
    name: id,
    email: `${id}@example.invalid`,
    passwordHash: 'synthetic-test-only',
    recoveryCodeHash: null,
    recoveryCodeCreatedAt: null,
    sessionVersion: 0,
    trustedDeviceVersion: 0,
    trustedProofFailures: 0,
    trustedProofWindowAt: null,
    salary: new Prisma.Decimal('0.00'),
    notes: null,
    createdAt: AS_OF,
    updatedAt: AS_OF,
  } as User;
}

function transaction(): AssetTransaction {
  return {
    id: 'synthetic-btc-buy',
    userId: OWNER_ID,
    asset: 'BTC',
    type: 'BUY',
    quantity: new Prisma.Decimal('0.125'),
    cashAmountBrl: new Prisma.Decimal('6250.00'),
    feeAmountBrl: new Prisma.Decimal('12.34'),
    feePercent: null,
    costBasisKnown: true,
    date: AS_OF,
    note: null,
    expenseId: null,
    entryId: null,
    createdAt: AS_OF,
  } as AssetTransaction;
}

test('GET /assets and GET /assets/:asset summarize BTC/USD with synthetic data and owner scope', async (t) => {
  const owners = new Map([
    [OWNER_ID, user(OWNER_ID)],
    [OTHER_OWNER_ID, user(OTHER_OWNER_ID)],
  ]);
  const queries: Array<{ userId: string; asset: string }> = [];
  const cacheWrites: Array<{ provider: string; key: string; value: string }> = [];
  const providerUrls: string[] = [];
  let providerFails = false;
  const localFetch = globalThis.fetch;

  t.mock.method(prisma.user, 'findUnique', async ({ where }) =>
    (owners.get(where.id) ?? null) as never,
  );
  t.mock.method(prisma.assetTransaction, 'findMany', async ({ where }) => {
    const userId = where.userId ?? '';
    const asset = where.asset ?? '';
    queries.push({ userId, asset });
    return (userId === OWNER_ID && asset === 'BTC' ? [transaction()] : []) as never;
  });
  t.mock.method(prisma.marketDataCache, 'findFirst', async () => null as never);
  t.mock.method(prisma.marketDataCache, 'upsert', async ({ where, create }) => {
    cacheWrites.push({
      provider: where.provider_key_at.provider,
      key: where.provider_key_at.key,
      value: String(create.value),
    });
    return {} as never;
  });
  t.mock.method(globalThis, 'fetch', async (input, init) => {
    const url = String(input);
    if (url.startsWith('http://127.0.0.1:')) return localFetch(input, init);
    providerUrls.push(url);
    if (providerFails) throw new TypeError('synthetic provider outage');
    if (url.includes('coinbase.com')) {
      return new Response(JSON.stringify({ data: { amount: '51234.56789123' } }), {
        headers: { 'content-type': 'application/json' },
      });
    }
    if (url.includes('awesomeapi.com.br')) {
      return new Response(JSON.stringify({ USDBRL: { bid: '5.43210987' } }), {
        headers: { 'content-type': 'application/json' },
      });
    }
    throw new Error(`unexpected synthetic provider URL: ${url}`);
  });

  const app = express();
  app.use(cookieParser());
  app.use('/assets', assetsRoutes);
  app.use(notFoundHandler);
  app.use(errorHandler);
  const server = app.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const origin = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
  const cookie = (ownerId: string) =>
    `${AUTH_COOKIE_NAME}=${signAuthToken(ownerId, 0)}`;
  const request = (path: string, ownerId = OWNER_ID) =>
    fetch(`${origin}${path}`, { headers: { Cookie: cookie(ownerId) } });

  try {
    const listResponse = await request('/assets');
    assert.equal(listResponse.status, 200);
    const list = (await listResponse.json()) as {
      BTC: Record<string, unknown>;
      USD: Record<string, unknown>;
    };
    assert.deepEqual(Object.keys(list), ['BTC', 'USD']);
    assert.equal(list.BTC.quantity, '0.125');
    assert.equal(list.BTC.knownQuantity, '0.125');
    assert.equal(list.BTC.investedBrl, '6250');
    assert.equal(list.BTC.feesBrl, '12.34');
    assert.equal(list.BTC.quoteBrl, '51234.56789123');
    assert.equal(list.BTC.marketValueBrl, '6404.32098640375');
    assert.equal(list.USD.quantity, '0');
    assert.equal(list.USD.averageCostBrl, null);
    assert.equal(list.USD.totalPnlPercent, null);
    const btcQuote = list.BTC.quote as Record<string, unknown>;
    assert.equal(btcQuote.valueBrl, '51234.56789123');
    assert.equal(btcQuote.stale, false);
    assert.equal(btcQuote.provider, 'coinbase');
    assert.match(String(btcQuote.asOf), /^\d{4}-\d{2}-\d{2}T/);

    const btcResponse = await request('/assets/BTC');
    assert.equal(btcResponse.status, 200);
    const btc = (await btcResponse.json()) as Record<string, unknown>;
    assert.equal(btc.asset, 'BTC');
    assert.equal(btc.marketValueBrl, '6404.32098640375');

    const usdResponse = await request('/assets/usd');
    assert.equal(usdResponse.status, 200);
    const usd = (await usdResponse.json()) as Record<string, unknown>;
    assert.equal(usd.asset, 'USD');
    assert.equal(usd.quoteBrl, '5.43210987');
    assert.equal(usd.averageCostBrl, null);
    assert.equal(usd.totalPnlPercent, null);

    const otherOwnerResponse = await request('/assets', OTHER_OWNER_ID);
    assert.equal(otherOwnerResponse.status, 200);
    const otherOwner = (await otherOwnerResponse.json()) as {
      BTC: Record<string, unknown>;
      USD: Record<string, unknown>;
    };
    assert.equal(otherOwner.BTC.quantity, '0');
    assert.equal(otherOwner.USD.quantity, '0');
    const otherOwnerBtcResponse = await request('/assets/BTC', OTHER_OWNER_ID);
    assert.equal(otherOwnerBtcResponse.status, 200);
    assert.equal(
      ((await otherOwnerBtcResponse.json()) as { quantity: string }).quantity,
      '0',
    );
    assert.ok(
      queries.every(
        ({ userId }) => userId === OWNER_ID || userId === OTHER_OWNER_ID,
      ),
    );
    assert.ok(queries.some(({ userId, asset }) => userId === OWNER_ID && asset === 'BTC'));
    assert.ok(queries.some(({ userId, asset }) => userId === OTHER_OWNER_ID && asset === 'BTC'));

    assert.deepEqual(
      cacheWrites
        .slice(0, 2)
        .map(({ provider, key, value }) => ({ provider, key, value }))
        .sort((left, right) => left.provider.localeCompare(right.provider)),
      [
        { provider: 'coinbase', key: 'BTC_BRL_QUOTE', value: '51234.56789123' },
        { provider: 'awesomeapi', key: 'USD_BRL_QUOTE', value: '5.43210987' },
      ],
    );

    providerFails = true;
    const failedList = await request('/assets');
    assert.equal(failedList.status, 503);
    assert.deepEqual(await failedList.json(), {
      error: 'Não foi possível obter as cotações e não há cache suficiente',
    });

    const failedQuote = await request('/assets/BTC');
    assert.equal(failedQuote.status, 503);
    assert.deepEqual(await failedQuote.json(), {
      error: 'Cotação indisponível e sem cache',
    });
    assert.ok(providerUrls.every((url) => url.startsWith('https://')));
  } finally {
    await new Promise<void>((resolve, reject) => {
      server.close((error) => (error ? reject(error) : resolve()));
    });
  }
});
