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

// BenchmarkSubstituteURL measures the only non-trivial work open_in_browser
// does before handing the URL to the OS launcher: in-process {{KEY}}
// substitution. The launcher spawn itself is a fixed os/exec cost and is not
// meaningfully representable in a Go benchmark, so this bounds the
// BlindEnv-owned portion of the open path.
func BenchmarkSubstituteURL(b *testing.B) {
	secrets := map[string]string{
		"SSO_TOKEN": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abcdef",
		"CLIENT_ID": "9f8e7d6c5b4a",
		"REGION":    "eu-west-1",
	}
	url := "https://idp.example.test/authorize?client={{CLIENT_ID}}&token={{SSO_TOKEN}}&region={{REGION}}&redirect=/home"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = substitute(url, secrets)
	}
}

// BenchmarkSubstituteCount measures substitution with occurrence counting.
// Allocations should stay flat across input sizes (bounded by the number of
// matching keys, not the input length) so counting does not grow memory with
// the request.
func BenchmarkSubstituteCount(b *testing.B) {
	secrets := map[string]string{
		"SSO_TOKEN": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.abcdef",
		"CLIENT_ID": "9f8e7d6c5b4a",
		"REGION":    "eu-west-1",
	}
	for _, tc := range []struct {
		name string
		unit string
	}{
		{"matches", "https://idp.example.test/authorize?client={{CLIENT_ID}}&token={{SSO_TOKEN}}&region={{REGION}}&redirect=/home\n"},
		{"no_matches", "https://example.test/health?probe=ok&region=eu-west-1\n"},
	} {
		for _, n := range []int{1, 20, 200} {
			input := strings.Repeat(tc.unit, n)
			b.Run(fmt.Sprintf("%s/size=%d", tc.name, n), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					_, _ = substitute(input, secrets)
				}
			})
		}
	}
}
