ALTER TABLE ticker
    ALTER COLUMN trading_session_active DROP NOT NULL;

ALTER TABLE ticker
    DROP COLUMN IF EXISTS secid;

ALTER TABLE ticker
    RENAME COLUMN ticker_name TO ticker;
