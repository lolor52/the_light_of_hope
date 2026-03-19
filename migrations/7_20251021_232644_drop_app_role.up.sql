DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_roles WHERE rolname = 'invest_intraday_app'
    ) THEN
        EXECUTE 'DROP ROLE invest_intraday_app';
    ELSE
        RAISE NOTICE 'Role % does not exist; nothing to drop.', 'invest_intraday_app';
    END IF;
END;
$$;
