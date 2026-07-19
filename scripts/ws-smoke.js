const http = require("http");

function requestJSON(method, url, body, headers = {}) {
  return new Promise((resolve, reject) => {
    const parsed = new URL(url);
    const req = http.request(
      {
        method,
        hostname: parsed.hostname,
        port: parsed.port,
        path: parsed.pathname + parsed.search,
        headers: {
          Accept: "application/json",
          ...headers,
        },
      },
      (res) => {
        let data = "";
        res.on("data", (chunk) => (data += chunk));
        res.on("end", () => {
          if (res.statusCode < 200 || res.statusCode >= 300) {
            return reject(new Error(`HTTP ${res.statusCode}: ${data}`));
          }
          try {
            resolve(JSON.parse(data));
          } catch (err) {
            reject(err);
          }
        });
      }
    );
    req.on("error", reject);
    if (body !== undefined) {
      req.setHeader("Content-Type", "application/json");
      req.write(JSON.stringify(body));
    }
    req.end();
  });
}

async function login() {
  const payload = await requestJSON("POST", "http://127.0.0.1:8080/api/v1/auth/login", {
    email: "admin@example.com",
    password: "admin123",
  });
  return payload.token;
}

function onceWebSocketMessage(url, timeoutMs = 5000) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(url);
    const timer = setTimeout(() => {
      try {
        ws.close();
      } catch {}
      reject(new Error("timeout"));
    }, timeoutMs);
    ws.onmessage = (event) => {
      clearTimeout(timer);
      const data = typeof event.data === "string" ? event.data : String(event.data);
      try {
        ws.close();
      } catch {}
      resolve(data);
    };
    ws.onerror = () => {
      clearTimeout(timer);
      reject(new Error("ws error"));
    };
  });
}

async function main() {
  const token = await login();
  const servers = await requestJSON(
    "GET",
    "http://127.0.0.1:8080/api/v1/servers",
    undefined,
    { Authorization: `Bearer ${token}` }
  );
  const list = Array.isArray(servers) ? servers : servers?.value;
  const preferred = list?.find?.((server) => server?.id === "44444444-4444-4444-4444-444444444444");
  const serverId = preferred?.id ?? list?.[0]?.id;
  if (!serverId) {
    throw new Error("no servers returned");
  }

  const statsUrl = `ws://127.0.0.1:8080/api/v1/servers/${encodeURIComponent(serverId)}/ws/stats?token=${encodeURIComponent(token)}`;
  const logsUrl = `ws://127.0.0.1:8080/api/v1/servers/${encodeURIComponent(serverId)}/ws/logs?token=${encodeURIComponent(token)}`;
  const consoleUrl = `ws://127.0.0.1:8080/api/v1/servers/${encodeURIComponent(serverId)}/ws/console?token=${encodeURIComponent(token)}`;

  const stats = await onceWebSocketMessage(statsUrl);
  const logs = await onceWebSocketMessage(logsUrl);
  const consoleFrame = await onceWebSocketMessage(consoleUrl);

  process.stdout.write(
    JSON.stringify(
      {
        serverId,
        stats: stats.slice(0, 200),
        logs: logs.slice(0, 200),
        console: consoleFrame.slice(0, 200),
      },
      null,
      2
    ) + "\n"
  );
}

main().catch((err) => {
  process.stderr.write(String(err?.stack || err) + "\n");
  process.exit(1);
});
