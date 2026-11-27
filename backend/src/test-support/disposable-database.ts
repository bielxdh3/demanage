export function assertDisposableDatabaseUrl(databaseUrl: string | undefined) {
  let parsed: URL;
  try {
    parsed = new URL(databaseUrl ?? '');
  } catch {
    throw new Error('DATABASE_URL must target the dedicated loopback test database');
  }

  const validSchema =
    parsed.searchParams.size === 0 ||
    (parsed.searchParams.size === 1 && parsed.searchParams.get('schema') === 'public');

  if (
    parsed.protocol !== 'postgresql:' ||
    !['localhost', '127.0.0.1', '[::1]'].includes(parsed.hostname) ||
    parsed.pathname !== '/demanage_backend_test' ||
    !validSchema ||
    parsed.hash !== ''
  ) {
    throw new Error('DATABASE_URL must target the dedicated loopback test database');
  }

  return parsed;
}
