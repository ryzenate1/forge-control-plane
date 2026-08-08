# @forge/sdk

TypeScript SDK for the GamePanel (Forge) API. Generated against the real
`/api/v1` routes exposed by `forge/api`.

## Install

```bash
npm install @forge/sdk
```

## Usage

```typescript
import { ForgeApiClient, createApiClient } from '@forge/sdk';

// baseUrl may include the `/api/v1` suffix or omit it — it is appended when missing.
const client = createApiClient({ baseUrl: 'https://panel.example.com' });

const servers = await client.listServers();
const server = await client.getServer('server-uuid');
```

## Configuration

```typescript
interface ApiClientConfig {
  /** Base URL of the API. `/api/v1` is appended when missing. */
  baseUrl: string;
  /** Bearer token used for `Authorization: Bearer` (takes precedence over apiKey). */
  token?: string;
  /** API key used for `X-API-Key`. */
  apiKey?: string;
  /** Custom fetch implementation (defaults to globalThis.fetch). */
  fetch?: typeof globalThis.fetch;
  /** Extra headers applied to every request. */
  headers?: HeadersInit;
  /** Cookie-session auth: sends credentials and `X-CSRF-Token` on mutations. */
  useCookies?: boolean;
  /** Per-request timeout in milliseconds (default: 30000). */
  timeoutMs?: number;
}
```

### Authentication

Auth is cookie-session based on the server. `login()` sets HttpOnly session and
CSRF cookies, so pass `useCookies: true` (browser) or a cookie jar (server) when
using session auth:

```typescript
const client = createApiClient({
  baseUrl: 'https://panel.example.com',
  useCookies: true,
});
await client.login({ email: 'admin@example.com', password: 'secret' });
const me = await client.me();
```

Machine access can instead use a bearer token or API key:

```typescript
const client = createApiClient({
  baseUrl: 'https://panel.example.com',
  token: 'forge_nt_...',
});
```

### Servers

```typescript
const servers = await client.listServers();
const server = await client.getServer(server.id);
await client.sendPowerAction(server.id, 'start'); // 'start' | 'stop' | 'restart' | 'kill'
await client.sendCommand(server.id, 'say hello');
const stats = await client.getServerStats(server.id);
```

### Nodes

```typescript
const nodes = await client.listNodes();
const node = await client.getNode(nodeId);
const created = await client.createNode({ name: 'edge-1', fqdn: 'node1.example.com' });
// created.node, created.token
```

### Errors

Non-2xx responses throw an `ApiError`:

```typescript
try {
  await client.getServer('missing');
} catch (err) {
  if (err instanceof ApiError) {
    console.error(err.status, err.statusText, err.data);
  }
}
```

## Development

```bash
npm run build       # tsc -p tsconfig.json
npm run typecheck   # tsc --noEmit
npm test            # vitest run
```