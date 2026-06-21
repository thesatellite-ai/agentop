package discover

import "testing"

func TestParseProcLine(t *testing.T) {
	// Real-shape line: pid ppid rss tty <5-token lstart> command (with spaces).
	line := "66216 66147 520512 ttys004 Mon Jun 15 22:59:59 2026 /Users/x/.local/bin/claude --resume abc --flag"
	p, ok := parseProcLine(line)
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	if p.PID != 66216 || p.PPID != 66147 {
		t.Errorf("pid/ppid = %d/%d", p.PID, p.PPID)
	}
	if p.RSSBytes != 520512*1024 {
		t.Errorf("rss = %d, want %d", p.RSSBytes, 520512*1024)
	}
	if p.TTY != "ttys004" {
		t.Errorf("tty = %q", p.TTY)
	}
	// command must keep its internal spacing and all args.
	want := "/Users/x/.local/bin/claude --resume abc --flag"
	if p.Command != want {
		t.Errorf("command = %q, want %q", p.Command, want)
	}
	if p.StartedAt.IsZero() {
		t.Error("StartedAt should parse from lstart")
	}
	if y := p.StartedAt.Year(); y != 2026 {
		t.Errorf("StartedAt year = %d, want 2026", y)
	}
}

func TestParseProcLineNoTTY(t *testing.T) {
	// processes with no controlling terminal show "??" for tty.
	line := "100 1 2048 ?? Mon Jun 15 09:00:00 2026 /usr/sbin/somed"
	p, ok := parseProcLine(line)
	if !ok {
		t.Fatal("expected success")
	}
	if p.TTY != "??" {
		t.Errorf("tty = %q, want ??", p.TTY)
	}
	if p.Command != "/usr/sbin/somed" {
		t.Errorf("command = %q", p.Command)
	}
}

func TestParseProcLineRejects(t *testing.T) {
	bad := []string{
		"",                       // empty
		"only three fields here", // too few
		"x y z ttys0 Mon Jun 15 22:59:59 2026 cmd", // non-numeric pid
	}
	for _, line := range bad {
		if _, ok := parseProcLine(line); ok {
			t.Errorf("expected reject for %q", line)
		}
	}
}
