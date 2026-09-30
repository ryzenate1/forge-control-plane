DO $$
DECLARE
    dependency RECORD;
BEGIN
    FOR dependency IN
        SELECT rel.relname AS table_name, c.conname AS constraint_name
        FROM pg_constraint c
        JOIN pg_class rel ON rel.oid = c.conrelid
        JOIN pg_class ref ON ref.oid = c.confrelid
        JOIN pg_namespace rel_ns ON rel_ns.oid = rel.relnamespace
        JOIN pg_namespace ref_ns ON ref_ns.oid = ref.relnamespace
        WHERE ref.relname = 'compose_stacks'
          AND rel_ns.nspname = current_schema()
          AND ref_ns.nspname = current_schema()
          AND c.contype = 'f'
    LOOP
        EXECUTE format(
            'ALTER TABLE %I DROP CONSTRAINT %I',
            dependency.table_name,
            dependency.constraint_name
        );
    END LOOP;
END $$;

ALTER TABLE compose_stacks
    ALTER COLUMN id DROP DEFAULT,
    ALTER COLUMN id TYPE TEXT USING id::text;

ALTER TABLE compose_services
    ALTER COLUMN stack_id TYPE TEXT USING stack_id::text;

ALTER TABLE compose_logs
    ALTER COLUMN stack_id TYPE TEXT USING stack_id::text;

ALTER TABLE compose_services
    ADD CONSTRAINT compose_services_stack_id_fkey
    FOREIGN KEY (stack_id) REFERENCES compose_stacks(id) ON DELETE CASCADE;

ALTER TABLE compose_logs
    ADD CONSTRAINT compose_logs_stack_id_fkey
    FOREIGN KEY (stack_id) REFERENCES compose_stacks(id) ON DELETE CASCADE;
