package monitor

import (
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// 采样数据
// ---------------------------------------------------------------------------

// ResourceStats 单次资源采样数据。
type ResourceStats struct {
	CPUPercent    float64   // CPU 使用率（百分比，多核场景可能 >100%）
	MemoryRSS     uint64   // 常驻内存（字节）
	MemoryVMS     uint64   // 虚拟内存（字节）
	MemoryPercent float32  // 内存使用率（百分比）
	NumGoroutines int      // Goroutine 数量
	NumGC         uint32   // GC 累计次数
	HeapAlloc     uint64   // 堆已分配内存（字节）
	HeapSys       uint64   // 堆系统内存（字节）
	Timestamp     time.Time // 采样时间
}

// FormatStats 将采样数据格式化为一行摘要字符串。
func (s *ResourceStats) FormatStats() string {
	return fmt.Sprintf("CPU=%.1f%%, 内存=%s(%.1f%%), Goroutines=%d, GC=%d",
		s.CPUPercent, FormatBytes(s.MemoryRSS), s.MemoryPercent, s.NumGoroutines, s.NumGC)
}

// ---------------------------------------------------------------------------
// 汇总数据
// ---------------------------------------------------------------------------

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

// SummaryRecord 持久化到 Redis 的 JSON 结构，包含 CPU 核心数、记录时间和资源汇总。
type SummaryRecord struct {
	NumCPU  int    `json:"num_cpu"`
	EndedAt string `json:"ended_at"`
	ResourceSummary
}

// ---------------------------------------------------------------------------
// 持久化接口
// ---------------------------------------------------------------------------

// SummarySaver 资源汇总持久化接口。
// 由调用方实现，Stop 时自动调用。不设置则不持久化。
type SummarySaver interface {
	SaveSummary(key string, jsonValue string) error
}

// ---------------------------------------------------------------------------
// 配置
// ---------------------------------------------------------------------------

// Config 监控器配置。
type Config struct {
	Interval    time.Duration              // 采样间隔，默认 2s
	LogInterval time.Duration              // 日志输出间隔，默认等于 Interval
	OnStats     func(stats *ResourceStats) // 采样回调（设置后不再输出默认日志）
	Saver       SummarySaver               // 汇总持久化实现（Stop 时保存），可为 nil
	SaveKey     string                     // 持久化的 Redis key
}

// ---------------------------------------------------------------------------
// 分析相关
// ---------------------------------------------------------------------------

// AnalyzeOptions 资源分析选项。
type AnalyzeOptions struct {
	Since time.Time // 仅分析此时间之后的记录，零值表示不过滤
}

// AnalyzeResult 单个 CPU 分组的聚合分析结果。
type AnalyzeResult struct {
	NumCPU       int     // CPU 核心数
	RecordCount  int     // 记录条数
	TotalSamples int     // 总采样次数
	CPUMin       float64 // CPU 使用率最小值
	CPUMax       float64 // CPU 使用率最大值
	CPUAvg       float64 // CPU 加权平均值
	MemoryMin    uint64  // 内存最小值（字节）
	MemoryMax    uint64  // 内存最大值（字节）
	MemoryAvg    uint64  // 内存加权平均值（字节）
	GoroutineMin int     // Goroutine 最小数量
	GoroutineMax int     // Goroutine 最大数量
	GoroutineAvg int     // Goroutine 加权平均数量
}
