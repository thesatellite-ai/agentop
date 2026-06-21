// Package discover lists OS processes and resolves their working directories.
// macOS-only in v1 (shells out to ps + lsof); Linux (/proc) is future work.
package discover

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Proc is a raw process row from ps, before agent classification.
type Proc struct {
	PID       int
	PPID      int
	RSSBytes  uint64
	TTY       string // controlling terminal, e.g. "ttys003" ("??" = none)
	StartedAt time.Time
	Command   string
}

// lstartLayout matches `ps -o lstart` output, e.g. "Mon Jun 15 22:59:59 2026".
const lstartLayout = "Mon Jan _2 15:04:05 2006"

// ListProcs returns every process on the system in one ps call. Capturing all
// of them (not just agents) lets the classifier walk parent chains without
// extra ps invocations.
func ListProcs() ([]Proc, error) {
	// Field order is fixed; lstart is exactly 5 space-delimited tokens, so we
	// can parse positionally and treat the remainder as the command.
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,rss=,tty=,lstart=,command=").Output()
	if err != nil {
		return nil, err
	}
	var procs []Proc
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	sc.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		p, ok := parseProcLine(line)
		if ok {
			procs = append(procs, p)
		}
	}
	return procs, sc.Err()
}

// parseProcLine parses "PID PPID RSS TTY Dow Mon DD HH:MM:SS YYYY command...".
func parseProcLine(line string) (Proc, bool) {
	fields := strings.Fields(line)
	if len(fields) < 10 {
		return Proc{}, false
	}
	pid, err1 := strconv.Atoi(fields[0])
	ppid, err2 := strconv.Atoi(fields[1])
	rssKB, err3 := strconv.ParseUint(fields[2], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return Proc{}, false
	}
	tty := fields[3]
	// lstart occupies fields[4:9] (5 tokens: Dow Mon DD HH:MM:SS YYYY).
	lstart := strings.Join(fields[4:9], " ")
	started, _ := time.ParseInLocation(lstartLayout, lstart, time.Local)
	// command is everything after the lstart tokens. Recover it from the raw
	// line to preserve internal spacing rather than re-joining fields.
	cmd := commandAfterLstart(line, fields[:9])
	return Proc{
		PID:       pid,
		PPID:      ppid,
		RSSBytes:  rssKB * 1024,
		TTY:       tty,
		StartedAt: started,
		Command:   cmd,
	}, true
}

// commandAfterLstart returns the substring of line following the 8th field.
func commandAfterLstart(line string, head []string) string {
	idx := 0
	for _, f := range head {
		j := strings.Index(line[idx:], f)
		if j < 0 {
			break
		}
		idx += j + len(f)
	}
	return strings.TrimSpace(line[idx:])
}
