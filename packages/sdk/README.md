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

Non-2xx responses throw an `ApiError` (same shape as the web client's
`ApiError` in `forge/web/lib/api/http.ts`):

```typescript
import { ForgeApiClient, ApiError, isApiError } from '@forge/sdk';

try {
  await client.getServer('missing');
} catch (err) {
  if (isApiError(err)) {
    console.error(err.status, err.statusText, err.data);
    // err.details carries structured 422 validation errors ({ errors } /
    // { details } / { fields } from the response body) when present.
  }
}
```

A 401 response additionally fires the `onUnauthorized` hook (the SDK
equivalent of the web client's `forge:session-expired` signal), so the host
app can clear auth state or redirect to login:

```typescript
const client = createApiClient({
  baseUrl: 'https://panel.example.com',
  onUnauthorized: () => window.location.assign('/login'),
});
```

### Retry policy and idempotency

Only idempotent `GET` requests are retried automatically (up to 3 attempts on
429/502/503/504 with `Retry-After` support, plus transient transport
failures). Mutations (`POST`/`PUT`/`PATCH`/`DELETE`) run exactly once:
automatically retrying a non-idempotent write could apply it twice (duplicate
server, double side effect).

To make a mutation safely retryable, attach an idempotency key — per client
or per call — which is sent as the `Idempotency-Key` header so the server can
de-duplicate repeated submissions:

```typescript
const client = createApiClient({
  baseUrl: 'https://panel.example.com',
  idempotencyKey: crypto.randomUUID(), // default for all mutations
});
```

## Development

```bash
npm run build       # tsc -p tsconfig.json
npm run typecheck   # tsc --noEmit
npm test            # vitest run
```