import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  ApiError,
  ForgeApiClient,
  combineSignals,
  createApiClient,
} from '../client';

function jsonResponse(body: unknown, status = 200, headers: Record<string, string> = {}): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json', ...headers },
  });
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
  delete (globalThis as Record<string, unknown>).document;
});

describe('request()', () => {
  it('sends JSON GETs and parses JSON bodies', async () => {
    const fetchMock = vi.fn(async () => jsonResponse({ data: [{ id: 'n1' }] }));
    const client = new ForgeApiClient({ baseUrl: 'https://panel.example.com', fetch: fetchMock as unknown as typeof fetch });

    const result = await client.listNodes();

    expect(result).toEqual([{ id: 'n1' }]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('https://panel.example.com/api/v1/nodes');
    expect(init.method).toBe('GET');
  });

  it('appends /api/v1 to a baseUrl that lacks it', async () => {
    const fetchMock = vi.fn(async () => jsonResponse({}));
    const client = new ForgeApiClient({ baseUrl: 'https://panel.example.com', fetch: fetchMock as unknown as typeof fetch });
    await client.getHealth();
    const [url] = fetchMock.mock.calls[0] as unknown as [string];
    expect(url).toBe('https://panel.example.com/api/v1/health');
  });

  it('retries a 429 and respects Retry-After', async () => {
    vi.useFakeTimers();
    const fetchMock = vi
      .fn()
      .mockImplementationOnce(async () => new Response('rate limited', { status: 429, headers: { 'retry-after': '0' } }))
      .mockImplementationOnce(async () => jsonResponse([{ id: '1' }]));

    const client = new ForgeApiClient({ baseUrl: 'https://panel.example.com/api/v1', fetch: fetchMock as unknown as typeof fetch });
    const promise = client.listUsers();
    await vi.advanceTimersByTimeAsync(200);

    await expect(promise).resolves.toEqual([{ id: '1' }]);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('throws ApiError with HTTP status for non-JSON error bodies', async () => {
    const fetchMock = vi.fn(async () => new Response('<html>boom</html>', { status: 500, statusText: 'Internal Server Error' }));
    const client = new ForgeApiClient({ baseUrl: 'https://panel.example.com/api/v1', fetch: fetchMock as unknown as typeof fetch });

    await expect(client.getHealth()).rejects.toMatchObject({
      name: 'ApiError',
      status: 500,
      statusText: 'Internal Server Error',
      data: '<html>boom</html>',
    });
  });

  it('does not retry non-idempotent 429s', async () => {
    const fetchMock = vi.fn(async () => new Response('rate limited', { status: 429, headers: { 'retry-after': '1' } }));
    const client = new ForgeApiClient({ baseUrl: 'https://panel.example.com/api/v1', fetch: fetchMock as unknown as typeof fetch });

    await expect(client.sendCommand('s1', 'say hi')).rejects.toMatchObject({ status: 429 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('propagates transport AbortError as-is (no retry, no wrap)', async () => {
    const fetchMock = vi.fn(async () => {
      throw new DOMException('The operation was aborted', 'AbortError');
    });
    const client = new ForgeApiClient({
      baseUrl: 'https://panel.example.com/api/v1',
      fetch: fetchMock as unknown as typeof fetch,
      timeoutMs: 1000,
    });

    await expect(client.listServers()).rejects.toMatchObject({ name: 'AbortError' });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('retries idempotent GETs when the internal timeout fires', async () => {
    vi.useFakeTimers();
    let calls = 0;
    const fetchMock = vi.fn(
      (_url: string, init: RequestInit) =>
        new Promise((_resolve, reject) => {
          calls++;
          init.signal!.addEventListener('abort', () => reject(init.signal!.reason));
        })
    );
    const client = new ForgeApiClient({
      baseUrl: 'https://panel.example.com/api/v1',
      fetch: fetchMock as unknown as typeof fetch,
      timeoutMs: 1000,
    });

    const promise = client.listUsers();
    // Attach a handler immediately so the fake-timer abort rejection is never
    // observed as unhandled while the timers below advance.
    void promise.catch(() => {});
    await vi.advanceTimersByTimeAsync(1000); // attempt 1: timeout fires
    await vi.advanceTimersByTimeAsync(1000); // backoff (1000ms)
    await vi.advanceTimersByTimeAsync(1000); // attempt 2: timeout fires
    await vi.advanceTimersByTimeAsync(2000); // backoff (2000ms)
    await vi.advanceTimersByTimeAsync(1000); // attempt 3: timeout fires

    await expect(promise).rejects.toThrow('Request timeout');
    expect(calls).toBe(3);
  });

  it('returns {} for 204 responses', async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 204 }));
    const client = new ForgeApiClient({ baseUrl: 'https://panel.example.com/api/v1', fetch: fetchMock as unknown as typeof fetch });

    await expect(client.deleteServer('s1')).resolves.toEqual({});
  });

  it('guards against null/empty bodies on 200s', async () => {
    const fetchMock = vi.fn(async () => new Response('', { status: 200 }));
    const client = new ForgeApiClient({ baseUrl: 'https://panel.example.com/api/v1', fetch: fetchMock as unknown as typeof fetch });

    await expect(client.refreshSession()).resolves.toEqual({});
  });

  it('returns raw text for raw responses (readFile)', async () => {
    const fetchMock = vi.fn(async () => new Response('hello world', { status: 200 }));
    const client = new ForgeApiClient({ baseUrl: 'https://panel.example.com/api/v1', fetch: fetchMock as unknown as typeof fetch });

    await expect(client.readFile('s1', '/etc/config.yml')).resolves.toBe('hello world');
    const [url] = fetchMock.mock.calls[0] as unknown as [string];
    expect(url).toBe('https://panel.example.com/api/v1/servers/s1/files/content?path=%2Fetc%2Fconfig.yml');
  });

  it('sends raw string bodies for writeFile', async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 204 }));
    const client = new ForgeApiClient({ baseUrl: 'https://panel.example.com/api/v1', fetch: fetchMock as unknown as typeof fetch });

    await client.writeFile('s1', '/a.txt', 'plain text');
    const [_url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(init.method).toBe('PUT');
    expect(init.body).toBe('plain text');
    expect((init.headers as Record<string, string>)['Content-Type']).toBe('text/plain; charset=utf-8');
  });
});

describe('CSRF cookie handling', () => {
  function makeClient(fetchMock: typeof fetch): ForgeApiClient {
    return new ForgeApiClient({
      baseUrl: 'https://panel.example.com/api/v1',
      fetch: fetchMock,
      useCookies: true,
    });
  }

  it('sends X-CSRF-Token from the __Host-forge_csrf cookie on mutations', async () => {
    (globalThis as Record<string, unknown>).document = { cookie: '__Host-forge_csrf=csrf-123; __Host-forge_session=abc' } as unknown as Document;
    const fetchMock = vi.fn(async () => jsonResponse({ ok: true }));
    const client = makeClient(fetchMock as unknown as typeof fetch);

    await client.deleteUser('u1');

    const [_url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect((init.headers as Record<string, string>)['X-CSRF-Token']).toBe('csrf-123');
    expect(init.credentials).toBe('include');
  });

  it('also matches the dev-mode forge_csrf (non-__Host-) cookie', async () => {
    (globalThis as Record<string, unknown>).document = { cookie: 'forge_csrf=dev-token; session=xyz' } as unknown as Document;
    const fetchMock = vi.fn(async () => jsonResponse({ ok: true }));
    const client = makeClient(fetchMock as unknown as typeof fetch);

    await client.deleteUser('u1');

    const [_url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect((init.headers as Record<string, string>)['X-CSRF-Token']).toBe('dev-token');
  });

  it('does not attach CSRF headers on GETs', async () => {
    (globalThis as Record<string, unknown>).document = { cookie: '__Host-forge_csrf=csrf-123' } as unknown as Document;
    const fetchMock = vi.fn(async () => jsonResponse({}));
    const client = makeClient(fetchMock as unknown as typeof fetch);

    await client.getHealth();

    const [_url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect((init.headers as Record<string, string>)['X-CSRF-Token']).toBeUndefined();
  });
});

describe('combineSignals', () => {
  it('removes abort listeners from inputs once the combined signal aborts', () => {
    const external = new AbortController();
    const addSpy = vi.spyOn(external.signal, 'addEventListener');
    const removeSpy = vi.spyOn(external.signal, 'removeEventListener');

    const combined = combineSignals(external.signal);
    expect(addSpy).toHaveBeenCalledWith('abort', expect.any(Function), { once: true });

    external.abort();

    expect(removeSpy).toHaveBeenCalledWith('abort', expect.any(Function));
  });

  it('removes abort listeners when an input signal aborts', () => {
    const external = new AbortController();
    const removeSpy = vi.spyOn(external.signal, 'removeEventListener');

    combineSignals(external.signal);
    external.abort(new Error('boom'));

    expect(removeSpy).toHaveBeenCalledWith('abort', expect.any(Function));
  });

  it('aborts the combined signal with the input reason', () => {
    const external = new AbortController();
    const combined = combineSignals(external.signal);
    expect(combined.aborted).toBe(false);

    const reason = new Error('boom');
    external.abort(reason);

    expect(combined.aborted).toBe(true);
    expect(combined.reason).toBe(reason);
  });
});

describe('client surface', () => {
  it('createApiClient returns a ForgeApiClient', () => {
    const client = createApiClient({ baseUrl: 'https://panel.example.com' });
    expect(client).toBeInstanceOf(ForgeApiClient);
  });

  it('ApiError carries status, statusText and data', () => {
    const err = new ApiError(404, 'Not Found', { message: 'nope' });
    expect(err.message).toContain('404');
    expect(err.status).toBe(404);
    expect(err.statusText).toBe('Not Found');
    expect(err.data).toEqual({ message: 'nope' });
  });

  it('listServers unwraps the PaginatedEnvelope data array', async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse({ data: [{ id: 's1', name: 'mc' }], meta: { pagination: { current: 1 } } })
    );
    const client = new ForgeApiClient({ baseUrl: 'https://panel.example.com/api/v1', fetch: fetchMock as unknown as typeof fetch });

    await expect(client.listServers()).resolves.toEqual([{ id: 's1', name: 'mc' }]);
  });
});
