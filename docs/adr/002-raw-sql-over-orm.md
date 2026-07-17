# ADR 002: Use Raw SQL Instead of ORM

**Status:** Accepted

## Context
We needed to decide on a data access strategy for the panel database.

## Decision
We chose raw SQL with pgx over ORMs like GORM or sqlx because:
1. Full control over SQL queries and optimization
2. Better performance (no ORM overhead)
3. Direct access to PostgreSQL-specific features (JSONB, arrays, CTEs)
4. Simpler migration management
5. No impedance mismatch between Go structs and database schema

## Consequences
- Positive: Maximum query performance and flexibility
- Positive: Direct use of PostgreSQL features
- Negative: More boilerplate code for CRUD operations
- Negative: Manual schema changes in migrations
