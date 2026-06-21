//go:build linux

package discover

import (
	"os"
	"syscall"
	"time"
)

// ttyActivity returns max(atime, mtime) of the device file, or zero on error.
func ttyActivity(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fi.ModTime()
	}
	at := time.Unix(st.Atim.Sec, st.Atim.Nsec)
	mt := time.Unix(st.Mtim.Sec, st.Mtim.Nsec)
	if at.After(mt) {
		return at
	}
	return mt
}
