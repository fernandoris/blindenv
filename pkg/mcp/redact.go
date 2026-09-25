package mcp

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"
)

// RedactionTokenFormat is the marker substituted for a secret value.
const RedactionTokenFormat = "[BLINDENV_REDACTED:%s]"

// Redactor replaces secret values in text with named markers.
type Redactor struct {
	entries []redactionEntry
	minLen  int
}

type redactionEntry struct {
	key   string
	value string
}

// NewRedactor builds a Redactor from key/value pairs. Values shorter than
// minLen (or empty) are ignored because they cannot be redacted reliably.
func NewRedactor(values map[string]string, minLen int) *Redactor {
	entries := make([]redactionEntry, 0, len(values))
	for key, value := range values {
		if len(value) < minLen {
			continue
		}
		entries = append(entries, redactionEntry{key: key, value: value})
	}
	// Longest values first so a shorter value that is a prefix of a longer
	// one does not leave remnants of the longer value behind.
	sort.Slice(entries, func(i, j int) bool {
		if len(entries[i].value) != len(entries[j].value) {
			return len(entries[i].value) > len(entries[j].value)
		}
		return entries[i].key < entries[j].key
	})
	return &Redactor{entries: entries, minLen: minLen}
}

// Redact normalizes input to UTF-8 and replaces every literal secret value
// with its marker. It returns the redacted text and the number of
// substitutions performed.
func (r *Redactor) Redact(input []byte) (string, int) {
	text := NormalizeUTF8(input)
	count := 0
	for _, e := range r.entries {
		token := fmt.Sprintf(RedactionTokenFormat, e.key)
		if n := strings.Count(text, e.value); n > 0 {
			text = strings.ReplaceAll(text, e.value, token)
			count += n
		}
	}
	return text, count
}

// RedactString redacts an already-decoded string value.
func (r *Redactor) RedactString(text string) (string, int) {
	return r.Redact([]byte(text))
}

// MaxValueLen returns the length of the longest redactable value, or zero when
// no value is long enough to redact. Entries are sorted longest-first.
func (r *Redactor) MaxValueLen() int {
	if len(r.entries) == 0 {
		return 0
	}
	return len(r.entries[0].value)
}

// utf16HeuristicLimit bounds the speculative BOM-less UTF-16 heuristic so a
// large buffer is never decoded as UTF-16 and amplified in memory.
const utf16HeuristicLimit = 1 << 20

// NormalizeUTF8 decodes input to a UTF-8 string, honoring a UTF-8 or UTF-16
// byte order mark and a heuristic for BOM-less UTF-16LE output (common with
// Windows PowerShell 5.1).
func NormalizeUTF8(input []byte) string {
	switch {
	case len(input) >= 3 && input[0] == 0xEF && input[1] == 0xBB && input[2] == 0xBF:
		return string(input[3:])
	case len(input) >= 2 && input[0] == 0xFF && input[1] == 0xFE:
		return decodeUTF16(input[2:], binary.LittleEndian)
	case len(input) >= 2 && input[0] == 0xFE && input[1] == 0xFF:
		return decodeUTF16(input[2:], binary.BigEndian)
	case len(input) <= utf16HeuristicLimit && looksUTF16LE(input):
		return decodeUTF16(input, binary.LittleEndian)
	default:
		return string(input)
	}
}

func decodeUTF16(b []byte, order binary.ByteOrder) string {
	if len(b)%2 == 1 {
		b = b[:len(b)-1]
	}
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = order.Uint16(b[i*2:])
	}
	return string(utf16.Decode(units))
}

func looksUTF16LE(b []byte) bool {
	if len(b) < 4 || len(b)%2 != 0 {
		return false
	}
	pairs := len(b) / 2
	zeros := 0
	for i := 0; i+1 < len(b); i += 2 {
		if b[i+1] == 0 {
			zeros++
		}
	}
	return zeros*100/pairs >= 30
}
