package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite" // uses database/sql with any sqlite driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// SQLiteStore implements the Store interface backed by a SQLite database.
// It is safe for concurrent use.
type SQLiteStore struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteStore opens or creates a SQLite database at the given path and
// runs all pending migrations. The path can be ":memory:" for testing.
func NewSQLiteStore(ctx context.Context, path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store.NewSQLiteStore: open db: %w", err)
	}

	// Configure modernc.org/sqlite for our workload.
	db.SetMaxOpenConns(1) // SQLite is single-writer; serialise access.
	db.SetMaxIdleConns(1)

	// Verify the connection is alive.
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.NewSQLiteStore: ping db: %w", err)
	}

	// Run migrations.
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.NewSQLiteStore: migrations: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close releases the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// runMigrations applies all pending SQL migration files to the database.
func runMigrations(db *sql.DB) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	defer src.Close()

	inst, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("migration instance: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "sqlite", inst)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}

	// NOTE: we do NOT call m.Close() here because it would close the
	// *sql.DB that the caller needs to keep using. The migrate instance
	// will be garbage-collected.
	return nil
}

// ---------------------------------------------------------------------------
// Store interface — stub implementations return "not implemented" errors.
// Full implementations are added in later steps (4.9–4.16).
// ---------------------------------------------------------------------------

func (s *SQLiteStore) RecordRun(_ context.Context, _ Run) (Run, error) {
	return Run{}, errNotImplemented("RecordRun")
}

func (s *SQLiteStore) FinishRun(_ context.Context, _ string, _ RunStatus, _ int, _ error) error {
	return errNotImplemented("FinishRun")
}

func (s *SQLiteStore) GetRun(_ context.Context, _ string) (Run, error) {
	return Run{}, errNotImplemented("GetRun")
}

func (s *SQLiteStore) ListRuns(_ context.Context, _ int) ([]Run, error) {
	return nil, errNotImplemented("ListRuns")
}

func (s *SQLiteStore) GetLastSuccessfulRunTime(_ context.Context) (*time.Time, error) {
	return nil, errNotImplemented("GetLastSuccessfulRunTime")
}

func (s *SQLiteStore) RecordMessage(_ context.Context, _ ProcessedMessage) error {
	return errNotImplemented("RecordMessage")
}

func (s *SQLiteStore) AlreadyProcessed(_ context.Context, _ []MessageKey) (map[MessageKey]bool, error) {
	return nil, errNotImplemented("AlreadyProcessed")
}

func (s *SQLiteStore) RecordFlag(_ context.Context, _ FlagRecord) error {
	return errNotImplemented("RecordFlag")
}

func (s *SQLiteStore) RecordDigest(_ context.Context, _ DigestRecord) error {
	return errNotImplemented("RecordDigest")
}

// errNotImplemented returns a sentinel error for methods that are not yet
// implemented. Callers can use errors.Is to detect these stubs.
func errNotImplemented(method string) error {
	return fmt.Errorf("store.%s: %w", method, ErrNotImplemented)
}

// ErrNotImplemented is returned by stub methods of SQLiteStore.
var ErrNotImplemented = errors.New("not implemented")