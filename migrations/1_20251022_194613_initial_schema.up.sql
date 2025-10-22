CREATE TABLE IF NOT EXISTS ticker_info (
    id BIGSERIAL PRIMARY KEY,
    ticker_name TEXT NOT NULL,
    secid TEXT NOT NULL,
    boardid TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS ticker_history (
    id BIGSERIAL PRIMARY KEY,
    trading_session_date DATE NOT NULL,
    trading_session_active BOOLEAN NOT NULL,
    ticker_info_id BIGINT NOT NULL REFERENCES ticker_info(id),
    vwap TEXT,
    val TEXT,
    vah TEXT,
    liquidity TEXT,
    volatility TEXT,
    flat_trend_filter TEXT
);

CREATE TABLE IF NOT EXISTS order_price (
    id BIGSERIAL PRIMARY KEY,
    ticker_info_id BIGINT NOT NULL REFERENCES ticker_info(id),
    price_long NUMERIC(12,2) NOT NULL,
    price_short NUMERIC(12,2) NOT NULL,
    vwap TEXT NOT NULL,
    val TEXT NOT NULL,
    vah TEXT NOT NULL,
    date_time TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT timezone('Europe/Moscow', now())
);
