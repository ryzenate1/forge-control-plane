import http from 'node:http';
import { URL } from 'node:url';

const PORT = Number(process.env.MOCK_API_PORT || 8080);

function json(res, status, body) {
  const payload = JSON.stringify(body);
  res.writeHead(status, { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(payload) });
  res.end(payload);
}

function handle(req, res) {
  const url = new URL(req.url || '/', `http://localhost:${PORT}`);
  const path = url.pathname;
  const method = (req.method || 'GET').toUpperCase();
  const cookie = req.headers.cookie || '';

  // Setup status - always not required for most tests, but allow override via header
  if (path.endsWith('/setup/status')) {
    // Allow forcing required via query ?required=1 for setup tests that need it
    const required = url.searchParams.get('required') === '1' || url.searchParams.get('mockRequired') === 'true';
    json(res, 200, { required, hasAdmin: !required, appVersion: '1.0.0-test' });
    return;
  }

  if (path.endsWith('/auth/me')) {
    // Session cookie is the hardened __Host- prefix in production; accept the
    // bare name only as the local-dev fallback (mirrors middleware.ts).
    const session = cookie.match(/(?:^|;\s*)__Host-forge_session=([^;]+)/) ?? cookie.match(/(?:^|;\s*)forge_session=([^;]+)/);
    if (session) {
      json(res, 200, { id: 'admin-1', email: 'admin@example.com', role: 'admin', username: 'admin' });
    } else {
      json(res, 401, { error: 'unauthorized' });
    }
    return;
  }

  if (path.endsWith('/auth/login') && method === 'POST') {
    let body = '';
    req.on('data', (chunk) => (body += chunk));
    req.on('end', () => {
      const payload = JSON.stringify({ complete: true, token: 'test-token', user: { id: 'u1', email: 'admin@example.com', role: 'admin' } });
      res.writeHead(200, {
        'Content-Type': 'application/json',
        'Content-Length': Buffer.byteLength(payload),
        'Set-Cookie': '__Host-forge_session=test-session; Path=/; Secure; HttpOnly; SameSite=Strict',
      });
      res.end(payload);
    });
    return;
  }

  if (path.includes('/nodes') && method === 'GET') {
    const nodes = [
      { id: 'node-1', uuid: 'node-1', name: 'Test Node 1', fqdn: 'node1.example.com', description: 'Test Node 1', status: 'online', memoryMb: 16384, cpuShares: 4, diskMb: 102400, daemonBase: '/srv', baseUrl: 'http://node1:8080', scheme: 'http', behindNat: false, maintenanceMode: false },
      { id: 'node-2', uuid: 'node-2', name: 'Test Node 2', fqdn: 'node2.example.com', description: 'Test Node 2', status: 'offline', memoryMb: 8192, cpuShares: 2, diskMb: 51200, daemonBase: '/srv', baseUrl: 'http://node2:8080', scheme: 'http', behindNat: false, maintenanceMode: false },
    ];
    // Support ?mockEmpty=1 to test empty state
    if (url.searchParams.get('mockEmpty') === '1') {
      json(res, 200, { data: [] });
      return;
    }
    if (path.match(/\/nodes\/[^/]+\/configuration/)) {
      json(res, 200, { token: 'cfg-token-123', nodeId: 'node-1', daemonBase: '/srv' });
      return;
    }
    if (path.match(/\/nodes\/[^/]+\/health/)) {
      json(res, 200, { status: 'online', checks: [{ name: 'daemon', status: 'ok' }], checkedAt: new Date().toISOString() });
      return;
    }
    if (path.match(/\/nodes\/[^/]+$/)) {
      json(res, 200, nodes[0]);
      return;
    }
    json(res, 200, { data: nodes });
    return;
  }

  if (path.endsWith('/health') || path.endsWith('/health/ready')) {
    json(res, 200, { status: 'ok', checks: [], checkedAt: new Date().toISOString() });
    return;
  }

  if (path.endsWith('/panel/settings/public')) {
    json(res, 200, { companyName: 'Test Panel' });
    return;
  }

  if (path.includes('/admin/activity')) {
    json(res, 200, { events: [], total: 0 });
    return;
  }

  // Default: empty data array for list endpoints, ok for others
  if (method === 'GET') {
    // Check if it looks like a list endpoint (plural)
    json(res, 200, { data: [] });
    return;
  }

  json(res, 200, { ok: true });
}

const server = http.createServer(handle);
server.listen(PORT, '127.0.0.1', () => {
  console.log(`[mock-api] listening on http://127.0.0.1:${PORT}`);
  // Signal readiness via stdout for Playwright polling
});
server.on('error', (err) => {
  console.error('[mock-api] failed to start', err);
  process.exit(1);
});

// Graceful shutdown
process.on('SIGTERM', () => server.close(() => process.exit(0)));
process.on('SIGINT', () => server.close(() => process.exit(0)));
