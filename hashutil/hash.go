package hashutil

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/cespare/xxhash/v2"
)

// MD5 返回输入字符串的 MD5 十六进制摘要。
func MD5(input string) (string, error) {
	h := md5.New()
	if _, err := h.Write([]byte(input)); err != nil {
		return "", fmt.Errorf("hashutil: md5 write: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// SHA256 返回输入字符串的 SHA-256 十六进制摘要。
func SHA256(input string) (string, error) {
	h := sha256.New()
	if _, err := h.Write([]byte(input)); err != nil {
		return "", fmt.Errorf("hashutil: sha256 write: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// BucketKey 使用 xxhash 生成一致性分桶 key。
// 格式："{prefix}_{xxhash(value) % buckets}"。
// xxhash 比 MD5 快 5-10 倍，适合大量 key 的分桶场景。
func BucketKey(prefix, value string, buckets uint64) string {
	n := xxhash.Sum64String(value)
	return fmt.Sprintf("%s_%d", prefix, n%buckets)
}

// RandomString 基于纳秒时间戳的 xxhash 生成指定长度的随机十六进制字符串。
// 注意：不适用于安全场景，仅用于生成唯一标识。
func RandomString(length int) string {
	hash := fmt.Sprintf("%x", xxhash.Sum64String(fmt.Sprintf("%d", time.Now().UnixNano())))
	if len(hash) >= length {
		return hash[:length]
	}
	return hash
}
