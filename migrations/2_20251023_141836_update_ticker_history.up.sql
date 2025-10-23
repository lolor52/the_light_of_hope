ALTER TABLE ticker_history
    DROP COLUMN IF EXISTS liquidity,
    DROP COLUMN IF EXISTS volatility,
    DROP COLUMN IF EXISTS flat_trend_filter,
    ADD COLUMN IF NOT EXISTS swing_count_paired TEXT;
