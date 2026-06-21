// Package emdash reads emdash's local SQLite DB to map a worktree path to its
// task name, status, and last-interaction time.
//
// The DB is emdash-owned (emdash4.db). We treat it as strictly read-only and
// query defensively: select only needed columns, tolerate a missing/locked DB,
// and pick the newest emdash*.db so a schema-generation bump (emdash5.db) keeps
// working.
package emdash

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite"
)

// Info is the enrichment for one worktree path.
type Info struct {
	Task       string
	Status     string
	LastActive time.Time
}

// lastActiveLayout matches emdash's stored timestamps, e.g. "2026-06-19 06:33:16".
const lastActiveLayout = "2006-01-02 15:04:05"

// DBPath returns the newest emdash*.db under the macOS app-support dir, or "".
func DBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, "Library", "Application Support", "emdash")
	matches, _ := filepath.Glob(filepath.Join(dir, "emdash*.db"))
	// keep only the base db files (skip -wal / -shm, which Glob won't match
	// against "*.db" anyway), newest schema-generation last.
	var dbs []string
	for _, m := range matches {
		dbs = append(dbs, m)
	}
	if len(dbs) == 0 {
		return ""
	}
	sort.Strings(dbs) // emdash4.db < emdash5.db lexically for single digits
	return dbs[len(dbs)-1]
}

// LoadMap opens the newest emdash DB read-only and returns a map of worktree
// path -> Info. A missing DB or any query error yields an empty map (graceful
// degradation: sessions just show "(no task)").
func LoadMap() (map[string]Info, error) {
	path := DBPath()
	if path == "" {
		return map[string]Info{}, nil
	}
	return loadFrom(path)
}

// loadFrom reads a specific emdash DB file. Split out from LoadMap so it can be
// tested against a fixture DB without depending on the host's emdash install.
func loadFrom(path string) (map[string]Info, error) {
	// Read-only + immutable avoids touching the WAL while emdash holds it.
	dsn := "file:" + path + "?mode=ro&immutable=1&_pragma=busy_timeout(2000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return map[string]Info{}, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT w.path,
		       COALESCE(t.name, ''),
		       COALESCE(t.status, ''),
		       COALESCE(t.last_interacted_at, '')
		FROM workspaces w
		LEFT JOIN tasks t ON t.workspace_id = w.id
		WHERE w.path IS NOT NULL AND w.path <> ''`)
	if err != nil {
		return map[string]Info{}, err
	}
	defer rows.Close()

	out := make(map[string]Info)
	for rows.Next() {
		var path, name, status, last string
		if err := rows.Scan(&path, &name, &status, &last); err != nil {
			continue
		}
		info := Info{Task: name, Status: status}
		if last != "" {
			if t, err := time.ParseInLocation(lastActiveLayout, last, time.Local); err == nil {
				info.LastActive = t
			}
		}
		// A path may map to multiple rows (e.g. project-root + worktree). Prefer
		// the row that actually has a task name.
		if existing, ok := out[path]; ok && existing.Task != "" && info.Task == "" {
			continue
		}
		out[path] = info
	}
	return out, rows.Err()
}
