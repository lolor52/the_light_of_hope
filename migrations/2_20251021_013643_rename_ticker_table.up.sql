ALTER TABLE ticker_vwap_va
    RENAME TO ticker;

ALTER TABLE ticker
    ADD COLUMN liquidity TEXT,
    ADD COLUMN volatility TEXT,
    ADD COLUMN flat_trend_filter TEXT;
