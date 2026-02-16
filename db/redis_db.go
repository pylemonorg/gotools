package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pylemonorg/gotools/logger"
	"github.com/redis/go-redis/v9"
)

// Redis 相关的哨兵错误。
var (
	ErrRedisNilParams = errors.New("redis: 连接参数不能为 nil")
	ErrRedisNotInit   = errors.New("redis: 客户端未初始化")
	ErrRedisNoParams  = errors.New("redis: 连接参数未设置，无法重连")
)

// connectionKeywords 用于判断连接类错误的关键词。
var connectionKeywords = []string{
	"connection", "timeout", "eof", "broken pipe",
	"network", "dial", "i/o timeout",
	"connection refused", "connection reset",
	"no connection", "connection closed",
}

// RedisClient 封装了 go-redis 客户端，内部管理 context，提供便捷的 Redis 操作方法。
type RedisClient struct {
	client *redis.Client
	ctx    context.Context
	params *RedisParams
}

// RedisParams 定义 Redis 连接所需的参数。
type RedisParams struct {
	Host     string // 主机地址
	Port     int    // 端口号
	Password string // 密码（无密码传空串）
	DB       int    // 数据库编号
}

// validateRedisParams 校验 Redis 连接参数的必填项。
func validateRedisParams(params *RedisParams) error {
	var missing []string
	if strings.TrimSpace(params.Host) == "" {
		missing = append(missing, "Host")
	}
	if params.Port <= 0 {
		missing = append(missing, "Port")
	}
	if len(missing) > 0 {
		return fmt.Errorf("redis: 缺少必要连接参数: %s", strings.Join(missing, ", "))
	}
	return nil
}

// dialRedis 创建 Redis 客户端并测试连通性（内部方法）。
func dialRedis(params *RedisParams) (*redis.Client, error) {
	addr := fmt.Sprintf("%s:%d", params.Host, params.Port)

	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     params.Password,
		DB:           params.DB,
		DialTimeout:  30 * time.Second,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	})

	if _, err := client.Ping(context.Background()).Result(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis: 连接 %s 失败: %w", addr, err)
	}

	return client, nil
}

// NewRedisClient 根据给定参数创建 RedisClient 实例。
func NewRedisClient(params *RedisParams) (*RedisClient, error) {
	if params == nil {
		return nil, ErrRedisNilParams
	}
	if err := validateRedisParams(params); err != nil {
		return nil, err
	}

	client, err := dialRedis(params)
	if err != nil {
		return nil, err
	}

	logger.Infof("redis: 连接成功 %s:%d db=%d", params.Host, params.Port, params.DB)
	return &RedisClient{
		client: client,
		ctx:    context.Background(),
		params: params,
	}, nil
}

// GetClient 返回底层 redis.Client，可用于执行未封装的高级操作。
func (rc *RedisClient) GetClient() *redis.Client { return rc.client }

// GetContext 返回当前使用的 context。
func (rc *RedisClient) GetContext() context.Context { return rc.ctx }

// SetContext 替换内部 context（如需要超时控制等场景）。
func (rc *RedisClient) SetContext(ctx context.Context) { rc.ctx = ctx }

// GetParams 返回创建时使用的连接参数。
func (rc *RedisClient) GetParams() *RedisParams { return rc.params }

// Close 关闭 Redis 连接。
func (rc *RedisClient) Close() error {
	if rc.client == nil {
		return nil
	}
	return rc.client.Close()
}

// Ping 测试当前连接是否可用。
func (rc *RedisClient) Ping() error {
	if rc.client == nil {
		return ErrRedisNotInit
	}
	_, err := rc.client.Ping(rc.ctx).Result()
	return err
}

// Reconnect 关闭旧连接并使用原始参数重新建立连接。
// maxRetries <= 0 时默认 3 次，retryDelay <= 0 时默认 1s。
func (rc *RedisClient) Reconnect(maxRetries int, retryDelay time.Duration) error {
	if rc.params == nil {
		return ErrRedisNoParams
	}
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if retryDelay <= 0 {
		retryDelay = time.Second
	}

	// 关闭旧连接
	if rc.client != nil {
		rc.client.Close()
		rc.client = nil
	}

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		logger.Warnf("redis: 正在重连 (%d/%d)...", i+1, maxRetries)
		newClient, err := dialRedis(rc.params)
		if err != nil {
			lastErr = err
			if i < maxRetries-1 {
				time.Sleep(retryDelay)
			}
			continue
		}
		rc.client = newClient
		logger.Infof("redis: 重连成功")
		return nil
	}
	return fmt.Errorf("redis: 重连失败（已重试 %d 次）: %w", maxRetries, lastErr)
}

// ExecuteWithRetry 执行操作函数，遇到连接错误时自动重连并重试。
// maxRetries <= 0 时默认 3 次，retryDelay <= 0 时默认 1s。
func (rc *RedisClient) ExecuteWithRetry(operation func() (any, error), maxRetries int, retryDelay time.Duration) (any, error) {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if retryDelay <= 0 {
		retryDelay = time.Second
	}

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		result, err := operation()
		if err == nil {
			return result, nil
		}
		// 非连接错误直接返回
		if !isConnectionError(err) {
			return nil, err
		}
		lastErr = err
		logger.Warnf("redis: 操作遇到连接错误，尝试重连: %v", err)
		if reconnErr := rc.Reconnect(maxRetries, retryDelay); reconnErr != nil {
			return nil, fmt.Errorf("redis: 操作失败且重连失败: %w (重连: %v)", err, reconnErr)
		}
		if i < maxRetries-1 {
			time.Sleep(retryDelay)
		}
	}
	return nil, fmt.Errorf("redis: 操作失败（已重试 %d 次）: %w", maxRetries, lastErr)
}

// isConnectionError 判断 err 是否为连接类错误。
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	for _, kw := range connectionKeywords {
		if strings.Contains(errStr, kw) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 基本操作
// ---------------------------------------------------------------------------

// Set 设置键值对，expiration 为 0 表示不过期。
func (rc *RedisClient) Set(key string, value any, expiration time.Duration) error {
	return rc.client.Set(rc.ctx, key, value, expiration).Err()
}

// Get 获取 key 对应的值。
func (rc *RedisClient) Get(key string) (string, error) {
	return rc.client.Get(rc.ctx, key).Result()
}

// Del 删除一个或多个 key，返回实际删除的数量。
func (rc *RedisClient) Del(keys ...string) (int64, error) {
	return rc.client.Del(rc.ctx, keys...).Result()
}

// Exists 检查 key 是否存在，返回存在的数量。
func (rc *RedisClient) Exists(keys ...string) (int64, error) {
	return rc.client.Exists(rc.ctx, keys...).Result()
}

// Expire 为 key 设置过期时间。
func (rc *RedisClient) Expire(key string, expiration time.Duration) (bool, error) {
	return rc.client.Expire(rc.ctx, key, expiration).Result()
}

// TTL 获取 key 的剩余过期时间。
// 返回 -1 表示永不过期，-2 表示 key 不存在。
func (rc *RedisClient) TTL(key string) (time.Duration, error) {
	return rc.client.TTL(rc.ctx, key).Result()
}

// ExpireIfNotSet 仅在 key 没有过期时间时设置（兼容所有 Redis 版本，需两次调用）。
func (rc *RedisClient) ExpireIfNotSet(key string, expiration time.Duration) (bool, error) {
	ttl, err := rc.TTL(key)
	if err != nil {
		return false, err
	}
	if ttl == -1*time.Second {
		return rc.Expire(key, expiration)
	}
	return false, nil
}

// ExpireNX 仅在 key 没有过期时间时设置（需要 Redis 7.0+，单次调用）。
func (rc *RedisClient) ExpireNX(key string, expiration time.Duration) (bool, error) {
	return rc.client.ExpireNX(rc.ctx, key, expiration).Result()
}

// ---------------------------------------------------------------------------
// 数值操作
// ---------------------------------------------------------------------------

// Incr 将 key 对应的值加 1。
func (rc *RedisClient) Incr(key string) (int64, error) {
	return rc.client.Incr(rc.ctx, key).Result()
}

// IncrBy 将 key 对应的值加上指定增量。
func (rc *RedisClient) IncrBy(key string, value int64) (int64, error) {
	return rc.client.IncrBy(rc.ctx, key, value).Result()
}

// Decr 将 key 对应的值减 1。
func (rc *RedisClient) Decr(key string) (int64, error) {
	return rc.client.Decr(rc.ctx, key).Result()
}

// DecrBy 将 key 对应的值减去指定值。
func (rc *RedisClient) DecrBy(key string, value int64) (int64, error) {
	return rc.client.DecrBy(rc.ctx, key, value).Result()
}

// ---------------------------------------------------------------------------
// Set（集合）操作
// ---------------------------------------------------------------------------

// SAdd 向集合添加成员，返回新增的成员数。
func (rc *RedisClient) SAdd(key string, members ...any) (int64, error) {
	return rc.client.SAdd(rc.ctx, key, members...).Result()
}

// SMembers 获取集合的所有成员。
func (rc *RedisClient) SMembers(key string) ([]string, error) {
	return rc.client.SMembers(rc.ctx, key).Result()
}

// SPopN 从集合中随机移除并返回 count 个成员。
func (rc *RedisClient) SPopN(key string, count int64) ([]string, error) {
	return rc.client.SPopN(rc.ctx, key, count).Result()
}

// SCard 返回集合的成员数量。
func (rc *RedisClient) SCard(key string) (int64, error) {
	return rc.client.SCard(rc.ctx, key).Result()
}

// SRem 从集合中移除指定成员，返回实际移除的数量。
func (rc *RedisClient) SRem(key string, members ...any) (int64, error) {
	return rc.client.SRem(rc.ctx, key, members...).Result()
}

// SIsMember 判断 member 是否是集合的成员。
func (rc *RedisClient) SIsMember(key string, member any) (bool, error) {
	return rc.client.SIsMember(rc.ctx, key, member).Result()
}

// ---------------------------------------------------------------------------
// Sorted Set（有序集合）操作
// ---------------------------------------------------------------------------

// ZAdd 向有序集合添加一个成员。
func (rc *RedisClient) ZAdd(key string, score float64, member string) (int64, error) {
	return rc.client.ZAdd(rc.ctx, key, redis.Z{Score: score, Member: member}).Result()
}

// ZAddMulti 向有序集合批量添加成员。
func (rc *RedisClient) ZAddMulti(key string, members ...redis.Z) (int64, error) {
	return rc.client.ZAdd(rc.ctx, key, members...).Result()
}

// ZRangeByScore 按分数范围获取成员（升序）。
func (rc *RedisClient) ZRangeByScore(key string, min, max float64) ([]string, error) {
	return rc.client.ZRangeByScore(rc.ctx, key, &redis.ZRangeBy{
		Min: fmt.Sprintf("%f", min),
		Max: fmt.Sprintf("%f", max),
	}).Result()
}

// ZRangeByScoreWithScores 按分数范围获取成员及分数（升序）。
func (rc *RedisClient) ZRangeByScoreWithScores(key string, min, max float64) ([]redis.Z, error) {
	return rc.client.ZRangeByScoreWithScores(rc.ctx, key, &redis.ZRangeBy{
		Min: fmt.Sprintf("%f", min),
		Max: fmt.Sprintf("%f", max),
	}).Result()
}

// ZRemRangeByScore 按分数范围删除成员，返回删除的数量。
func (rc *RedisClient) ZRemRangeByScore(key string, min, max float64) (int64, error) {
	return rc.client.ZRemRangeByScore(rc.ctx, key, fmt.Sprintf("%f", min), fmt.Sprintf("%f", max)).Result()
}

// ZCard 返回有序集合的成员数量。
func (rc *RedisClient) ZCard(key string) (int64, error) {
	return rc.client.ZCard(rc.ctx, key).Result()
}

// ZScore 获取指定成员的分数。
func (rc *RedisClient) ZScore(key, member string) (float64, error) {
	return rc.client.ZScore(rc.ctx, key, member).Result()
}

// ZRem 删除有序集合中的指定成员。
func (rc *RedisClient) ZRem(key string, members ...any) (int64, error) {
	return rc.client.ZRem(rc.ctx, key, members...).Result()
}

// ---------------------------------------------------------------------------
// Hash（哈希）操作
// ---------------------------------------------------------------------------

// HSet 设置哈希表中的字段值。
func (rc *RedisClient) HSet(key string, values ...any) (int64, error) {
	return rc.client.HSet(rc.ctx, key, values...).Result()
}

// HGet 获取哈希表中指定字段的值。
func (rc *RedisClient) HGet(key, field string) (string, error) {
	return rc.client.HGet(rc.ctx, key, field).Result()
}

// HGetAll 获取哈希表中所有字段和值。
func (rc *RedisClient) HGetAll(key string) (map[string]string, error) {
	return rc.client.HGetAll(rc.ctx, key).Result()
}

// HDel 删除哈希表中的指定字段。
func (rc *RedisClient) HDel(key string, fields ...string) (int64, error) {
	return rc.client.HDel(rc.ctx, key, fields...).Result()
}

// HExists 判断哈希表中字段是否存在。
func (rc *RedisClient) HExists(key, field string) (bool, error) {
	return rc.client.HExists(rc.ctx, key, field).Result()
}

// HIncrBy 为哈希表中指定字段的值加上增量。
func (rc *RedisClient) HIncrBy(key, field string, incr int64) (int64, error) {
	return rc.client.HIncrBy(rc.ctx, key, field, incr).Result()
}

// ---------------------------------------------------------------------------
// List（列表）操作
// ---------------------------------------------------------------------------

// LPush 从列表左侧推入元素，返回列表长度。
func (rc *RedisClient) LPush(key string, values ...any) (int64, error) {
	return rc.client.LPush(rc.ctx, key, values...).Result()
}

// RPush 从列表右侧推入元素，返回列表长度。
func (rc *RedisClient) RPush(key string, values ...any) (int64, error) {
	return rc.client.RPush(rc.ctx, key, values...).Result()
}

// LPop 从列表左侧弹出一个元素。
func (rc *RedisClient) LPop(key string) (string, error) {
	return rc.client.LPop(rc.ctx, key).Result()
}

// RPop 从列表右侧弹出一个元素。
func (rc *RedisClient) RPop(key string) (string, error) {
	return rc.client.RPop(rc.ctx, key).Result()
}

// LLen 返回列表的长度。
func (rc *RedisClient) LLen(key string) (int64, error) {
	return rc.client.LLen(rc.ctx, key).Result()
}

// ---------------------------------------------------------------------------
// 其他操作
// ---------------------------------------------------------------------------

// MemoryUsage 返回指定 key 的内存占用（字节），使用 MEMORY USAGE 命令。
func (rc *RedisClient) MemoryUsage(key string) (int64, error) {
	result, err := rc.client.Do(rc.ctx, "MEMORY", "USAGE", key).Result()
	if err != nil {
		return 0, err
	}
	if size, ok := result.(int64); ok {
		return size, nil
	}
	return 0, fmt.Errorf("redis: 无法解析 MEMORY USAGE 返回值: %v", result)
}

// GetRedisVersion 获取 Redis 服务器版本号（如 "7.0.5"）。
func (rc *RedisClient) GetRedisVersion() (string, error) {
	info, err := rc.client.Info(rc.ctx, "server").Result()
	if err != nil {
		return "", fmt.Errorf("redis: 获取 server info 失败: %w", err)
	}
	for _, line := range strings.Split(info, "\n") {
		if strings.HasPrefix(line, "redis_version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "redis_version:")), nil
		}
	}
	return "", errors.New("redis: server info 中未找到 redis_version 字段")
}

// IsRedis7OrAbove 判断 Redis 服务器版本是否 >= 7.0。
func (rc *RedisClient) IsRedis7OrAbove() bool {
	version, err := rc.GetRedisVersion()
	if err != nil {
		return false
	}
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	return major >= 7
}

// ---------------------------------------------------------------------------
// Pipeline / 事务
// ---------------------------------------------------------------------------

// Pipeline 创建一个管道，用于批量发送命令。
func (rc *RedisClient) Pipeline() redis.Pipeliner {
	return rc.client.Pipeline()
}

// TxPipeline 创建一个事务管道（MULTI/EXEC）。
func (rc *RedisClient) TxPipeline() redis.Pipeliner {
	return rc.client.TxPipeline()
}

// ExecPipeline 执行管道中缓冲的所有命令。
func (rc *RedisClient) ExecPipeline(pipe redis.Pipeliner) ([]redis.Cmder, error) {
	return pipe.Exec(rc.ctx)
}

// PipeHIncrBy 向管道追加一条 HINCRBY 命令。
func (rc *RedisClient) PipeHIncrBy(pipe redis.Pipeliner, key, field string, incr int64) {
	pipe.HIncrBy(rc.ctx, key, field, incr)
}

// ---------------------------------------------------------------------------
// 带重试的操作（连接异常时自动重连）
// ---------------------------------------------------------------------------

// SetWithRetry 设置键值对（带自动重连重试）。
func (rc *RedisClient) SetWithRetry(key string, value any, expiration time.Duration, maxRetries int, retryDelay time.Duration) error {
	_, err := rc.ExecuteWithRetry(func() (any, error) {
		return nil, rc.Set(key, value, expiration)
	}, maxRetries, retryDelay)
	return err
}

// GetWithRetry 获取键值（带自动重连重试）。
func (rc *RedisClient) GetWithRetry(key string, maxRetries int, retryDelay time.Duration) (string, error) {
	result, err := rc.ExecuteWithRetry(func() (any, error) {
		return rc.Get(key)
	}, maxRetries, retryDelay)
	if err != nil {
		return "", err
	}
	return result.(string), nil
}

// SPopNWithRetry 从集合中随机弹出 count 个成员（带自动重连重试）。
func (rc *RedisClient) SPopNWithRetry(key string, count int64, maxRetries int, retryDelay time.Duration) ([]string, error) {
	result, err := rc.ExecuteWithRetry(func() (any, error) {
		return rc.SPopN(key, count)
	}, maxRetries, retryDelay)
	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

// SMembersWithRetry 获取集合所有成员（带自动重连重试）。
func (rc *RedisClient) SMembersWithRetry(key string, maxRetries int, retryDelay time.Duration) ([]string, error) {
	result, err := rc.ExecuteWithRetry(func() (any, error) {
		return rc.SMembers(key)
	}, maxRetries, retryDelay)
	if err != nil {
		return nil, err
	}
	return result.([]string), nil
}

// SAddWithRetry 向集合添加成员（带自动重连重试）。
func (rc *RedisClient) SAddWithRetry(key string, maxRetries int, retryDelay time.Duration, members ...any) (int64, error) {
	result, err := rc.ExecuteWithRetry(func() (any, error) {
		return rc.SAdd(key, members...)
	}, maxRetries, retryDelay)
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

// SRemWithRetry 从集合中移除成员（带自动重连重试）。
func (rc *RedisClient) SRemWithRetry(key string, maxRetries int, retryDelay time.Duration, members ...any) (int64, error) {
	result, err := rc.ExecuteWithRetry(func() (any, error) {
		return rc.SRem(key, members...)
	}, maxRetries, retryDelay)
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

// SCardWithRetry 获取集合成员数量（带自动重连重试）。
func (rc *RedisClient) SCardWithRetry(key string, maxRetries int, retryDelay time.Duration) (int64, error) {
	result, err := rc.ExecuteWithRetry(func() (any, error) {
		return rc.SCard(key)
	}, maxRetries, retryDelay)
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

// HGetAllWithRetry 获取哈希所有字段和值（带自动重连重试）。
func (rc *RedisClient) HGetAllWithRetry(key string, maxRetries int, retryDelay time.Duration) (map[string]string, error) {
	result, err := rc.ExecuteWithRetry(func() (any, error) {
		return rc.HGetAll(key)
	}, maxRetries, retryDelay)
	if err != nil {
		return nil, err
	}
	return result.(map[string]string), nil
}

// IncrWithRetry 将 key 对应的值加 1（带自动重连重试）。
func (rc *RedisClient) IncrWithRetry(key string, maxRetries int, retryDelay time.Duration) (int64, error) {
	result, err := rc.ExecuteWithRetry(func() (any, error) {
		return rc.Incr(key)
	}, maxRetries, retryDelay)
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

// IncrByWithRetry 将 key 对应的值加上指定增量（带自动重连重试）。
func (rc *RedisClient) IncrByWithRetry(key string, value int64, maxRetries int, retryDelay time.Duration) (int64, error) {
	result, err := rc.ExecuteWithRetry(func() (any, error) {
		return rc.IncrBy(key, value)
	}, maxRetries, retryDelay)
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}
