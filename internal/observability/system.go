package observability

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SystemStats is a lightweight host snapshot for the dashboard.
type SystemStats struct {
	CPUPercent  float64   `json:"cpu_percent"`
	MemTotalMB  uint64    `json:"mem_total_mb"`
	MemUsedMB   uint64    `json:"mem_used_mb"`
	MemFreeMB   uint64    `json:"mem_free_mb"`
	Load1       float64   `json:"load_1"`
	Load5       float64   `json:"load_5"`
	Load15      float64   `json:"load_15"`
	Goroutines  int       `json:"goroutines"`
	HeapMB      float64   `json:"heap_mb"`
	SysMB       float64   `json:"sys_mb"`
	GCPercent   int       `json:"gc_percent"`
	UptimeSec   int64     `json:"uptime_sec"`
	NumCPU      int       `json:"num_cpu"`
	GoVersion   string    `json:"go_version"`
	CollectedAt time.Time `json:"collected_at"`
}

// SystemMonitor samples /proc to estimate CPU and memory without cgo.
type SystemMonitor struct {
	mu        sync.Mutex
	prevIdle  uint64
	prevTotal uint64
	start     time.Time
}

// NewSystemMonitor builds a monitor.
func NewSystemMonitor() *SystemMonitor { return &SystemMonitor{start: time.Now()} }

// Snapshot returns current host and process statistics.
func (m *SystemMonitor) Snapshot() SystemStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	var st SystemStats
	st.CollectedAt = time.Now().UTC()
	st.Goroutines = runtime.NumGoroutine()
	st.NumCPU = runtime.NumCPU()
	st.GoVersion = runtime.Version()
	st.UptimeSec = int64(time.Since(m.start).Seconds())
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	st.HeapMB = float64(ms.HeapAlloc) / (1 << 20)
	st.SysMB = float64(ms.Sys) / (1 << 20)
	st.GCPercent = int(ms.NumGC)

	idle, total := readCPU()
	if m.prevTotal > 0 && total > m.prevTotal {
		deltaTotal := float64(total - m.prevTotal)
		deltaIdle := float64(idle - m.prevIdle)
		st.CPUPercent = (1 - deltaIdle/deltaTotal) * 100
	}
	m.prevIdle, m.prevTotal = idle, total

	memTotal, memAvail := readMemInfo()
	if memTotal > 0 {
		st.MemTotalMB = memTotal / 1024
		st.MemFreeMB = memAvail / 1024
		if memTotal > memAvail {
			st.MemUsedMB = (memTotal - memAvail) / 1024
		}
	}
	st.Load1, st.Load5, st.Load15 = readLoad()
	return st
}

func readCPU() (idle, total uint64) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return 0, 0
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0
	}
	for i, field := range fields[1:] {
		v, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			continue
		}
		total += v
		if i == 3 || i == 4 { // idle + iowait
			idle += v
		}
	}
	return idle, total
}

func readMemInfo() (totalKB, availKB uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB = v
		case "MemAvailable:":
			availKB = v
		}
	}
	return totalKB, availKB
}

func readLoad() (l1, l5, l15 float64) {
	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 3 {
		return 0, 0, 0
	}
	l1, _ = strconv.ParseFloat(fields[0], 64)
	l5, _ = strconv.ParseFloat(fields[1], 64)
	l15, _ = strconv.ParseFloat(fields[2], 64)
	return l1, l5, l15
}
