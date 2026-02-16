# monitor - 进程资源监控

```go
import "github.com/pylemonorg/gotools/monitor"
```

定时采样当前进程的 CPU、内存、Goroutine 等指标，Stop 时输出汇总，并可选将汇总持久化到 Redis。

## 快速开始

```go
mon, err := monitor.NewResourceMonitor(nil) // 默认配置，采样间隔 2s
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
        fmt.Println(stats.FormatStats()) // 自定义回调（设置后不再输出默认日志）
    },
})
```

## 保存汇总到 Redis

汇总数据通过 `RPUSH` 追加到 Redis List，每次保存为一条 JSON 记录。

### 初始化时设置（推荐）

```go
saver := monitor.NewRedisSummarySaver(redisClient)

mon, _ := monitor.NewResourceMonitor(&monitor.Config{
    Interval:    5 * time.Second,
    LogInterval: 10 * time.Second,
    Saver:       saver,
    SaveKey:     "resource:summary:myapp",
})
mon.Start()

// ... 业务逻辑 ...

mon.Stop() // 自动输出汇总 + 保存到 Redis
```

### 运行中动态设置

```go
mon.SetSaver(saver, "resource:summary:myapp")
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

## 资源分析

从 Redis 读取历史汇总记录，按 CPU 核心数分组后聚合分析，输出格式化报告。

### 从 Redis 分析

```go
results, report, err := monitor.AnalyzeFromRedis(redisClient, "resource:summary:myapp", nil)
if err != nil {
    log.Fatal(err)
}
fmt.Println(report)
```

### 带时间过滤

```go
since, _ := time.Parse(time.RFC3339, "2026-02-01T00:00:00+08:00")
results, report, err := monitor.AnalyzeFromRedis(redisClient, key, &monitor.AnalyzeOptions{
    Since: since,
})
```

### 直接分析记录切片（不依赖 Redis）

```go
results, report := monitor.AnalyzeRecords(records, nil)
```

`AnalyzeResult` 结构体包含聚合后的指标：

| 字段 | 类型 | 说明 |
|------|------|------|
| `NumCPU` | int | CPU 核心数 |
| `RecordCount` | int | 记录条数 |
| `TotalSamples` | int | 总采样次数 |
| `CPUMin / CPUMax / CPUAvg` | float64 | CPU 使用率（加权） |
| `MemoryMin / MemoryMax / MemoryAvg` | uint64 | 常驻内存（字节，加权） |
| `GoroutineMin / GoroutineMax / GoroutineAvg` | int | Goroutine 数量（加权） |

## 文件结构

| 文件 | 职责 |
|------|------|
| `types.go` | 所有结构体、接口定义 |
| `resource.go` | 监控器生命周期、采集、汇总 |
| `redis_saver.go` | Redis 持久化实现 |
| `analyze.go` | 历史记录聚合分析 |
| `format.go` | 格式化工具（FormatBytes、报告排版） |

## 主要 API

| 方法 | 说明 |
|------|------|
| `NewResourceMonitor(cfg)` | 创建监控器，cfg 可为 nil |
| `Start()` | 启动异步采样（清空上轮历史） |
| `Stop()` | 停止采样、输出汇总、可选持久化 |
| `GetStats()` | 获取当前资源快照 |
| `GetSummary()` | 获取已采集数据的汇总 |
| `SetSaver(saver, key)` | 设置或更新持久化方式 |
| `NewRedisSummarySaver(client)` | 创建 Redis SummarySaver 实例 |
| `AnalyzeFromRedis(client, key, opts)` | 从 Redis 读取并聚合分析 |
| `AnalyzeRecords(records, opts)` | 直接分析记录切片 |
| `FormatBytes(bytes)` | 字节数格式化（B/KB/MB/GB） |
