ALTER TABLE ticker
    DROP COLUMN IF EXISTS liquidity,
    DROP COLUMN IF EXISTS volatility,
    DROP COLUMN IF EXISTS flat_trend_filter;

ALTER TABLE ticker RENAME TO ticker_vwap_va;
