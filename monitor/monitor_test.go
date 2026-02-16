package monitor

import (
	"encoding/json"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// FormatBytes
// ---------------------------------------------------------------------------

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    uint64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1073741824, "1.00 GB"},
		{2684354560, "2.50 GB"},
	}

	for _, tt := range tests {
		result := FormatBytes(tt.input)
		if result != tt.expected {
			t.Errorf("FormatBytes(%d) = %q, 期望 %q", tt.input, result, tt.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// padRightCJK / cjkWidth
// ---------------------------------------------------------------------------

func TestCJKWidth(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"hello", 5},
		{"你好", 4},
		{"CPU使用率", 9},
		{"", 0},
		{"abc你好def", 10},
	}

	for _, tt := range tests {
		result := cjkWidth(tt.input)
		if result != tt.expected {
			t.Errorf("cjkWidth(%q) = %d, 期望 %d", tt.input, result, tt.expected)
		}
	}
}

func TestPadRightCJK(t *testing.T) {
	result := padRightCJK("你好", 10)
	if cjkWidth(result) != 10 {
		t.Errorf("padRightCJK 后宽度应为 10, 实际 %d", cjkWidth(result))
	}

	result = padRightCJK("hello", 5)
	if result != "hello" {
		t.Errorf("不需要填充时应原样返回, 实际 %q", result)
	}
}

// ---------------------------------------------------------------------------
// analyzeOneGroup
// ---------------------------------------------------------------------------

func TestAnalyzeOneGroup(t *testing.T) {
	records := []SummaryRecord{
		{
			NumCPU:  4,
			EndedAt: "2026-02-16T10:00:00+08:00",
			ResourceSummary: ResourceSummary{
				SampleCount:  100,
				CPUMin:       10.0,
				CPUMax:       80.0,
				CPUAvg:       40.0,
				MemoryMin:    1000,
				MemoryMax:    5000,
				MemoryAvg:    3000,
				GoroutineMin: 5,
				GoroutineMax: 50,
				GoroutineAvg: 20,
			},
		},
		{
			NumCPU:  4,
			EndedAt: "2026-02-16T11:00:00+08:00",
			ResourceSummary: ResourceSummary{
				SampleCount:  200,
				CPUMin:       5.0,
				CPUMax:       90.0,
				CPUAvg:       50.0,
				MemoryMin:    800,
				MemoryMax:    6000,
				MemoryAvg:    4000,
				GoroutineMin: 3,
				GoroutineMax: 60,
				GoroutineAvg: 30,
			},
		},
	}

	result := analyzeOneGroup(4, records)

	if result.NumCPU != 4 {
		t.Errorf("NumCPU = %d, 期望 4", result.NumCPU)
	}
	if result.RecordCount != 2 {
		t.Errorf("RecordCount = %d, 期望 2", result.RecordCount)
	}
	if result.TotalSamples != 300 {
		t.Errorf("TotalSamples = %d, 期望 300", result.TotalSamples)
	}
	if result.CPUMin != 5.0 {
		t.Errorf("CPUMin = %.1f, 期望 5.0", result.CPUMin)
	}
	if result.CPUMax != 90.0 {
		t.Errorf("CPUMax = %.1f, 期望 90.0", result.CPUMax)
	}

	// 加权平均: (40*100 + 50*200) / 300 = 14000/300 ≈ 46.67
	expectedAvg := (40.0*100 + 50.0*200) / 300
	if result.CPUAvg < expectedAvg-0.01 || result.CPUAvg > expectedAvg+0.01 {
		t.Errorf("CPUAvg = %.2f, 期望 %.2f", result.CPUAvg, expectedAvg)
	}

	if result.MemoryMin != 800 {
		t.Errorf("MemoryMin = %d, 期望 800", result.MemoryMin)
	}
	if result.MemoryMax != 6000 {
		t.Errorf("MemoryMax = %d, 期望 6000", result.MemoryMax)
	}
	if result.GoroutineMin != 3 {
		t.Errorf("GoroutineMin = %d, 期望 3", result.GoroutineMin)
	}
	if result.GoroutineMax != 60 {
		t.Errorf("GoroutineMax = %d, 期望 60", result.GoroutineMax)
	}
}

func TestAnalyzeOneGroupZeroSamples(t *testing.T) {
	records := []SummaryRecord{
		{ResourceSummary: ResourceSummary{SampleCount: 0}},
	}
	result := analyzeOneGroup(4, records)
	if result.CPUAvg != 0 {
		t.Errorf("TotalSamples=0 时 CPUAvg 应为 0, 实际 %.2f", result.CPUAvg)
	}
}

// ---------------------------------------------------------------------------
// filterRecords
// ---------------------------------------------------------------------------

func TestFilterRecords(t *testing.T) {
	records := []SummaryRecord{
		{EndedAt: "2026-02-15T10:00:00+08:00", ResourceSummary: ResourceSummary{SampleCount: 10}},
		{EndedAt: "2026-02-16T10:00:00+08:00", ResourceSummary: ResourceSummary{SampleCount: 20}},
		{EndedAt: "2026-02-17T10:00:00+08:00", ResourceSummary: ResourceSummary{SampleCount: 30}},
	}

	// 无过滤
	result := filterRecords(records, nil)
	if len(result) != 3 {
		t.Errorf("无过滤条件时应返回全部 3 条, 实际 %d", len(result))
	}

	// 有过滤
	since, _ := time.Parse(time.RFC3339, "2026-02-16T00:00:00+08:00")
	result = filterRecords(records, &AnalyzeOptions{Since: since})
	if len(result) != 2 {
		t.Errorf("过滤后应返回 2 条, 实际 %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// SummaryRecord JSON
// ---------------------------------------------------------------------------

func TestSummaryRecordJSON(t *testing.T) {
	record := SummaryRecord{
		NumCPU:  8,
		EndedAt: "2026-02-16T15:04:05+08:00",
		ResourceSummary: ResourceSummary{
			SampleCount:  100,
			CPUMin:       1.5,
			CPUMax:       95.0,
			CPUAvg:       30.0,
			MemoryMin:    1024,
			MemoryMax:    2048,
			MemoryAvg:    1536,
			GoroutineMin: 5,
			GoroutineMax: 50,
			GoroutineAvg: 20,
		},
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}

	var decoded SummaryRecord
	if err = json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}

	if decoded.NumCPU != record.NumCPU {
		t.Errorf("NumCPU = %d, 期望 %d", decoded.NumCPU, record.NumCPU)
	}
	if decoded.SampleCount != record.SampleCount {
		t.Errorf("SampleCount = %d, 期望 %d", decoded.SampleCount, record.SampleCount)
	}
	if decoded.CPUAvg != record.CPUAvg {
		t.Errorf("CPUAvg = %.1f, 期望 %.1f", decoded.CPUAvg, record.CPUAvg)
	}
}

// ---------------------------------------------------------------------------
// AnalyzeRecords
// ---------------------------------------------------------------------------

func TestAnalyzeRecords(t *testing.T) {
	records := []SummaryRecord{
		{
			NumCPU:  4,
			EndedAt: "2026-02-16T10:00:00+08:00",
			ResourceSummary: ResourceSummary{
				SampleCount: 100, CPUMin: 10, CPUMax: 80, CPUAvg: 40,
				MemoryMin: 1000, MemoryMax: 5000, MemoryAvg: 3000,
				GoroutineMin: 5, GoroutineMax: 50, GoroutineAvg: 20,
			},
		},
		{
			NumCPU:  8,
			EndedAt: "2026-02-16T11:00:00+08:00",
			ResourceSummary: ResourceSummary{
				SampleCount: 200, CPUMin: 20, CPUMax: 160, CPUAvg: 80,
				MemoryMin: 2000, MemoryMax: 8000, MemoryAvg: 5000,
				GoroutineMin: 10, GoroutineMax: 100, GoroutineAvg: 50,
			},
		},
	}

	results, report := AnalyzeRecords(records, nil)
	if len(results) != 2 {
		t.Fatalf("应返回 2 个分组, 实际 %d", len(results))
	}
	if results[0].NumCPU != 4 {
		t.Errorf("第一组 NumCPU = %d, 期望 4", results[0].NumCPU)
	}
	if results[1].NumCPU != 8 {
		t.Errorf("第二组 NumCPU = %d, 期望 8", results[1].NumCPU)
	}
	if report == "" {
		t.Error("报告不应为空")
	}
}

func TestAnalyzeRecordsEmpty(t *testing.T) {
	results, report := AnalyzeRecords(nil, nil)
	if results != nil {
		t.Error("空输入应返回 nil results")
	}
	if report != "无记录" {
		t.Errorf("空输入报告 = %q, 期望 %q", report, "无记录")
	}
}
