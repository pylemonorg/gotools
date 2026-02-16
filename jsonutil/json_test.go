package jsonutil

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Marshal / Unmarshal
// ---------------------------------------------------------------------------

func TestMarshalString(t *testing.T) {
	m := map[string]string{"name": "go", "version": "1.22"}
	s, err := MarshalString(m)
	if err != nil {
		t.Fatalf("MarshalString: %v", err)
	}
	if s == "" {
		t.Fatal("MarshalString returned empty string")
	}
	t.Logf("MarshalString: %s", s)
}

func TestMustMarshalString(t *testing.T) {
	s := MustMarshalString(map[string]int{"a": 1, "b": 2})
	if s == "" {
		t.Fatal("MustMarshalString returned empty string")
	}
	t.Logf("MustMarshalString: %s", s)
}

func TestUnmarshalString(t *testing.T) {
	type User struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	var u User
	if err := UnmarshalString(`{"name":"alice","age":30}`, &u); err != nil {
		t.Fatalf("UnmarshalString: %v", err)
	}
	if u.Name != "alice" || u.Age != 30 {
		t.Errorf("unexpected result: %+v", u)
	}
}

func TestUnmarshalInvalid(t *testing.T) {
	var m map[string]any
	if err := UnmarshalString("not json", &m); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ---------------------------------------------------------------------------
// ToMap / Get*
// ---------------------------------------------------------------------------

func TestToMapAndGetters(t *testing.T) {
	raw := `{"name":"bob","age":25,"score":99.5,"active":true}`
	m, err := ToMapFromString(raw)
	if err != nil {
		t.Fatalf("ToMapFromString: %v", err)
	}

	if got := GetString(m, "name"); got != "bob" {
		t.Errorf("GetString(name) = %q, want %q", got, "bob")
	}
	if got := GetString(m, "missing"); got != "" {
		t.Errorf("GetString(missing) = %q, want empty", got)
	}
	if got := GetInt(m, "age"); got != 25 {
		t.Errorf("GetInt(age) = %d, want 25", got)
	}
	if got := GetFloat64(m, "score"); got != 99.5 {
		t.Errorf("GetFloat64(score) = %f, want 99.5", got)
	}
	if got := GetBool(m, "active"); !got {
		t.Error("GetBool(active) = false, want true")
	}
	if got := GetBool(m, "missing"); got {
		t.Error("GetBool(missing) = true, want false")
	}
}

// ---------------------------------------------------------------------------
// IsValid
// ---------------------------------------------------------------------------

func TestIsValid(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{`{"key":"value"}`, true},
		{`[1,2,3]`, true},
		{`"hello"`, true},
		{`not json`, false},
		{`{broken`, false},
	}
	for _, tt := range tests {
		if got := IsValidString(tt.input); got != tt.want {
			t.Errorf("IsValidString(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ReadFile / WriteFile
// ---------------------------------------------------------------------------

func TestReadWriteFile(t *testing.T) {
	type Config struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// 写入
	original := Config{Host: "localhost", Port: 8080}
	if err := WriteFile(path, original); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// 读取
	var loaded Config
	if err := ReadFile(path, &loaded); err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if loaded != original {
		t.Errorf("ReadFile got %+v, want %+v", loaded, original)
	}

	// 读取不存在的文件
	if err := ReadFile(filepath.Join(dir, "nope.json"), &loaded); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("not json"), 0644)

	var m map[string]any
	if err := ReadFile(path, &m); err == nil {
		t.Fatal("expected error for invalid JSON file")
	}
}

// ---------------------------------------------------------------------------
// MarshalIndent
// ---------------------------------------------------------------------------

func TestMarshalIndentString(t *testing.T) {
	m := map[string]any{"key": "value"}
	s, err := MarshalIndentString(m)
	if err != nil {
		t.Fatalf("MarshalIndentString: %v", err)
	}
	if s == "" {
		t.Fatal("MarshalIndentString returned empty string")
	}
	t.Logf("MarshalIndentString:\n%s", s)
}
