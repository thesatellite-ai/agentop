package emdash

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// makeFixtureDB writes a minimal emdash-shaped DB (the columns our query reads)
// and returns its path.
func makeFixtureDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "emdash4.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE workspaces (id TEXT PRIMARY KEY, path TEXT)`,
		`CREATE TABLE tasks (name TEXT, status TEXT, workspace_id TEXT, last_interacted_at TEXT)`,
		`INSERT INTO workspaces (id, path) VALUES ('w1', '/code/proj-a'), ('w2', '/code/proj-b')`,
		// proj-a has a task; proj-b has none (LEFT JOIN should still list it, empty).
		`INSERT INTO tasks (name, status, workspace_id, last_interacted_at)
		   VALUES ('default-a', 'in_progress', 'w1', '2026-06-19 06:33:16')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec %q: %v", s, err)
		}
	}
	return path
}

func TestLoadFrom(t *testing.T) {
	m, err := loadFrom(makeFixtureDB(t))
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}

	a, ok := m["/code/proj-a"]
	if !ok {
		t.Fatal("proj-a missing from map")
	}
	if a.Task != "default-a" || a.Status != "in_progress" {
		t.Errorf("proj-a = %+v", a)
	}
	if a.LastActive.IsZero() || a.LastActive.Year() != 2026 {
		t.Errorf("proj-a LastActive not parsed: %v", a.LastActive)
	}

	b, ok := m["/code/proj-b"]
	if !ok {
		t.Fatal("proj-b (taskless) should still appear")
	}
	if b.Task != "" || !b.LastActive.IsZero() {
		t.Errorf("proj-b should have empty task/time, got %+v", b)
	}
}

func TestLoadFromMissing(t *testing.T) {
	// A non-existent DB path should error rather than panic; callers degrade.
	if _, err := loadFrom(filepath.Join(t.TempDir(), "nope.db")); err == nil {
		t.Error("expected an error for a missing DB file")
	}
}
