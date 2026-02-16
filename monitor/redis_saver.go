package monitor

import (
	"github.com/pylemonorg/gotools/db"
)

// RedisSummarySaver 基于 db.RedisClient 的 SummarySaver 实现。
// 通过 RPUSH 将 JSON 追加到 Redis List。
//
// 用法：
//
//	saver := monitor.NewRedisSummarySaver(redisClient)
//	mon, _ := monitor.NewResourceMonitor(&monitor.Config{
//	    Saver:   saver,
//	    SaveKey: "resource:summary:myapp",
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
