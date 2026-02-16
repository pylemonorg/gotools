package urlutil

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/pylemonorg/gotools/hashutil"
)

// Resolve 基于已解析的 base URL，将相对 URL 转为绝对 URL。
// 仅返回以 http/https 开头的结果，否则报错。
func Resolve(base *url.URL, relativeURL string) (string, error) {
	abs, err := base.Parse(relativeURL)
	if err != nil {
		return "", fmt.Errorf("urlutil: 解析相对 URL 失败: %w", err)
	}
	href := abs.String()
	if strings.HasPrefix(href, "http") {
		return href, nil
	}
	return "", fmt.Errorf("urlutil: 无法解析为绝对 URL: %s", relativeURL)
}

// ResolveStr 将 baseURL 字符串解析后，将相对 URL 转为绝对 URL。
func ResolveStr(baseURL, relativeURL string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("urlutil: 解析 base URL 失败: %w", err)
	}
	return Resolve(base, relativeURL)
}

// normalizeHTTPS 将 http:// 统一替换为 https://。
func normalizeHTTPS(rawURL string) string {
	return strings.Replace(rawURL, "http://", "https://", 1)
}

// ToMD5 先将 URL 标准化为 https，再返回其 MD5 十六进制摘要。
func ToMD5(rawURL string) (string, error) {
	return hashutil.MD5(normalizeHTTPS(rawURL))
}

// ToSHA256 先将 URL 标准化为 https，再返回其 SHA-256 十六进制摘要。
func ToSHA256(rawURL string) (string, error) {
	return hashutil.SHA256(normalizeHTTPS(rawURL))
}
