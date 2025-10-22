CREATE TABLE IF NOT EXISTS ticker_info (
    id BIGSERIAL PRIMARY KEY,
    ticker_name TEXT NOT NULL,
    secid TEXT NOT NULL,
    boardid TEXT NOT NULL
);
