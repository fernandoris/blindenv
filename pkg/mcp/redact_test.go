package mcp

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestRedactorBasic(t *testing.T) {
	r := NewRedactor(map[string]string{"API_KEY": "sk-1234567890"}, 6)
	got, count := r.RedactString("token=sk-1234567890 done")
	want := "token=[BLINDENV_REDACTED:API_KEY] done"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestRedactorCountsAllOccurrences(t *testing.T) {
	r := NewRedactor(map[string]string{"TOKEN": "abcdef123456"}, 6)
	_, count := r.RedactString("abcdef123456 and abcdef123456")
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}

func TestRedactorSkipsShortValues(t *testing.T) {
	r := NewRedactor(map[string]string{"SHORT": "dev"}, 6)
	got, count := r.RedactString("environment dev")
	if got != "environment dev" || count != 0 {
		t.Fatalf("got %q count %d, want unchanged", got, count)
	}
}

func TestRedactorLongestFirst(t *testing.T) {
	r := NewRedactor(map[string]string{
		"LONG":   "secretvalue",
		"PREFIX": "secret",
	}, 6)
	got, _ := r.RedactString("the secretvalue here")
	if strings.Contains(got, "secret") || strings.Contains(got, "value") {
		t.Fatalf("residual secret in %q", got)
	}
	if !strings.Contains(got, "[BLINDENV_REDACTED:LONG]") {
		t.Fatalf("expected LONG marker in %q", got)
	}
}

func TestNormalizeUTF8BOM(t *testing.T) {
	if got := NormalizeUTF8([]byte{0xEF, 0xBB, 0xBF, 'h', 'i'}); got != "hi" {
		t.Fatalf("got %q, want hi", got)
	}
}

func TestRedactorUTF16LE(t *testing.T) {
	const secret = "sk-utf16-secret"
	text := "value=" + secret
	r := NewRedactor(map[string]string{"API_KEY": secret}, 6)
	_, count := r.Redact(encodeUTF16LE(text))
	if count != 1 {
		t.Fatalf("count = %d, want 1 (UTF-16 not normalized)", count)
	}
}

func encodeUTF16LE(s string) []byte {
	units := utf16.Encode([]rune(s))
	buf := make([]byte, 0, len(units)*2)
	for _, u := range units {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], u)
		buf = append(buf, b[:]...)
	}
	return buf
}
