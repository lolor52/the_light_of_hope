package migrate

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

func TestEnsureDownMigrations(t *testing.T) {
	dir := t.TempDir()

	mustWrite(t, filepath.Join(dir, "1_20240101_010101_init.up.sql"))
	mustWrite(t, filepath.Join(dir, "1_20240101_010101_init.down.sql"))
	mustWrite(t, filepath.Join(dir, "legacy.sql"))

	if err := ensureDownMigrations(dir); err != nil {
		t.Fatalf("ensureDownMigrations returned error: %v", err)
	}

	mustWrite(t, filepath.Join(dir, "2_20240202_020202_missing.up.sql"))

	if err := ensureDownMigrations(dir); err == nil {
		t.Fatal("expected error for missing down migration, got nil")
	}
}

func TestMigrationFilenamesFollowConvention(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "migrations")

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("migrations directory %q does not exist", dir)
		}
		t.Fatalf("failed to read migrations directory: %v", err)
	}

	pattern := regexp.MustCompile(`^(?P<version>\d+)_(?P<timestamp>\d{8}_\d{6})_(?P<name>[A-Za-z0-9_]+)\.(?P<kind>up|down)\.sql$`)
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Fatalf("failed to load Europe/Moscow timezone: %v", err)
	}

	type suffixCount struct {
		up   int
		down int
	}

	files := map[string]*suffixCount{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".sql" {
			continue
		}
		if !pattern.MatchString(name) {
			t.Fatalf("migration %q does not follow naming convention", name)
		}

		matches := pattern.FindStringSubmatch(name)
		timestamp := matches[pattern.SubexpIndex("timestamp")]
		if _, err := time.ParseInLocation("20060102_150405", timestamp, loc); err != nil {
			t.Fatalf("migration %q has invalid timestamp: %v", name, err)
		}

		base := matches[pattern.SubexpIndex("version")] + "_" + timestamp + "_" + matches[pattern.SubexpIndex("name")]
		count := files[base]
		if count == nil {
			count = &suffixCount{}
			files[base] = count
		}

		kind := matches[pattern.SubexpIndex("kind")]
		switch kind {
		case "up":
			count.up++
			if count.up > 1 {
				t.Fatalf("duplicate up migration detected for %q", base)
			}
		case "down":
			count.down++
			if count.down > 1 {
				t.Fatalf("duplicate down migration detected for %q", base)
			}
		}
	}
}

func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("-- test"), 0o600); err != nil {
		t.Fatalf("failed to write file %q: %v", path, err)
	}
}
