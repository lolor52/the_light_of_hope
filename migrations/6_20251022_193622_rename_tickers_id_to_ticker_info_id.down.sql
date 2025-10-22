ALTER TABLE order_price
    DROP CONSTRAINT IF EXISTS order_price_ticker_info_id_fkey;

ALTER TABLE order_price
    RENAME COLUMN ticker_info_id TO tickers_id;

ALTER TABLE order_price
    ADD CONSTRAINT order_price_tickers_id_fkey
        FOREIGN KEY (tickers_id) REFERENCES tickers(id);
