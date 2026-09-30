-- Event history is operationally durable. Reversal is performed from backup,
-- not by deleting the outbox or dead-letter history.
SELECT 1;
