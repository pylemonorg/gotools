package htmlutil

import (
	"io"
	"strings"

	"github.com/saintfish/chardet"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
)

const defaultChardetConfidence = 50

// DecodeResult HTML 编码检测与解码结果。
type DecodeResult struct {
	Text     string // 解码后的 UTF-8 文本
	Encoding string // 检测到的编码名称，如 "utf-8"、"gbk"
	Certain  bool   // 编码是否确定（来源于明确声明或高置信度检测）
}

// Decode 将 HTML 字节解码为 UTF-8 文本（标准模式）。
// 通过 Content-Type 头和 HTML meta 标签检测编码。
// 适用于大多数有编码声明的网页。
func Decode(raw []byte, contentType string) *DecodeResult {
	enc, name, certain := charset.DetermineEncoding(raw, contentType)
	return &DecodeResult{
		Text:     decodeBytes(raw, enc),
		Encoding: name,
		Certain:  certain,
	}
}

// DecodeSmart 将 HTML 字节解码为 UTF-8 文本（增强模式）。
// 标准检测不确定时，使用 chardet 统计分析做二次检测。
// 适用于通用采集场景，能处理无编码声明的老旧网页。
func DecodeSmart(raw []byte, contentType string) *DecodeResult {
	enc, name, certain := charset.DetermineEncoding(raw, contentType)
	if certain {
		return &DecodeResult{Text: decodeBytes(raw, enc), Encoding: name, Certain: true}
	}

	detector := chardet.NewTextDetector()
	best, err := detector.DetectBest(raw)
	if err == nil && best.Confidence > defaultChardetConfidence {
		if detected, dname := resolveEncoding(best.Charset); detected != nil {
			return &DecodeResult{
				Text:     decodeBytes(raw, detected),
				Encoding: dname,
				Certain:  best.Confidence > 80,
			}
		}
	}

	return &DecodeResult{Text: decodeBytes(raw, enc), Encoding: name, Certain: false}
}

// NewReader 包装 io.Reader，自动检测编码并转换为 UTF-8 输出。
// 内部通过 Content-Type 头和 HTML 内容预扫描确定编码。
// 适用于流式读取场景，不需要将全部内容加载到内存。
func NewReader(r io.Reader, contentType string) (io.Reader, error) {
	return charset.NewReader(r, contentType)
}

// resolveEncoding 根据编码名称查找对应的 encoding.Encoding。
// 兼容 chardet 返回的名称格式（如 "GB-18030"）。
func resolveEncoding(name string) (encoding.Encoding, string) {
	if enc, err := htmlindex.Get(name); err == nil {
		canonical, _ := htmlindex.Name(enc)
		return enc, canonical
	}
	// chardet 可能返回带连字符的名称（如 "GB-18030"），htmlindex 需要 "gb18030"
	normalized := strings.ReplaceAll(name, "-", "")
	if enc, err := htmlindex.Get(normalized); err == nil {
		canonical, _ := htmlindex.Name(enc)
		return enc, canonical
	}
	return nil, ""
}

func decodeBytes(raw []byte, enc encoding.Encoding) string {
	if enc == nil || enc == encoding.Nop {
		return string(raw)
	}
	decoded, _, err := transform.Bytes(enc.NewDecoder(), raw)
	if err != nil {
		return string(raw)
	}
	return string(decoded)
}
