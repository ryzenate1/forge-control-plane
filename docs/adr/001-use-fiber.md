# ADR 001: Use Fiber as HTTP Framework

**Status:** Accepted

## Context
We needed a Go HTTP framework for the API that balances performance, developer experience, and ecosystem compatibility.

## Decision
We chose Fiber v2 because:
1. Express.js-like API (familiar to Node.js developers)
2. High performance (fasthttp-based)
3. Built-in middleware (CORS, CSRF, rate limiting)
4. Active community and maintenance
5. WebSocket support via contrib package

## Consequences
- Positive: Fast request handling, familiar API patterns
- Positive: Good middleware ecosystem
- Negative: Not fully compatible with standard net/http middleware
- Negative: Smaller ecosystem compared to Chi or Gorilla Mux
