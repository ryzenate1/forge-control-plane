-- 138_consolidate_legacy_batch2.sql deliberately has no destructive down step.
-- It records legacy Batch 2 schema that may already be shared with older
-- deployments and later canonical migrations. Dropping those objects would
-- corrupt data or undo schema owned by another migration. Rollback therefore
-- removes only its ledger entry; restore schema changes from a database backup
-- when a full reversal is required.
SELECT 1;
