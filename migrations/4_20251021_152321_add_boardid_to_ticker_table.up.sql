ALTER TABLE ticker
    ADD COLUMN boardid TEXT DEFAULT '';

UPDATE ticker
SET boardid = ''
WHERE boardid IS NULL;

ALTER TABLE ticker
    ALTER COLUMN boardid SET NOT NULL,
    ALTER COLUMN boardid DROP DEFAULT;
