DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_roles WHERE rolname = 'invest_intraday_app'
    ) THEN
        RAISE NOTICE 'Role % already exists; skipping creation.', 'invest_intraday_app';
    ELSE
        EXECUTE 'CREATE ROLE invest_intraday_app LOGIN';
        RAISE NOTICE 'Role % created without password; set a password manually if login is required.', 'invest_intraday_app';
    END IF;
END;
$$;
