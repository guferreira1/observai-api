// Package dbmigrate runs database schema migrations at API startup.
//
// The runner is opt-in (OBSERVAI_MIGRATE_ON_START) so operators that prefer
// a separate migration step in CI/CD can keep their workflow. When
// enabled, the API blocks startup until migrations reach the latest
// version, which matches the contract expected by managed Kubernetes
// deployments where an init container does not exist.
package dbmigrate

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/golang-migrate/migrate/v4"

	// Drivers are imported for their side effects so migrate can resolve
	// the postgres:// scheme and the file:// source.
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Options configures the runner.
type Options struct {
	DatabaseDSN   string
	MigrationsDir string
	Logger        *slog.Logger
}

// Run applies pending migrations against the supplied database. It is a
// no-op when the database DSN or migrations directory are not provided so
// local mode without a Postgres backend continues to start cleanly.
func Run(opts Options) error {
	dsn := strings.TrimSpace(opts.DatabaseDSN)
	dir := strings.TrimSpace(opts.MigrationsDir)
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if dsn == "" {
		logger.Warn("skipping migrations: database dsn not configured")
		return nil
	}
	if dir == "" {
		logger.Warn("skipping migrations: migrations directory not configured")
		return nil
	}

	source := "file://" + dir
	migrator, err := migrate.New(source, dsn)
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer func() {
		if sourceErr, dbErr := migrator.Close(); sourceErr != nil || dbErr != nil {
			logger.Warn("migrate close returned errors", "sourceErr", sourceErr, "dbErr", dbErr)
		}
	}()

	if err := migrator.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("migrations already up to date")
			return nil
		}
		return fmt.Errorf("apply migrations: %w", err)
	}

	version, dirty, versionErr := migrator.Version()
	if versionErr != nil {
		logger.Warn("migrations applied but version read failed", "error", versionErr)
		return nil
	}
	logger.Info("migrations applied", "version", version, "dirty", dirty)
	return nil
}
