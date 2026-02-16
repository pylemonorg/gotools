# monitor - 进程资源监控

```go
import "github.com/pylemonorg/gotools/monitor"
```

定时采样当前进程的 CPU、内存、Goroutine 等指标，Stop 时输出汇总，并可选将汇总持久化到 Redis。

## 快速开始

```go
// 使用默认配置（采样间隔 2s）
mon, err := monitor.NewResourceMonitor(nil)
if err != nil {
    log.Fatal(err)
}
mon.Start()

// ... 业务逻辑 ...

mon.Stop() // 停止并打印汇总
```

## 自定义配置

```go
mon, _ := monitor.NewResourceMonitor(&monitor.Config{
    Interval:    5 * time.Second,  // 采样间隔
    LogInterval: 10 * time.Second, // 日志输出间隔（不影响采样频率）
    OnStats: func(stats *monitor.ResourceStats) {
        // 自定义采样回调（设置后不再输出默认日志）
        fmt.Println(stats.FormatStats())
    },
})
```

## 保存汇总到 Redis

汇总数据通过 `RPUSH` 追加到 Redis List，每次保存为一条 JSON 记录。

### 方式一：手动保存

在 `Stop()` 之后调用 `SaveSummaryToRedis`：

```go
mon.Stop()
err := mon.SaveSummaryToRedis(redisClient, "resource:summary:myapp")
```

### 方式二：自动保存（Stop 时触发）

通过 `SaveToRedis` 便捷方法，在 `Stop()` 时自动保存：

```go
saver := monitor.NewRedisSummarySaver(redisClient)
mon.SaveToRedis(saver, "resource:summary:myapp")

mon.Start()
// ... 业务逻辑 ...
mon.Stop() // 自动保存汇总到 Redis
```

或通过 Config 初始化时设置：

```go
saver := monitor.NewRedisSummarySaver(redisClient)
mon, _ := monitor.NewResourceMonitor(&monitor.Config{
    GetSummarySaver: func() (monitor.SummarySaver, string) {
        return saver, "resource:summary:myapp"
    },
})
```

## 保存到 Redis 的 JSON 结构

每次 RPUSH 的 value 为以下 JSON：

```json
{
  "num_cpu": 8,
  "ended_at": "2026-02-16T15:04:05+08:00",
  "sample_count": 150,
  "cpu_min": 2.1,
  "cpu_max": 85.3,
  "cpu_avg": 32.5,
  "memory_min": 52428800,
  "memory_max": 209715200,
  "memory_avg": 104857600,
  "goroutine_min": 5,
  "goroutine_max": 120,
  "goroutine_avg": 45
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `num_cpu` | int | CPU 核心数 |
| `ended_at` | string | 记录时间（RFC3339） |
| `sample_count` | int | 采样次数 |
| `cpu_min` | float64 | CPU 使用率最小值（%，多核可能 >100） |
| `cpu_max` | float64 | CPU 使用率最大值 |
| `cpu_avg` | float64 | CPU 使用率平均值 |
| `memory_min` | uint64 | 常驻内存最小值（字节） |
| `memory_max` | uint64 | 常驻内存最大值（字节） |
| `memory_avg` | uint64 | 常驻内存平均值（字节） |
| `goroutine_min` | int | Goroutine 最小数量 |
| `goroutine_max` | int | Goroutine 最大数量 |
| `goroutine_avg` | int | Goroutine 平均数量 |

## 主要 API

| 方法 | 说明 |
|------|------|
| `NewResourceMonitor(cfg)` | 创建监控器，cfg 可为 nil |
| `Start()` | 启动异步采样 |
| `Stop()` | 停止采样并输出汇总 |
| `GetStats()` | 获取当前资源快照 |
| `GetSummary()` | 获取已采集数据的汇总 |
| `SaveSummaryToRedis(client, key)` | 手动保存汇总到 Redis List |
| `SaveToRedis(saver, key)` | 设置自动保存（Stop 时触发） |
| `SetSummarySaver(getter)` | 设置自定义持久化回调 |
| `NewRedisSummarySaver(client)` | 创建 Redis SummarySaver 实例 |
| `FormatBytes(bytes)` | 字节数格式化（B/KB/MB/GB） |
