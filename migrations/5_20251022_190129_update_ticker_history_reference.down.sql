ALTER TABLE ticker_history
    ADD COLUMN ticker_name TEXT,
    ADD COLUMN secid TEXT,
    ADD COLUMN boardid TEXT;

UPDATE ticker_history th
SET ticker_name = ti.ticker_name,
    secid = ti.secid,
    boardid = ti.boardid
FROM ticker_info ti
WHERE ti.id = th.ticker_info_id;

ALTER TABLE ticker_history
    ALTER COLUMN ticker_name SET NOT NULL,
    ALTER COLUMN secid SET NOT NULL,
    ALTER COLUMN boardid SET NOT NULL;

ALTER TABLE ticker_history
    DROP CONSTRAINT ticker_history_ticker_info_id_fkey;

ALTER TABLE ticker_history
    DROP COLUMN ticker_info_id;
