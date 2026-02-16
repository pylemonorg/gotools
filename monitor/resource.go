package monitor

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/pylemonorg/gotools/logger"
	"github.com/shirou/gopsutil/v3/process"
)

// ResourceMonitor 进程资源监控器，定时采样 CPU / 内存 / Goroutine 等指标。
type ResourceMonitor struct {
	proc        *process.Process
	interval    time.Duration
	logInterval time.Duration
	lastLogTime time.Time
	stopChan    chan struct{}
	wg          sync.WaitGroup
	running     bool
	mu          sync.Mutex
	numCPU      int

	onStats func(stats *ResourceStats)

	saverMu sync.Mutex
	saver   SummarySaver
	saveKey string

	historyMu sync.Mutex
	history   []ResourceStats
}

// NewResourceMonitor 创建资源监控器。cfg 可为 nil，使用默认配置。
func NewResourceMonitor(cfg *Config) (*ResourceMonitor, error) {
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		return nil, fmt.Errorf("monitor: 获取进程信息失败: %w", err)
	}

	interval := 2 * time.Second
	logInterval := 2 * time.Second
	var onStats func(stats *ResourceStats)
	var saver SummarySaver
	var saveKey string

	if cfg != nil {
		if cfg.Interval > 0 {
			interval = cfg.Interval
			logInterval = cfg.Interval
		}
		if cfg.LogInterval > 0 {
			logInterval = cfg.LogInterval
		}
		onStats = cfg.OnStats
		saver = cfg.Saver
		saveKey = cfg.SaveKey
	}

	return &ResourceMonitor{
		proc:        p,
		interval:    interval,
		logInterval: logInterval,
		stopChan:    make(chan struct{}),
		onStats:     onStats,
		saver:       saver,
		saveKey:     saveKey,
		numCPU:      runtime.NumCPU(),
		history:     make([]ResourceStats, 0, 1000),
	}, nil
}

// SetSaver 设置或更新汇总持久化方式（可在 Start 之后调用）。
func (m *ResourceMonitor) SetSaver(saver SummarySaver, key string) {
	m.saverMu.Lock()
	defer m.saverMu.Unlock()
	m.saver = saver
	m.saveKey = key
}

// Start 启动异步监控。每次启动会清空历史数据，确保汇总只包含本次运行的采样。
func (m *ResourceMonitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	m.historyMu.Lock()
	m.history = m.history[:0]
	m.historyMu.Unlock()

	m.wg.Add(1)
	go m.loop()
	logger.Infof("monitor: 资源监控已启动（间隔: %v, CPU 核心数: %d）", m.interval, m.numCPU)
}

// Stop 停止监控并输出汇总。
func (m *ResourceMonitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.stopChan)
	m.mu.Unlock()

	m.wg.Wait()

	m.logAndSaveSummary()
	logger.Infof("monitor: 资源监控已停止")

	m.mu.Lock()
	m.stopChan = make(chan struct{})
	m.mu.Unlock()
}

// GetStats 同步获取当前资源快照。
func (m *ResourceMonitor) GetStats() (*ResourceStats, error) {
	stats := &ResourceStats{
		Timestamp:     time.Now(),
		NumGoroutines: runtime.NumGoroutine(),
	}

	if cpu, err := m.proc.CPUPercent(); err == nil {
		stats.CPUPercent = cpu
	} else {
		logger.Debugf("monitor: 获取 CPU 使用率失败: %v", err)
	}
	if mem, err := m.proc.MemoryInfo(); err == nil {
		stats.MemoryRSS = mem.RSS
		stats.MemoryVMS = mem.VMS
	} else {
		logger.Debugf("monitor: 获取内存信息失败: %v", err)
	}
	if pct, err := m.proc.MemoryPercent(); err == nil {
		stats.MemoryPercent = pct
	} else {
		logger.Debugf("monitor: 获取内存百分比失败: %v", err)
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	stats.NumGC = ms.NumGC
	stats.HeapAlloc = ms.HeapAlloc
	stats.HeapSys = ms.HeapSys

	return stats, nil
}

// GetSummary 获取当前已采集数据的汇总。无数据时返回 nil。
func (m *ResourceMonitor) GetSummary() *ResourceSummary {
	m.historyMu.Lock()
	defer m.historyMu.Unlock()

	n := len(m.history)
	if n == 0 {
		return nil
	}

	summary := &ResourceSummary{
		SampleCount:  n,
		CPUMin:       m.history[0].CPUPercent,
		CPUMax:       m.history[0].CPUPercent,
		MemoryMin:    m.history[0].MemoryRSS,
		MemoryMax:    m.history[0].MemoryRSS,
		GoroutineMin: m.history[0].NumGoroutines,
		GoroutineMax: m.history[0].NumGoroutines,
	}

	var cpuSum float64
	var memSum uint64
	var grSum int

	for _, s := range m.history {
		if s.CPUPercent < summary.CPUMin {
			summary.CPUMin = s.CPUPercent
		}
		if s.CPUPercent > summary.CPUMax {
			summary.CPUMax = s.CPUPercent
		}
		cpuSum += s.CPUPercent

		if s.MemoryRSS < summary.MemoryMin {
			summary.MemoryMin = s.MemoryRSS
		}
		if s.MemoryRSS > summary.MemoryMax {
			summary.MemoryMax = s.MemoryRSS
		}
		memSum += s.MemoryRSS

		if s.NumGoroutines < summary.GoroutineMin {
			summary.GoroutineMin = s.NumGoroutines
		}
		if s.NumGoroutines > summary.GoroutineMax {
			summary.GoroutineMax = s.NumGoroutines
		}
		grSum += s.NumGoroutines
	}

	summary.CPUAvg = cpuSum / float64(n)
	summary.MemoryAvg = memSum / uint64(n)
	summary.GoroutineAvg = grSum / n

	return summary
}

// ---------------------------------------------------------------------------
// 内部方法
// ---------------------------------------------------------------------------

// loop 监控主循环。
func (m *ResourceMonitor) loop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats, err := m.GetStats()
			if err != nil {
				logger.Debugf("monitor: 获取资源统计失败: %v", err)
				continue
			}

			m.historyMu.Lock()
			const maxHistory = 500000
			const trimCount = 50000
			if len(m.history) >= maxHistory {
				n := copy(m.history, m.history[trimCount:])
				m.history = m.history[:n]
			}
			m.history = append(m.history, *stats)
			m.historyMu.Unlock()

			if m.onStats != nil {
				m.onStats(stats)
			} else {
				now := time.Now()
				if now.Sub(m.lastLogTime) >= m.logInterval {
					m.logStats(stats)
					m.lastLogTime = now
				}
			}

		case <-m.stopChan:
			return
		}
	}
}

// logStats 输出单次采样日志。
func (m *ResourceMonitor) logStats(stats *ResourceStats) {
	coresUsed := stats.CPUPercent / 100.0
	logger.Infof("monitor: CPU=%.1f%% (%.1f/%d核), 内存=%s(%.1f%%), Goroutines=%d, GC=%d",
		stats.CPUPercent, coresUsed, m.numCPU,
		FormatBytes(stats.MemoryRSS), stats.MemoryPercent,
		stats.NumGoroutines, stats.NumGC)
}

// logAndSaveSummary 输出汇总日志，并在设置了 Saver 时持久化。
func (m *ResourceMonitor) logAndSaveSummary() {
	summary := m.GetSummary()
	if summary == nil {
		logger.Infof("monitor: 汇总 - 无采样数据")
		return
	}

	logger.Infof("monitor: ========== 资源使用汇总 ==========")
	logger.Infof("monitor: 采样次数: %d", summary.SampleCount)
	logger.Infof("monitor: CPU (总核心: %d) - 最小: %.1f%%, 最大: %.1f%%, 平均: %.1f%%",
		m.numCPU, summary.CPUMin, summary.CPUMax, summary.CPUAvg)
	logger.Infof("monitor: 内存 - 最小: %s, 最大: %s, 平均: %s",
		FormatBytes(summary.MemoryMin), FormatBytes(summary.MemoryMax), FormatBytes(summary.MemoryAvg))
	logger.Infof("monitor: Goroutines - 最小: %d, 最大: %d, 平均: %d",
		summary.GoroutineMin, summary.GoroutineMax, summary.GoroutineAvg)
	logger.Infof("monitor: ====================================")

	// 持久化
	m.saverMu.Lock()
	saver, key := m.saver, m.saveKey
	m.saverMu.Unlock()

	if saver == nil || key == "" {
		return
	}

	record := SummaryRecord{
		NumCPU:          m.numCPU,
		EndedAt:         time.Now().Format(time.RFC3339),
		ResourceSummary: *summary,
	}
	jsonBytes, err := json.Marshal(record)
	if err != nil {
		logger.Warnf("monitor: 汇总 JSON 序列化失败: %v", err)
		return
	}
	if err = saver.SaveSummary(key, string(jsonBytes)); err != nil {
		logger.Warnf("monitor: 汇总保存失败: %v", err)
		return
	}
	logger.Infof("monitor: 汇总已保存到 [%s]", key)
}
