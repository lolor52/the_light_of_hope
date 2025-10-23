ALTER TABLE ticker_history
    ADD COLUMN IF NOT EXISTS liquidity TEXT,
    ADD COLUMN IF NOT EXISTS volatility TEXT,
    ADD COLUMN IF NOT EXISTS flat_trend_filter TEXT,
    DROP COLUMN IF EXISTS swing_count_paired;
