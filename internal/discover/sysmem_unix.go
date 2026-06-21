//go:build darwin || linux

package discover

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// PhysicalMemory returns total system RAM in bytes, or 0 if it can't be
// determined. darwin via sysctl hw.memsize; linux via /proc/meminfo.
func PhysicalMemory() uint64 {
	if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
		if v, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64); err == nil && v > 0 {
			return v
		}
	}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
						return kb * 1024 // value is in kB
					}
				}
			}
		}
	}
	return 0
}
