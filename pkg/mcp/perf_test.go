package mcp

import (
	"fmt"
	"strings"
	"testing"
)

// BenchmarkRedactorMixedSensitivity builds the redactor from a sensitive-only
// map and substitutes over a payload that also contains a configuration value,
// which must be left untouched.
func BenchmarkRedactorMixedSensitivity(b *testing.B) {
	sensitive := make(map[string]string, 100)
	for i := 0; i < 100; i++ {
		sensitive[fmt.Sprintf("SECRET_%03d", i)] = fmt.Sprintf("secret-value-%03d-abcdef", i)
	}
	payload := strings.Repeat("output with secret-value-042-abcdef and https://example.test/0042\n", 50)
	r := NewRedactor(sensitive, 6)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.RedactString(payload)
	}
}
