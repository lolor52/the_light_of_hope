ALTER TABLE ticker_history
    ADD COLUMN ticker_info_id BIGINT;

UPDATE ticker_history th
SET ticker_info_id = ti.id
FROM ticker_info ti
WHERE ti.ticker_name = th.ticker_name
  AND ti.secid = th.secid
  AND ti.boardid = th.boardid;

ALTER TABLE ticker_history
    ALTER COLUMN ticker_info_id SET NOT NULL;

ALTER TABLE ticker_history
    ADD CONSTRAINT ticker_history_ticker_info_id_fkey
    FOREIGN KEY (ticker_info_id) REFERENCES ticker_info(id);

ALTER TABLE ticker_history
    DROP COLUMN ticker_name,
    DROP COLUMN secid,
    DROP COLUMN boardid;
