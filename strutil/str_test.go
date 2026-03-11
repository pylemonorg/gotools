package strutil

import "testing"

func TestBase64(t *testing.T) {
	input := "www.baidu.com"
	encoded := Base64Encode(input)
	t.Logf("Base64Encode: %s", encoded)

	decoded, err := Base64Decode(encoded)
	if err != nil {
		t.Fatalf("Base64Decode: %v", err)
	}
	if decoded != input {
		t.Errorf("Base64Decode = %q, want %q", decoded, input)
	}
}

func TestBase64RawURL(t *testing.T) {
	input := "https://www.baidu.com/s?wd=go&ie=utf-8"
	encoded := Base64RawURLEncode(input)
	t.Logf("Base64RawURLEncode: %s", encoded)

	decoded, err := Base64RawURLDecode(encoded)
	if err != nil {
		t.Fatalf("Base64RawURLDecode: %v", err)
	}
	if decoded != input {
		t.Errorf("Base64RawURLDecode = %q, want %q", decoded, input)
	}
}

func TestStrip(t *testing.T) {
	tests := []struct {
		input string
		chars []string
		want  string
	}{
		{"  hello  ", nil, "hello"},
		{"##hello##", []string{"#"}, "hello"},
		{"\t hello \n", nil, "hello"},
	}
	for _, tt := range tests {
		got := Strip(tt.input, tt.chars...)
		if got != tt.want {
			t.Errorf("Strip(%q, %v) = %q, want %q", tt.input, tt.chars, got, tt.want)
		}
	}
}
