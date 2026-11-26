import assert from 'node:assert/strict';
import { type ChildProcess, spawn } from 'node:child_process';
import { createServer, type Server } from 'node:http';
import { setTimeout as delay } from 'node:timers/promises';

const databaseUrl = process.env.DATABASE_URL;
const goBinary = process.env.GO_CONTRACT_BINARY;
const ownerA = 'parity-owner-a';
const ownerB = 'parity-owner-b';
const timestamp = new Date('2026-01-02T03:04:05.000Z');
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

type ResponseValue = { status: number; body: unknown };

async function get(url: string, token?: string): Promise<ResponseValue> {
  assert.ok(cookieName);
  const response = await fetch(url, {
    headers: token ? { Cookie: `${cookieName}=${token}` } : {},
    signal: AbortSignal.timeout(5_000),
  });
  const body = response.status === 204 ? null : await response.json();
  return { status: response.status, body };
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
  return expressResponse;
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

  goProcess = spawn(goBinary!, [], { env: process.env, stdio: 'inherit' });
  await Promise.all([
    waitFor(`${expressUrl}/health`, goProcess),
    waitFor(`${goUrl}/health`, goProcess),
  ]);

  assert.ok(signAuthToken);
  const ownerAToken = signAuthToken(ownerA, 0);
  const ownerBToken = signAuthToken(ownerB, 0);

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

  console.log('Express and Go contract cases passed.');
  if (process.argv.includes('--load')) await compareLoad(ownerAToken);
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
