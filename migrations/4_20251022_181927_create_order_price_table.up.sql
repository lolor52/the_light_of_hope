CREATE TABLE IF NOT EXISTS order_price (
    id BIGSERIAL PRIMARY KEY,
    tickers_id BIGINT NOT NULL REFERENCES tickers(id),
    price_long NUMERIC(12,2) NOT NULL,
    price_short NUMERIC(12,2) NOT NULL,
    vwap TEXT NOT NULL,
    val TEXT NOT NULL,
    vah TEXT NOT NULL,
    date_time TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT timezone('Europe/Moscow', now())
);
