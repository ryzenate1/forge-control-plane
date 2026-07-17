# ADR 003: Use HMAC-SHA256 Tokens Instead of Standard JWT

**Status:** Accepted

## Context
We needed an authentication token format for user sessions and API authentication.

## Decision
We chose custom HMAC-SHA256 signed tokens over standard JWT because:
1. Simpler implementation (no JWT library dependency)
2. Smaller token size (no standard JWT header overhead)
3. Sufficient security for internal authentication
4. Easy to implement custom token types (2FA confirmation tokens)

## Consequences
- Positive: Smaller tokens, simpler code
- Positive: No JWT library dependency
- Negative: Not standard JWT (incompatible with some third-party tools)
- Negative: Manual implementation of token parsing/validation
