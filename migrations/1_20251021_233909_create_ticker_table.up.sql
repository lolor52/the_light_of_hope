CREATE TABLE IF NOT EXISTS ticker (
    trading_session_date DATE NOT NULL,
    trading_session_active BOOLEAN NOT NULL,
    ticker_name TEXT NOT NULL,
    vwap TEXT,
    val TEXT,
    vah TEXT,
    liquidity TEXT,
    volatility TEXT,
    flat_trend_filter TEXT,
    secid TEXT NOT NULL,
    boardid TEXT NOT NULL
);
