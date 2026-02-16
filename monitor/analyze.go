package monitor

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/pylemonorg/gotools/db"
	"github.com/pylemonorg/gotools/logger"
)

// AnalyzeFromRedis 从 Redis List 读取资源汇总记录，按 CPU 核心数分组后聚合分析。
// 返回按 CPU 核心数升序排列的分析结果和格式化的报告字符串。
//
// 用法：
//
//	results, report, err := monitor.AnalyzeFromRedis(redisClient, "resource:summary:myapp", nil)
//	fmt.Println(report)
func AnalyzeFromRedis(redisClient *db.RedisClient, key string, opts *AnalyzeOptions) ([]AnalyzeResult, string, error) {
	values, err := redisClient.LRange(key, 0, -1)
	if err != nil {
		return nil, "", fmt.Errorf("monitor: LRANGE [%s] 失败: %w", key, err)
	}

	logger.Infof("monitor: 从 Redis key [%s] 读取到 %d 条记录", key, len(values))

	if len(values) == 0 {
		return nil, "无记录", nil
	}

	records, parseErrors := parseRecords(values, opts)
	if parseErrors > 0 {
		logger.Warnf("monitor: 解析 %d 条记录失败", parseErrors)
	}

	if len(records) == 0 {
		return nil, "过滤后无有效记录", nil
	}

	grouped := groupByCPU(records)
	results := analyzeGroups(grouped)
	report := formatReport(results)

	return results, report, nil
}

// AnalyzeRecords 对给定的 SummaryRecord 切片进行聚合分析，不依赖 Redis。
// 适合已经从其他渠道获取到记录的场景。
func AnalyzeRecords(records []SummaryRecord, opts *AnalyzeOptions) ([]AnalyzeResult, string) {
	if len(records) == 0 {
		return nil, "无记录"
	}

	filtered := filterRecords(records, opts)
	if len(filtered) == 0 {
		return nil, "过滤后无有效记录"
	}

	grouped := groupByCPU(filtered)
	results := analyzeGroups(grouped)
	report := formatReport(results)

	return results, report
}

// ---------------------------------------------------------------------------
// 内部实现
// ---------------------------------------------------------------------------

// parseRecords 解析 JSON 字符串列表并按时间过滤。
func parseRecords(values []string, opts *AnalyzeOptions) ([]SummaryRecord, int) {
	var records []SummaryRecord
	var parseErrors int

	for _, v := range values {
		var r SummaryRecord
		if err := json.Unmarshal([]byte(v), &r); err != nil {
			parseErrors++
			continue
		}
		records = append(records, r)
	}

	return filterRecords(records, opts), parseErrors
}

// filterRecords 按 Since 时间过滤记录。
func filterRecords(records []SummaryRecord, opts *AnalyzeOptions) []SummaryRecord {
	if opts == nil || opts.Since.IsZero() {
		return records
	}

	var filtered []SummaryRecord
	for _, r := range records {
		t, err := time.Parse(time.RFC3339, r.EndedAt)
		if err != nil {
			logger.Warnf("monitor: 解析记录时间失败: %s, 错误: %v", r.EndedAt, err)
			continue
		}
		if t.After(opts.Since) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// groupByCPU 按 CPU 核心数分组。
func groupByCPU(records []SummaryRecord) map[int][]SummaryRecord {
	grouped := make(map[int][]SummaryRecord)
	for _, r := range records {
		grouped[r.NumCPU] = append(grouped[r.NumCPU], r)
	}
	return grouped
}

// analyzeGroups 对分组后的记录进行聚合计算，返回按 CPU 核心数升序排列的结果。
func analyzeGroups(grouped map[int][]SummaryRecord) []AnalyzeResult {
	var cpuCounts []int
	for k := range grouped {
		cpuCounts = append(cpuCounts, k)
	}
	sort.Ints(cpuCounts)

	results := make([]AnalyzeResult, 0, len(cpuCounts))
	for _, cpu := range cpuCounts {
		results = append(results, analyzeOneGroup(cpu, grouped[cpu]))
	}
	return results
}

// analyzeOneGroup 对单个 CPU 分组进行加权聚合。
func analyzeOneGroup(cpu int, records []SummaryRecord) AnalyzeResult {
	r := AnalyzeResult{
		NumCPU:       cpu,
		RecordCount:  len(records),
		CPUMin:       records[0].CPUMin,
		MemoryMin:    records[0].MemoryMin,
		GoroutineMin: records[0].GoroutineMin,
	}

	var weightedCPU, weightedMem, weightedGor float64

	for _, rec := range records {
		samples := float64(rec.SampleCount)
		r.TotalSamples += rec.SampleCount

		if rec.CPUMin < r.CPUMin {
			r.CPUMin = rec.CPUMin
		}
		if rec.CPUMax > r.CPUMax {
			r.CPUMax = rec.CPUMax
		}
		weightedCPU += rec.CPUAvg * samples

		if rec.MemoryMin < r.MemoryMin {
			r.MemoryMin = rec.MemoryMin
		}
		if rec.MemoryMax > r.MemoryMax {
			r.MemoryMax = rec.MemoryMax
		}
		weightedMem += float64(rec.MemoryAvg) * samples

		if rec.GoroutineMin < r.GoroutineMin {
			r.GoroutineMin = rec.GoroutineMin
		}
		if rec.GoroutineMax > r.GoroutineMax {
			r.GoroutineMax = rec.GoroutineMax
		}
		weightedGor += float64(rec.GoroutineAvg) * samples
	}

	if r.TotalSamples > 0 {
		total := float64(r.TotalSamples)
		r.CPUAvg = weightedCPU / total
		r.MemoryAvg = uint64(weightedMem / total)
		r.GoroutineAvg = int(weightedGor / total)
	}

	return r
}
