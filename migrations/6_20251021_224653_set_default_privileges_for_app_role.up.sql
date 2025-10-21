DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_roles WHERE rolname = 'invest_intraday_app'
    ) THEN
        RAISE NOTICE 'Role % does not exist; skipping default privilege grants.', 'invest_intraday_app';
    ELSE
        EXECUTE $$ALTER DEFAULT PRIVILEGES IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO invest_intraday_app$$;
    END IF;
END;
$$;
