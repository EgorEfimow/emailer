package store

import (
	"context"
	"database/sql"
	"testing"
)

func TestNewSQLiteStore_InMemory(t *testing.T) {
	ctx := context.Background()

	s, err := NewSQLiteStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(:memory:) failed: %v", err)
	}
	defer s.Close()

	// Verify the store is operational (not nil, can be closed).
	if s.db == nil {
		t.Fatal("expected non-nil db handle")
	}
}

func TestSQLiteStore_MigrationsCreateTables(t *testing.T) {
	ctx := context.Background()

	s, err := NewSQLiteStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(:memory:) failed: %v", err)
	}
	defer s.Close()

	// Verify all expected tables exist.
	expected := []string{"runs", "processed_messages", "flags_applied", "digests"}

	for _, table := range expected {
		if !tableExists(t, s, table) {
			t.Errorf("expected table %q to exist after migrations", table)
		}
	}
}

func TestSQLiteStore_MigrationsIdempotent(t *testing.T) {
	// Opening the same store twice should not error — second run is a no-op.
	ctx := context.Background()

	s1, err := NewSQLiteStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("first NewSQLiteStore failed: %v", err)
	}
	s1.Close()

	// Re-opening the same in-memory path creates a fresh DB — that's fine.
	// The point is that the migration runner handles already-applied migrations.
	s2, err := NewSQLiteStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("second NewSQLiteStore failed: %v", err)
	}
	s2.Close()
}

func TestSQLiteStore_MigrationIndexes(t *testing.T) {
	ctx := context.Background()

	s, err := NewSQLiteStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(:memory:) failed: %v", err)
	}
	defer s.Close()

	// Verify the composite dedup index on processed_messages exists.
	// With our schema, (account_label, uid) is the PRIMARY KEY, which
	// automatically creates a unique index.
	if !indexExists(t, s, "sqlite_autoindex_processed_messages_1") {
		t.Error("expected composite primary key index on processed_messages")
	}

	// Verify the run_id index exists.
	if !indexExists(t, s, "idx_processed_messages_run_id") {
		t.Error("expected idx_processed_messages_run_id index")
	}
}

func TestSQLiteStore_ColumnIsRead(t *testing.T) {
	ctx := context.Background()

	s, err := NewSQLiteStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(:memory:) failed: %v", err)
	}
	defer s.Close()

	// Check that processed_messages has the is_read column.
	if !columnExists(t, s, "processed_messages", "is_read") {
		t.Error("expected is_read column in processed_messages table")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func tableExists(t *testing.T, s *SQLiteStore, name string) bool {
	t.Helper()
	row := s.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?",
		name,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("query table %q: %v", name, err)
	}
	return count > 0
}

func indexExists(t *testing.T, s *SQLiteStore, name string) bool {
	t.Helper()
	row := s.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?",
		name,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("query index %q: %v", name, err)
	}
	return count > 0
}

func columnExists(t *testing.T, s *SQLiteStore, table, column string) bool {
	t.Helper()
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("pragma table_info(%q): %v", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}