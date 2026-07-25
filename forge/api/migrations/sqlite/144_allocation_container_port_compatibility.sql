-- SQLite dialect override for 144_allocation_container_port_compatibility.sql
--
-- Encodes the PostgreSQL BEFORE trigger (default container_port := port) in
-- SQLite trigger syntax. The plpgsql function form has no SQLite spelling, so
-- the function is replaced, not skipped: skipping would silently store
-- container_port IS NULL rows that PostgreSQL would have backfilled.
PRAGMA recursive_triggers=OFF;

-- Backfill rows that predate the trigger (the PG trigger only fires on
-- INSERT/UPDATE, so this matches production semantics for old rows that the
-- application later touches... kept conservative: only NULL rows).
UPDATE allocations SET container_port = port WHERE container_port IS NULL;

DROP TRIGGER IF EXISTS allocations_default_container_port;
CREATE TRIGGER allocations_default_container_port
AFTER INSERT ON allocations
FOR EACH ROW WHEN (NEW.container_port IS NULL)
BEGIN
    UPDATE allocations SET container_port = NEW.port WHERE id = NEW.id;
END;

DROP TRIGGER IF EXISTS allocations_default_container_port_upd;
CREATE TRIGGER allocations_default_container_port_upd
AFTER UPDATE OF port, container_port ON allocations
FOR EACH ROW WHEN (NEW.container_port IS NULL)
BEGIN
    UPDATE allocations SET container_port = NEW.port WHERE id = NEW.id;
END;
