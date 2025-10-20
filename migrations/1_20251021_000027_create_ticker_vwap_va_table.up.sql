CREATE TABLE IF NOT EXISTS ticker_vwap_va (
    trading_session_date DATE NOT NULL,
    trading_session_active BOOLEAN,
    ticker TEXT NOT NULL,
    vwap TEXT,
    val TEXT,
    vah TEXT
);
