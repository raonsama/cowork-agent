// Package thermal monitors device temperature and CPU load via sysfs and
// /proc/stat, throttling the agent when thresholds are exceeded.
package thermal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Status represents the current thermal state.
type Status struct {
	TempCelsius float64
	CPUPercent  float64
	Throttled   bool
	ThrottleMsg string
}

// Monitor watches device temperature and CPU load,
// broadcasting status updates and pause signals.
type Monitor struct {
	tempThreshold float64 // °C
	cpuThreshold  float64 // %
	pollInterval  time.Duration

	paused   atomic.Bool
	status   Status
	mu       sync.RWMutex
	statusCh chan Status
	started  atomic.Bool
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewMonitor creates a thermal monitor with the given thresholds.
func NewMonitor(tempThreshold, cpuThreshold float64) *Monitor {
	return &Monitor{
		tempThreshold: tempThreshold,
		cpuThreshold:  cpuThreshold,
		pollInterval:  3 * time.Second,
		statusCh:      make(chan Status, 8),
		stopCh:        make(chan struct{}),
	}
}

// Start begins background monitoring. Call Stop() to halt.
func (m *Monitor) Start() {
	if m.started.Swap(true) {
		return
	}
	go m.loop()
}

// Stop halts the monitor.
func (m *Monitor) Stop() {
	m.stopOnce.Do(func() { close(m.stopCh) })
}

// StatusCh returns the channel on which Status updates are broadcast.
func (m *Monitor) StatusCh() <-chan Status {
	return m.statusCh
}

// IsThrottled returns true if the device is currently overheating or overloaded.
func (m *Monitor) IsThrottled() bool {
	return m.paused.Load()
}

// Current returns the latest thermal snapshot.
func (m *Monitor) Current() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

// WaitIfThrottled blocks the caller until the device cools down.
// Pass a channel to abort waiting early.
func (m *Monitor) WaitIfThrottled(abort <-chan struct{}) {
	for m.IsThrottled() {
		select {
		case <-abort:
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (m *Monitor) loop() {
	var prevIdle, prevTotal uint64
	first := true

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
		}

		temp := m.readTemp()
		idle, total := readCPUStat()

		var cpuPercent float64
		if !first && total > prevTotal {
			cpuPercent = 100.0 * float64(total-idle-(prevTotal-prevIdle)) / float64(total-prevTotal)
		}
		prevIdle = idle
		prevTotal = total
		first = false

		throttled := false
		var msg string
		if temp >= m.tempThreshold {
			throttled = true
			msg = fmt.Sprintf("🌡 Overheating %.1f°C (threshold %.1f°C) — throttling", temp, m.tempThreshold)
		} else if cpuPercent >= m.cpuThreshold {
			throttled = true
			msg = fmt.Sprintf("🔥 CPU %.0f%% (threshold %.0f%%) — throttling", cpuPercent, m.cpuThreshold)
		}

		s := Status{
			TempCelsius: temp,
			CPUPercent:  cpuPercent,
			Throttled:   throttled,
			ThrottleMsg: msg,
		}

		m.paused.Store(throttled)
		m.mu.Lock()
		m.status = s
		m.mu.Unlock()

		select {
		case m.statusCh <- s:
		default:
		}
	}
}

// readTemp reads the highest temperature from the thermal zone sysfs
// (works on Android/Termux and Linux).
func (m *Monitor) readTemp() float64 {
	zones, err := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	if err != nil || len(zones) == 0 {
		return 0
	}
	var maxTemp float64
	for _, zone := range zones {
		data, err := os.ReadFile(zone)
		if err != nil {
			continue
		}
		raw := strings.TrimSpace(string(data))
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue
		}
		// Android reports millidegrees
		if v > 1000 {
			v /= 1000.0
		}
		if v > maxTemp {
			maxTemp = v
		}
	}
	return maxTemp
}

// readCPUStat reads /proc/stat and returns (idle, total) jiffies.
func readCPUStat() (idle, total uint64) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		// cpu user nice system idle iowait irq softirq steal guest guest_nice
		if len(fields) < 5 {
			break
		}
		for i, field := range fields[1:] {
			v, _ := strconv.ParseUint(field, 10, 64)
			total += v
			if i == 3 { // idle
				idle = v
			}
		}
		break
	}
	return idle, total
}
