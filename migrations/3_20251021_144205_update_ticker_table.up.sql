ALTER TABLE ticker
    RENAME COLUMN ticker TO ticker_name;

ALTER TABLE ticker
    ADD COLUMN secid TEXT DEFAULT '';

UPDATE ticker
SET secid = ''
WHERE secid IS NULL;

UPDATE ticker
SET trading_session_active = FALSE
WHERE trading_session_active IS NULL;

ALTER TABLE ticker
    ALTER COLUMN secid SET NOT NULL,
    ALTER COLUMN secid DROP DEFAULT,
    ALTER COLUMN ticker_name SET NOT NULL,
    ALTER COLUMN trading_session_active SET NOT NULL,
    ALTER COLUMN trading_session_date SET NOT NULL;
