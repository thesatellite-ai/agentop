//go:build darwin

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
	at := time.Unix(st.Atimespec.Sec, st.Atimespec.Nsec)
	mt := time.Unix(st.Mtimespec.Sec, st.Mtimespec.Nsec)
	if at.After(mt) {
		return at
	}
	return mt
}
