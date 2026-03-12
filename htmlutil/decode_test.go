package htmlutil

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// --------
// Decode 测试
// --------

func TestDecode_UTF8WithContentType(t *testing.T) {
	html := []byte(`<html><body>你好世界</body></html>`)
	result := Decode(html, "text/html; charset=utf-8")

	if result.Encoding != "utf-8" {
		t.Errorf("encoding: want utf-8, got %s", result.Encoding)
	}
	if !result.Certain {
		t.Error("certain: want true, got false")
	}
	if !strings.Contains(result.Text, "你好世界") {
		t.Errorf("text should contain '你好世界', got: %s", result.Text)
	}
}

func TestDecode_UTF8WithMeta(t *testing.T) {
	html := []byte(`<html><head><meta charset="utf-8"></head><body>你好世界</body></html>`)
	result := Decode(html, "text/html")

	if result.Encoding != "utf-8" {
		t.Errorf("encoding: want utf-8, got %s", result.Encoding)
	}
	if !strings.Contains(result.Text, "你好世界") {
		t.Errorf("text should contain '你好世界', got: %s", result.Text)
	}
}

func TestDecode_GBKWithContentType(t *testing.T) {
	raw := encodeGBK(t, `<html><body>你好世界</body></html>`)
	result := Decode(raw, "text/html; charset=gbk")

	if !result.Certain {
		t.Error("certain: want true, got false")
	}
	if !strings.Contains(result.Text, "你好世界") {
		t.Errorf("text should contain '你好世界', got: %s", result.Text)
	}
}

func TestDecode_GBKWithMeta(t *testing.T) {
	raw := encodeGBK(t, `<html><head><meta charset="gbk"></head><body>你好世界</body></html>`)
	result := Decode(raw, "text/html")

	if !strings.Contains(result.Text, "你好世界") {
		t.Errorf("text should contain '你好世界', got: %s", result.Text)
	}
}

// --------
// DecodeSmart 测试
// --------

func TestDecodeSmart_CertainEncoding(t *testing.T) {
	html := []byte(`<html><head><meta charset="utf-8"></head><body>你好世界</body></html>`)
	result := DecodeSmart(html, "text/html; charset=utf-8")

	if result.Encoding != "utf-8" {
		t.Errorf("encoding: want utf-8, got %s", result.Encoding)
	}
	if !result.Certain {
		t.Error("certain: want true, got false")
	}
}

func TestDecodeSmart_GBKWithoutDeclaration(t *testing.T) {
	// GBK 内容无任何编码声明 — 需要 chardet 统计检测
	content := `<html><body>` +
		strings.Repeat("中华人民共和国成立于一九四九年十月一日。北京是中国的首都，拥有悠久的历史和文化。", 10) +
		`</body></html>`
	raw := encodeGBK(t, content)
	result := DecodeSmart(raw, "text/html")

	if !strings.Contains(result.Text, "中华人民共和国") {
		t.Errorf("text should contain '中华人民共和国', got: %s", result.Text)
	}
}

func TestDecodeSmart_GBKWithContentType(t *testing.T) {
	raw := encodeGBK(t, `<html><body>你好世界</body></html>`)
	result := DecodeSmart(raw, "text/html; charset=gbk")

	if !result.Certain {
		t.Error("certain: want true, got false")
	}
	if !strings.Contains(result.Text, "你好世界") {
		t.Errorf("text should contain '你好世界', got: %s", result.Text)
	}
}

// --------
// NewReader 测试
// --------

func TestNewReader_GBK(t *testing.T) {
	raw := encodeGBK(t, `<html><body>你好世界</body></html>`)
	reader, err := NewReader(bytes.NewReader(raw), "text/html; charset=gbk")
	if err != nil {
		t.Fatalf("NewReader error: %v", err)
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if !strings.Contains(string(body), "你好世界") {
		t.Errorf("text should contain '你好世界', got: %s", string(body))
	}
}

func TestNewReader_UTF8(t *testing.T) {
	html := `<html><body>你好世界</body></html>`
	reader, err := NewReader(strings.NewReader(html), "text/html; charset=utf-8")
	if err != nil {
		t.Fatalf("NewReader error: %v", err)
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if !strings.Contains(string(body), "你好世界") {
		t.Errorf("text should contain '你好世界', got: %s", string(body))
	}
}

// --------
// helpers
// --------

func encodeGBK(t *testing.T, s string) []byte {
	t.Helper()
	encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(s))
	if err != nil {
		t.Fatalf("encode GBK: %v", err)
	}
	return encoded
}
