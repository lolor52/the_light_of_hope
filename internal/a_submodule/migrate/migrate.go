package migrate

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
)

// Up применяет доступные миграции. Если миграции уже применены, возвращает nil.
func Up(db *sql.DB, migrationsDir string) (err error) {
	m, cleanup, err := newMigrator(db, migrationsDir)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, cleanup())
	}()

	if err = m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			return nil
		}
		return err
	}

	return nil
}

// Force устанавливает версию миграции вручную. Используется для очистки dirty-состояний.
func Force(db *sql.DB, migrationsDir string, version int) (err error) {
	if version < 0 {
		return fmt.Errorf("force version must be non-negative")
	}

	m, cleanup, err := newMigrator(db, migrationsDir)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, cleanup())
	}()

	return m.Force(version)
}

func newMigrator(db *sql.DB, migrationsDir string) (*migrate.Migrate, func() error, error) {
	if db == nil {
		return nil, nil, fmt.Errorf("db is nil")
	}
	if strings.TrimSpace(migrationsDir) == "" {
		return nil, nil, fmt.Errorf("migrations directory is required")
	}

	absDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve migrations directory: %w", err)
	}

	info, err := os.Stat(absDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("migrations directory %q does not exist", absDir)
		}
		return nil, nil, fmt.Errorf("failed to stat migrations directory: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("migrations path %q is not a directory", absDir)
	}

	if err := ensureDownMigrations(absDir); err != nil {
		return nil, nil, err
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create postgres driver: %w", err)
	}

	sourceURL := fmt.Sprintf("file://%s", absDir)

	m, err := migrate.NewWithDatabaseInstance(sourceURL, "postgres", driver)
	if err != nil {
		cleanupErr := driver.Close()
		return nil, nil, errors.Join(fmt.Errorf("failed to create migrator: %w", err), cleanupErr)
	}

	cleanup := func() error {
		sourceErr, dbErr := m.Close()
		return errors.Join(sourceErr, dbErr)
	}

	return m, cleanup, nil
}

func ensureDownMigrations(dir string) error {
	upEntries, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return fmt.Errorf("failed to list up migrations: %w", err)
	}

	downFiles := map[string]struct{}{}
	downEntries, err := filepath.Glob(filepath.Join(dir, "*.down.sql"))
	if err != nil {
		return fmt.Errorf("failed to list down migrations: %w", err)
	}
	for _, down := range downEntries {
		base := strings.TrimSuffix(filepath.Base(down), ".down.sql")
		downFiles[base] = struct{}{}
	}

	for _, up := range upEntries {
		base := strings.TrimSuffix(filepath.Base(up), ".up.sql")
		if _, ok := downFiles[base]; !ok {
			return fmt.Errorf("missing down migration for %s", filepath.Base(up))
		}
	}

	return nil
}
