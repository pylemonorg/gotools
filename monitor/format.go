package monitor

import (
	"fmt"
	"strings"
	"text/tabwriter"
)

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

// ---------------------------------------------------------------------------
// 分析报告格式化（内部）
// ---------------------------------------------------------------------------

// formatReport 将分析结果格式化为可读的表格报告。
func formatReport(results []AnalyzeResult) string {
	var buf strings.Builder
	w := tabwriter.NewWriter(&buf, 0, 0, 3, ' ', 0)

	fmt.Fprintln(w, "\n========================================= 资源分析 =========================================")

	for _, r := range results {
		formatOneGroup(w, r)
	}

	w.Flush()
	return buf.String()
}

// formatOneGroup 格式化单个 CPU 分组的报告。
func formatOneGroup(w *tabwriter.Writer, r AnalyzeResult) {
	col1, col2, col3, col4, col5 := 18, 15, 15, 15, 15

	fmt.Fprintf(w, "CPU 核心数: %d\t(总记录数: %d, 总样本数: %d)\n", r.NumCPU, r.RecordCount, r.TotalSamples)
	fmt.Fprintln(w, strings.Repeat("-", 100))

	// 表头
	fmt.Fprintf(w, "%s%s%s%s%s\n",
		padRightCJK("指标", col1),
		padRightCJK("最小值", col2),
		padRightCJK("最大值", col3),
		padRightCJK("加权平均值", col4),
		padRightCJK("平均值/核心", col5))

	fmt.Fprintf(w, "%s%s%s%s%s\n",
		padRightCJK("------", col1),
		padRightCJK("---", col2),
		padRightCJK("---", col3),
		padRightCJK("--------", col4),
		padRightCJK("--------", col5))

	// CPU
	perCore := "-"
	if r.NumCPU > 0 {
		perCore = fmt.Sprintf("%.2f", r.CPUAvg/float64(r.NumCPU))
	}
	fmt.Fprintf(w, "%s%s%s%s%s\n",
		padRightCJK("CPU使用率 (%)", col1),
		padRightCJK(fmt.Sprintf("%.2f", r.CPUMin), col2),
		padRightCJK(fmt.Sprintf("%.2f", r.CPUMax), col3),
		padRightCJK(fmt.Sprintf("%.2f", r.CPUAvg), col4),
		padRightCJK(perCore, col5))

	// 内存
	fmt.Fprintf(w, "%s%s%s%s%s\n",
		padRightCJK("内存", col1),
		padRightCJK(FormatBytes(r.MemoryMin), col2),
		padRightCJK(FormatBytes(r.MemoryMax), col3),
		padRightCJK(FormatBytes(r.MemoryAvg), col4),
		padRightCJK("-", col5))

	// Goroutine
	fmt.Fprintf(w, "%s%s%s%s%s\n",
		padRightCJK("协程数", col1),
		padRightCJK(fmt.Sprintf("%d", r.GoroutineMin), col2),
		padRightCJK(fmt.Sprintf("%d", r.GoroutineMax), col3),
		padRightCJK(fmt.Sprintf("%d", r.GoroutineAvg), col4),
		padRightCJK("-", col5))

	fmt.Fprintln(w)
}

// cjkWidth 计算字符串显示宽度（CJK 字符算 2，ASCII 算 1）。
func cjkWidth(s string) int {
	n := 0
	for _, r := range s {
		if r > 127 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// padRightCJK 按显示宽度右填充空格，正确处理中文字符。
func padRightCJK(s string, width int) string {
	w := cjkWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}
