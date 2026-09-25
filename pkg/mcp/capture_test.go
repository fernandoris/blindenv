package mcp

import (
	"bytes"
	"strings"
	"testing"
)

func TestBoundedWriterRetainsAndDrains(t *testing.T) {
	w := newBoundedWriter(100)
	n, err := w.Write(bytes.Repeat([]byte("x"), 1000))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 1000 {
		t.Fatalf("Write returned %d, want 1000 (must drain)", n)
	}
	if got := len(w.Bytes()); got != 100 {
		t.Fatalf("retained = %d, want 100", got)
	}
	if w.Total() != 1000 {
		t.Fatalf("total = %d, want 1000", w.Total())
	}
	if !w.Truncated() {
		t.Fatal("expected Truncated to be true")
	}
}

func TestStreamRedactorSplitAcrossWrites(t *testing.T) {
	r := NewRedactor(map[string]string{"API_KEY": "sk-abcdef123456"}, 6)
	var buf bytes.Buffer
	s := NewStreamRedactor(&buf, r)
	if _, err := s.Write([]byte("prefix sk-abc")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := s.Write([]byte("def123456 suffix")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "sk-abc") || strings.Contains(out, "abcdef123456") {
		t.Fatalf("secret fragment leaked: %q", out)
	}
	if !strings.Contains(out, "[BLINDENV_REDACTED:API_KEY]") {
		t.Fatalf("expected redaction marker in %q", out)
	}
	if s.Count() != 1 {
		t.Fatalf("count = %d, want 1", s.Count())
	}
}

func TestRedactingWriterNormalizesSplitUTF16(t *testing.T) {
	r := NewRedactor(map[string]string{"API_KEY": "sk-abcdef123456"}, 6)
	var buf bytes.Buffer
	w := NewRedactingWriter(&buf, r)
	raw := encodeUTF16LE("value=sk-abcdef123456 end")
	if _, err := w.Write(raw[:len(raw)/2]); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := w.Write(raw[len(raw)/2:]); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "sk-abcdef123456") {
		t.Fatalf("secret leaked after UTF-16 normalization: %q", out)
	}
	if !strings.Contains(out, "[BLINDENV_REDACTED:API_KEY]") {
		t.Fatalf("expected redaction marker in %q", out)
	}
}

func TestNormalizeUTF8DoesNotDecodeLargeNonUTF16(t *testing.T) {
	big := bytes.Repeat([]byte("a\x00"), (2<<20)/2)
	if got := NormalizeUTF8(big); len(got) != len(big) {
		t.Fatalf("large buffer was decoded: got %d bytes, want %d", len(got), len(big))
	}
}

func TestRedactBoundedPrefixAllocs(t *testing.T) {
	r := NewRedactor(map[string]string{
		"API_KEY": "sk-abcdef123456",
		"TOKEN":   "tok-abcdefghij",
	}, 6)
	prefix := bytes.Repeat([]byte("filler line with some text\n"), 4000)
	allocs := testing.AllocsPerRun(5, func() {
		_, _ = r.Redact(prefix)
	})
	if allocs > 100 {
		t.Fatalf("Redact allocations = %.1f, want bounded (<100)", allocs)
	}
}
