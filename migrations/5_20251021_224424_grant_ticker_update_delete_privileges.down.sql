DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_roles WHERE rolname = 'invest_intraday_app'
    ) THEN
        RAISE NOTICE 'Role % does not exist; skipping revokes.', 'invest_intraday_app';
    ELSE
        EXECUTE 'REVOKE UPDATE (trading_session_date, trading_session_active, ticker_name, secid, boardid, vwap, val, vah, liquidity, volatility, flat_trend_filter) ON TABLE ticker FROM invest_intraday_app';
        EXECUTE 'REVOKE UPDATE, DELETE ON TABLE ticker FROM invest_intraday_app';
    END IF;
END;
$$;
