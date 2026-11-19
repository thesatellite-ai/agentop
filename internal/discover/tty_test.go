package discover

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTTYActivity(t *testing.T) {
	t.Parallel()
	if got := TTYActivity(""); !got.IsZero() {
		t.Fatalf("no tty = %v, want zero", got)
	}
	if got := TTYActivity("no-such-tty-alice"); !got.IsZero() {
		t.Fatalf("missing device = %v, want zero", got)
	}

	// ttyActivity takes the newer of atime and mtime of the device file.
	f := filepath.Join(t.TempDir(), "ttys-bob")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	mt := at.Add(-time.Hour)
	if err := os.Chtimes(f, at, mt); err != nil {
		t.Fatal(err)
	}
	if got := ttyActivity(f); !got.Equal(at) {
		t.Fatalf("atime newer: %v, want %v", got, at)
	}
	if err := os.Chtimes(f, mt, at); err != nil {
		t.Fatal(err)
	}
	if got := ttyActivity(f); !got.Equal(at) {
		t.Fatalf("mtime newer: %v, want %v", got, at)
	}
}

func TestPhysicalMemory(t *testing.T) {
	t.Parallel()
	if PhysicalMemory() == 0 {
		t.Skip("physical memory not readable on this host")
	}
}
