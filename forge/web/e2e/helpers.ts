import type { Page, Route } from '@playwright/test';

export async function mockSetupStatus(page: Page, required = false) {
  await page.route('**/api/v1/setup/status', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ required, hasAdmin: !required, appVersion: '1.0.0-test' }),
    });
  });
  // Also handle Next rewrite path /api/setup/status? Check both
  await page.route('**/api/setup/status', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ required, hasAdmin: !required, appVersion: '1.0.0-test' }),
    });
  });
}

export async function mockAuthMeUnauthenticated(page: Page) {
  await page.route('**/api/v1/auth/me', async (route: Route) => {
    await route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ error: 'unauthorized' }) });
  });
}

export async function mockAuthMe(page: Page, user: Record<string, unknown> = { id: 'u1', email: 'admin@example.com', role: 'admin' }) {
  await page.route('**/api/v1/auth/me', async (route: Route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(user) });
  });
}

export async function mockLoginSuccess(page: Page) {
  await page.route('**/api/v1/auth/login', async (route: Route) => {
    const body = { complete: true, token: 'test-token', user: { id: 'u1', email: 'admin@example.com', role: 'admin' } };
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
  });
  await page.route('**/api/v1/auth/me', async (route: Route) => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ id: 'u1', email: 'admin@example.com', role: 'admin' }) });
  });
}

export async function mockNodes(page: Page) {
  const nodes = [
    { id: 'node-1', uuid: 'node-1', name: 'Test Node 1', fqdn: 'node1.example.com', status: 'online', memoryMb: 16384, cpuShares: 4, diskMb: 102400, daemonBase: '/srv', baseUrl: 'http://node1:8080', scheme: 'http', behindNat: false, maintenanceMode: false },
    { id: 'node-2', uuid: 'node-2', name: 'Test Node 2', fqdn: 'node2.example.com', status: 'offline', memoryMb: 8192, cpuShares: 2, diskMb: 51200, daemonBase: '/srv', baseUrl: 'http://node2:8080', scheme: 'http', behindNat: false, maintenanceMode: false },
  ];
  // Mirror the real paginated envelope (handlers_admin.go): { data,
  // meta: { pagination: { current, total, count, per_page, total_records } } }
  // where `total` is the page count. Mocks that omit `meta` would make
  // fetchAllNodes silently stop after one page, hiding fan-out regressions.
  const envelope = {
    data: nodes,
    meta: { pagination: { current: 1, total: 1, count: nodes.length, per_page: 100, total_records: nodes.length } },
  };
  await page.route('**/api/v1/nodes**', async (route: Route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(envelope) });
    } else {
      await route.continue();
    }
  });
  await page.route('**/api/v1/nodes/*', async (route: Route) => {
    if (route.request().method() === 'GET') {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(nodes[0]) });
    } else {
      await route.continue();
    }
  });
}

function paginatedEnvelope<T>(rows: T[], page = 1, perPage = 100) {
  const total = Math.max(1, Math.ceil(rows.length / perPage));
  return {
    data: rows,
    meta: { pagination: { current: page, total, count: rows.length, per_page: perPage, total_records: rows.length } },
  };
}

export { paginatedEnvelope };

export async function mockPanelHealth(page: Page) {
  await page.route('**/api/v1/health**', async (route: Route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ status: 'ok', checks: [], checkedAt: new Date().toISOString() }),
    });
  });
}
