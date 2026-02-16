package monitor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pylemonorg/gotools/db"
	"github.com/pylemonorg/gotools/logger"
)

// SaveSummaryToRedis 将资源使用汇总序列化为 JSON，通过 RPUSH 追加到 Redis List。
// 适用于在 Stop() 之后手动调用，或在任意时刻保存当前汇总快照。
//
// 参数：
//   - redisClient: db.RedisClient 实例
//   - key: Redis List 的 key
//
// 用法：
//
//	mon.Stop()
//	mon.SaveSummaryToRedis(redisClient, "resource:summary:myapp")
func (m *ResourceMonitor) SaveSummaryToRedis(redisClient *db.RedisClient, key string) error {
	summary := m.GetSummary()
	if summary == nil {
		return fmt.Errorf("monitor: 无采样数据，无法保存汇总")
	}

	record := resourceSummaryRecord{
		NumCPU:          m.numCPU,
		EndedAt:         time.Now().Format(time.RFC3339),
		ResourceSummary: summary,
	}

	jsonBytes, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("monitor: 汇总 JSON 序列化失败: %w", err)
	}

	if _, err = redisClient.RPush(key, string(jsonBytes)); err != nil {
		return fmt.Errorf("monitor: RPUSH 到 Redis [%s] 失败: %w", key, err)
	}

	logger.Infof("monitor: 汇总已保存到 Redis List [%s]", key)
	return nil
}

// RedisSummarySaver 基于 db.RedisClient 的 SummarySaver 实现。
// 通过 RPUSH 将 JSON 追加到 Redis List，适用于通过 Config.GetSummarySaver 自动保存的场景。
//
// 用法：
//
//	saver := monitor.NewRedisSummarySaver(redisClient)
//	mon, _ := monitor.NewResourceMonitor(&monitor.Config{
//	    GetSummarySaver: func() (monitor.SummarySaver, string) {
//	        return saver, "resource:summary:myapp"
//	    },
//	})
type RedisSummarySaver struct {
	client *db.RedisClient
}

// NewRedisSummarySaver 创建基于 RedisClient 的 SummarySaver。
func NewRedisSummarySaver(client *db.RedisClient) *RedisSummarySaver {
	return &RedisSummarySaver{client: client}
}

// SaveSummary 实现 SummarySaver 接口，通过 RPUSH 将 jsonValue 追加到 Redis List。
func (s *RedisSummarySaver) SaveSummary(key string, jsonValue string) error {
	_, err := s.client.RPush(key, jsonValue)
	return err
}
