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

// ResourceStats 单次资源采样数据。
type ResourceStats struct {
	CPUPercent    float64   // CPU 使用率（百分比，多核场景可能 >100%）
	MemoryRSS     uint64    // 常驻内存（字节）
	MemoryVMS     uint64    // 虚拟内存（字节）
	MemoryPercent float32   // 内存使用率（百分比）
	NumGoroutines int       // Goroutine 数量
	NumGC         uint32    // GC 累计次数
	HeapAlloc     uint64    // 堆已分配内存（字节）
	HeapSys       uint64    // 堆系统内存（字节）
	Timestamp     time.Time // 采样时间
}

// FormatStats 将采样数据格式化为一行摘要字符串。
func (s *ResourceStats) FormatStats() string {
	return fmt.Sprintf("CPU=%.1f%%, 内存=%s(%.1f%%), Goroutines=%d, GC=%d",
		s.CPUPercent, FormatBytes(s.MemoryRSS), s.MemoryPercent, s.NumGoroutines, s.NumGC)
}

// ResourceSummary 一段时间内的资源使用汇总。
type ResourceSummary struct {
	SampleCount  int     `json:"sample_count"`
	CPUMin       float64 `json:"cpu_min"`
	CPUMax       float64 `json:"cpu_max"`
	CPUAvg       float64 `json:"cpu_avg"`
	MemoryMin    uint64  `json:"memory_min"`
	MemoryMax    uint64  `json:"memory_max"`
	MemoryAvg    uint64  `json:"memory_avg"`
	GoroutineMin int     `json:"goroutine_min"`
	GoroutineMax int     `json:"goroutine_max"`
	GoroutineAvg int     `json:"goroutine_avg"`
}

// SummarySaver 资源汇总持久化接口（可选）。
// 由调用方实现（如保存到 Redis List），不设置则不持久化。
type SummarySaver interface {
	SaveSummary(key string, jsonValue string) error
}

// Config 监控器配置。
type Config struct {
	Interval        time.Duration                 // 采样间隔，默认 2s
	LogInterval     time.Duration                 // 日志输出间隔，默认等于 Interval
	OnStats         func(stats *ResourceStats)    // 采样回调（设置后不再输出默认日志）
	GetSummarySaver func() (SummarySaver, string) // 返回 (saver, key)，停止时保存汇总
}

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

	saverMu         sync.Mutex
	getSummarySaver func() (SummarySaver, string)

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
	var getSummarySaver func() (SummarySaver, string)

	if cfg != nil {
		if cfg.Interval > 0 {
			interval = cfg.Interval
			logInterval = cfg.Interval
		}
		if cfg.LogInterval > 0 {
			logInterval = cfg.LogInterval
		}
		onStats = cfg.OnStats
		getSummarySaver = cfg.GetSummarySaver
	}

	return &ResourceMonitor{
		proc:            p,
		interval:        interval,
		logInterval:     logInterval,
		stopChan:        make(chan struct{}),
		onStats:         onStats,
		getSummarySaver: getSummarySaver,
		numCPU:          runtime.NumCPU(),
		history:         make([]ResourceStats, 0, 1000),
	}, nil
}

// SetSummarySaver 设置汇总持久化回调（可在启动后再设置）。
func (m *ResourceMonitor) SetSummarySaver(getter func() (SummarySaver, string)) {
	if getter == nil {
		return
	}
	m.saverMu.Lock()
	defer m.saverMu.Unlock()
	m.getSummarySaver = getter
}

// SaveToRedis 便捷方法，设置将汇总保存到指定 key。
func (m *ResourceMonitor) SaveToRedis(saver SummarySaver, key string) {
	m.SetSummarySaver(func() (SummarySaver, string) {
		return saver, key
	})
}

// Start 启动异步监控。
func (m *ResourceMonitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

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
	m.mu.Unlock()

	close(m.stopChan)
	m.wg.Wait()

	m.printSummary()
	logger.Infof("monitor: 资源监控已停止")

	// 重置 stopChan 以允许再次 Start
	m.stopChan = make(chan struct{})
}

// GetStats 同步获取当前资源快照。
func (m *ResourceMonitor) GetStats() (*ResourceStats, error) {
	stats := &ResourceStats{
		Timestamp:     time.Now(),
		NumGoroutines: runtime.NumGoroutine(),
	}

	if cpu, err := m.proc.CPUPercent(); err == nil {
		stats.CPUPercent = cpu
	}
	if mem, err := m.proc.MemoryInfo(); err == nil {
		stats.MemoryRSS = mem.RSS
		stats.MemoryVMS = mem.VMS
	}
	if pct, err := m.proc.MemoryPercent(); err == nil {
		stats.MemoryPercent = pct
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	stats.NumGC = ms.NumGC
	stats.HeapAlloc = ms.HeapAlloc
	stats.HeapSys = ms.HeapSys

	return stats, nil
}

// GetSummary 获取当前已采集数据的汇总。
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

			// 记录历史（上限 500000 条，超出时丢弃最早的 50000 条）
			m.historyMu.Lock()
			const maxHistory = 500000
			if len(m.history) >= maxHistory {
				m.history = m.history[50000:]
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

// resourceSummaryRecord 持久化时的 JSON 结构。
type resourceSummaryRecord struct {
	NumCPU  int    `json:"num_cpu"`
	EndedAt string `json:"ended_at"`
	*ResourceSummary
}

// printSummary 输出汇总日志，并可选持久化。
func (m *ResourceMonitor) printSummary() {
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

	// 可选持久化
	m.saverMu.Lock()
	getter := m.getSummarySaver
	m.saverMu.Unlock()

	if getter == nil {
		return
	}
	saver, key := getter()
	if saver == nil || key == "" {
		return
	}

	record := resourceSummaryRecord{
		NumCPU:          m.numCPU,
		EndedAt:         time.Now().Format(time.RFC3339),
		ResourceSummary: summary,
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

// ---------------------------------------------------------------------------
// 工具函数
// ---------------------------------------------------------------------------

// FormatBytes 将字节数格式化为人类可读的字符串（B / KB / MB / GB）。
func FormatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
