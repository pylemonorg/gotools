# gotools

Go 通用工具包，提供日志、数据库、对象存储、JSON、哈希、字符串、URL、时间、指针等常用工具函数。

## 安装

```bash
go get github.com/pylemonorg/gotools@latest
```

## 包说明

| 包 | 导入路径 | 说明 |
|----|---------|------|
| **logger** | `gotools/logger` | 基于 zerolog 的日志库，支持彩色控制台 / JSON 输出 / 文件写入 |
| **db** | `gotools/db` | Redis 和 PostgreSQL 客户端封装，支持重连、重试、批量插入 |
| **obsutil** | `gotools/obsutil` | 华为云 OBS 对象存储客户端封装，支持上传/下载/分段上传/流式上传/分布式锁 |
| **monitor** | `gotools/monitor` | 进程资源监控（CPU/内存/Goroutine），支持定时采样、汇总统计、持久化 |
| **jsonutil** | `gotools/jsonutil` | JSON 序列化/反序列化、文件读写、类型安全取值 |
| **hashutil** | `gotools/hashutil` | MD5、SHA-256、xxhash 分桶、随机字符串 |
| **strutil** | `gotools/strutil` | 字符串处理（Strip）、Base64 编解码 |
| **urlutil** | `gotools/urlutil` | 相对 URL 解析为绝对 URL、URL 哈希 |
| **timeutil** | `gotools/timeutil` | 耗时格式化、函数计时、最小运行时间保障 |
| **ptr** | `gotools/ptr` | 泛型指针工具 `To[T]` / `Deref[T]` |

## 快速示例

```go
import (
    "github.com/pylemonorg/gotools/logger"
    "github.com/pylemonorg/gotools/jsonutil"
    "github.com/pylemonorg/gotools/hashutil"
    "github.com/pylemonorg/gotools/ptr"
    "github.com/pylemonorg/gotools/timeutil"
)

// 日志
logger.Init(logger.LevelInfo, true)
logger.Infof("hello %s", "world")

// JSON
s := jsonutil.MustMarshalString(map[string]any{"name": "张三"})
m, _ := jsonutil.ToMapFromString(s)
name := jsonutil.GetString(m, "name")

// 哈希
md5, _ := hashutil.MD5("hello")
key := hashutil.BucketKey("user", "abc", 1024)

// 指针
p := ptr.To(42)
v := ptr.Deref(p)

// 函数计时
func DoWork() {
    defer timeutil.TrackTime("DoWork")()
    // ...
}
```

## 版本管理

### 发布版本

```bash
# 创建附注 Tag
git tag -a v0.2.0 -m "新增 jsonutil 包，重构 utils 为独立子包"

# 推送 Tag
git push origin v0.2.0

# 批量推送所有本地未推送的 Tag
git push origin --tags
```

### 版本号规则

按语义化版本（SemVer）更新 Tag：

- **修订号**（第三位）：修复 Bug，不新增功能（如 `v0.1.1`）
- **次版本**（第二位）：新增兼容功能（如 `v0.2.0`）
- **主版本**（第一位）：不兼容的重大更新（如 `v1.0.0`）

### 删除版本

删除本地和远程 Tag 即可，GitHub Release 会自动清理。

```bash
git tag -d v0.1.1              # 删除本地 Tag
git push origin --delete v0.1.1 # 删除远程 Tag
```

## License

[MIT](LICENSE)
