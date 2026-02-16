package timeutil

import (
	"fmt"
	"time"

	"github.com/pylemonorg/gotools/logger"
)

// FormatDuration 将 time.Duration 格式化为人类可读的字符串。
// < 1s → "320ms"，< 1min → "2.50秒"，>= 1min → "3分12秒"。
func FormatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.2f秒", d.Seconds())
	default:
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%d分%d秒", m, s)
	}
}

// TrackTime 返回一个 deferred 函数，用于统计并记录代码块的执行耗时。
//
// 用法：
//
//	func DoWork() {
//	    defer timeutil.TrackTime("DoWork")()
//	    // ... 业务逻辑
//	}
func TrackTime(name string) func() {
	start := time.Now()
	return func() {
		logger.Infof("%s 总耗时: %s", name, FormatDuration(time.Since(start)))
	}
}

// EnsureMinRunTime 返回一个 deferred 函数，确保代码块的运行时间不低于 minDuration。
// 若实际运行时间不足，则暂停 pauseMinutes 分钟后再返回。
//
// 用法：
//
//	func MyTask() {
//	    defer timeutil.EnsureMinRunTime("MyTask", 5*time.Minute, 10)()
//	    // ... 任务逻辑
//	}
func EnsureMinRunTime(name string, minDuration time.Duration, pauseMinutes int) func() {
	start := time.Now()
	return func() {
		elapsed := time.Since(start)
		logger.Infof("%s 总耗时: %s", name, FormatDuration(elapsed))

		if elapsed >= minDuration {
			return
		}

		pause := time.Duration(pauseMinutes) * time.Minute
		logger.Warnf("%s 运行时间(%s)小于阈值(%s)，暂停 %d 分钟...",
			name, FormatDuration(elapsed), FormatDuration(minDuration), pauseMinutes)
		time.Sleep(pause)
		logger.Infof("%s 暂停结束，继续执行", name)
	}
}
