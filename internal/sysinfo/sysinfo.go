package sysinfo

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// Stats holds a snapshot of CPU, memory, and disk utilisation.
type Stats struct {
	CPUPercent int
	MemTotalKB uint64
	MemAvailKB uint64
	DiskTotal  uint64 // bytes
	DiskFree   uint64 // bytes
}

// Sampler continuously measures CPU usage in the background (requires two
// /proc/stat reads to compute a delta) and reads memory/disk on demand.
type Sampler struct {
	disk      string
	stats     atomic.Pointer[Stats]
	prevIdle  uint64
	prevTotal uint64
}

func NewSampler(disk string) *Sampler {
	if disk == "" {
		disk = "/"
	}
	s := &Sampler{disk: disk}
	s.prevIdle, s.prevTotal = readCPUStat()
	s.stats.Store(&Stats{})
	return s
}

func (s *Sampler) Start() {
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s.sample()
		}
	}()
}

func (s *Sampler) Current() Stats {
	if p := s.stats.Load(); p != nil {
		return *p
	}
	return Stats{}
}

func (s *Sampler) sample() {
	idle, total := readCPUStat()

	diffIdle := idle - s.prevIdle
	diffTotal := total - s.prevTotal
	s.prevIdle = idle
	s.prevTotal = total

	var cpuPct int
	if diffTotal > 0 {
		cpuPct = int((diffTotal-diffIdle)*100 / diffTotal)
	}

	memTotal, memAvail := readMemInfo()
	diskTotal, diskFree := readDiskInfo(s.disk)

	s.stats.Store(&Stats{
		CPUPercent: cpuPct,
		MemTotalKB: memTotal,
		MemAvailKB: memAvail,
		DiskTotal:  diskTotal,
		DiskFree:   diskFree,
	})
}

// readCPUStat returns (idle, total) jiffies from the first "cpu" line in
// /proc/stat. idle includes iowait so the result matches what top shows.
func readCPUStat() (idle, total uint64) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		// fields[0]="cpu", then: user nice system idle iowait irq softirq steal …
		for i, v := range fields[1:] {
			n, _ := strconv.ParseUint(v, 10, 64)
			total += n
			if i == 3 || i == 4 { // idle + iowait
				idle += n
			}
		}
		return
	}
	return
}

func readMemInfo() (totalKB, availKB uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			totalKB = parseMemLine(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			availKB = parseMemLine(line)
		}
		if totalKB > 0 && availKB > 0 {
			return
		}
	}
	return
}

func parseMemLine(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	n, _ := strconv.ParseUint(fields[1], 10, 64)
	return n
}

func readDiskInfo(path string) (total, free uint64) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return
	}
	bsize := uint64(st.Bsize)
	return st.Blocks * bsize, st.Bavail * bsize
}
