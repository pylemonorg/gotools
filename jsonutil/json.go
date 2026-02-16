package jsonutil

import (
	"encoding/json"
	"os"

	"github.com/pylemonorg/gotools/logger"
)

// Marshal 将任意值序列化为 JSON 字节切片。
// 对 json.Marshal 的简单封装，统一错误格式。
func Marshal(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, logger.ErrorfE("jsonutil: marshal 失败: %v", err)
	}
	return data, nil
}

// MarshalString 将任意值序列化为 JSON 字符串。
func MarshalString(v any) (string, error) {
	data, err := Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MarshalIndent 将任意值序列化为带缩进的 JSON 字节切片（便于调试/日志）。
func MarshalIndent(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, logger.ErrorfE("jsonutil: marshal indent 失败: %v", err)
	}
	return data, nil
}

// MarshalIndentString 将任意值序列化为带缩进的 JSON 字符串。
func MarshalIndentString(v any) (string, error) {
	data, err := MarshalIndent(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MustMarshal 将任意值序列化为 JSON 字节切片，失败时记录错误日志并返回 nil。
// 适用于确信不会失败的场景（如序列化 map[string]string），省去 if err 判断。
func MustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		logger.Errorf("jsonutil: MustMarshal 失败: %v", err)
		return nil
	}
	return data
}

// MustMarshalString 将任意值序列化为 JSON 字符串，失败时记录错误日志并返回空串。
func MustMarshalString(v any) string {
	return string(MustMarshal(v))
}

// Unmarshal 将 JSON 字节切片反序列化到目标对象。
func Unmarshal(data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return logger.ErrorfE("jsonutil: unmarshal 失败: %v", err)
	}
	return nil
}

// UnmarshalString 将 JSON 字符串反序列化到目标对象。
func UnmarshalString(s string, v any) error {
	return Unmarshal([]byte(s), v)
}

// ToMap 将 JSON 字节切片反序列化为 map[string]any。
// 适用于不想定义结构体、只需快速访问字段的场景。
func ToMap(data []byte) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, logger.ErrorfE("jsonutil: 解析为 map 失败: %v", err)
	}
	return m, nil
}

// ToMapFromString 将 JSON 字符串反序列化为 map[string]any。
func ToMapFromString(s string) (map[string]any, error) {
	return ToMap([]byte(s))
}

// ReadFile 读取 JSON 文件并反序列化到目标对象。
func ReadFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return logger.ErrorfE("jsonutil: 读取文件 [%s] 失败: %v", path, err)
	}
	if err = json.Unmarshal(data, v); err != nil {
		return logger.ErrorfE("jsonutil: 解析文件 [%s] 失败: %v", path, err)
	}
	return nil
}

// WriteFile 将任意值序列化为带缩进的 JSON 并写入文件。
// 文件权限为 0644，已存在则覆盖。
func WriteFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return logger.ErrorfE("jsonutil: 序列化失败: %v", err)
	}
	// 追加换行符，符合 POSIX 文件规范
	data = append(data, '\n')
	if err = os.WriteFile(path, data, 0644); err != nil {
		return logger.ErrorfE("jsonutil: 写入文件 [%s] 失败: %v", path, err)
	}
	return nil
}

// IsValid 检查字节切片是否为合法的 JSON。
func IsValid(data []byte) bool {
	return json.Valid(data)
}

// IsValidString 检查字符串是否为合法的 JSON。
func IsValidString(s string) bool {
	return json.Valid([]byte(s))
}

// GetString 从 map[string]any 中安全取出 string 类型的值。
// key 不存在或类型不匹配时返回空串。
func GetString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// GetInt 从 map[string]any 中安全取出整数值。
// JSON 数字默认反序列化为 float64，此函数自动处理转换。
// key 不存在或类型不匹配时返回 0。
func GetInt(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0
		}
		return int(i)
	default:
		return 0
	}
}

// GetFloat64 从 map[string]any 中安全取出 float64 值。
// key 不存在或类型不匹配时返回 0。
func GetFloat64(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

// GetBool 从 map[string]any 中安全取出 bool 值。
// key 不存在或类型不匹配时返回 false。
func GetBool(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}
