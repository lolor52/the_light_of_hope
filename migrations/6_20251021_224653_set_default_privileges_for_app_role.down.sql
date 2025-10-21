DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_roles WHERE rolname = 'invest_intraday_app'
    ) THEN
        RAISE NOTICE 'Role % does not exist; skipping default privilege revokes.', 'invest_intraday_app';
    ELSE
        EXECUTE 'ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM invest_intraday_app';
    END IF;
END;
$$;
