CREATE OR REPLACE FUNCTION default_allocation_container_port()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.container_port IS NULL THEN
        NEW.container_port := NEW.port;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS allocations_default_container_port ON allocations;
CREATE TRIGGER allocations_default_container_port
BEFORE INSERT OR UPDATE OF port, container_port ON allocations
FOR EACH ROW
EXECUTE FUNCTION default_allocation_container_port();
