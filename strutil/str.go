package strutil

import (
	"encoding/base64"
	"strings"
)

// Strip 去除字符串两端的字符。
// 不传 chars 时等同于 strings.TrimSpace；传入 chars 时去除指定字符。
func Strip(s string, chars ...string) string {
	if len(chars) == 0 || chars[0] == "" {
		return strings.TrimSpace(s)
	}
	return strings.Trim(s, chars[0])
}

// Base64Encode 对字符串进行标准 Base64 编码。
func Base64Encode(input string) string {
	return base64.StdEncoding.EncodeToString([]byte(input))
}

// Base64Decode 对标准 Base64 编码的字符串进行解码。
func Base64Decode(input string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// Base64RawURLEncode 使用 RawURLEncoding 对字符串进行 Base64 编码（无填充、URL 安全）。
func Base64RawURLEncode(input string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(input))
}

// Base64RawURLDecode 对 RawURLEncoding 编码的 Base64 字符串进行解码。
func Base64RawURLDecode(input string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
