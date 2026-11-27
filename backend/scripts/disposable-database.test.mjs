import assert from 'node:assert/strict';
import test from 'node:test';

import { assertDisposableDatabaseUrl } from '../src/test-support/disposable-database.ts';

test('disposable database guard accepts only loopback and the dedicated test database', () => {
  for (const host of ['localhost', '127.0.0.1', '[::1]']) {
    assert.doesNotThrow(() =>
      assertDisposableDatabaseUrl(
        `postgresql://test:test@${host}:5432/demanage_backend_test?schema=public`,
      ),
    );
  }
});

test('disposable database guard rejects remote and non-dedicated targets', () => {
  for (const databaseUrl of [
    'postgresql://test:test@db.example.com:5432/demanage_backend_test',
    'postgresql://test:test@localhost:5432/demanage',
    'postgresql://test:test@localhost:5432/demanage_backend_test?host=db.example.com',
    'not a database url',
    undefined,
  ]) {
    assert.throws(() => assertDisposableDatabaseUrl(databaseUrl));
  }
});
