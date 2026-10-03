package mcp

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkTranslateOneTag(b *testing.B) {
	keys := keySet(map[string]string{"API_KEY": "sk-abcdef123456"})
	cmd := `curl --header 'Content-Type: application/json' --data '{"token":"x"}' https://api.example.com/v1/{{API_KEY}}`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, _, _, err := translateSecretTags(shellPOSIX, cmd, keys); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTranslateTagCount(b *testing.B) {
	keys := keySet(map[string]string{"API_KEY": "sk-abcdef123456"})
	for _, n := range []int{1, 10, 100} {
		cmd := strings.Repeat("echo {{API_KEY}} ", n)
		b.Run(fmt.Sprintf("tags=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, _, _, err := translateSecretTags(shellPOSIX, cmd, keys); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkTranslateManyKeysNoTags(b *testing.B) {
	keys := manyKeys(100)
	cmd := strings.Repeat("echo hello ", 100)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, _, _, err := translateSecretTags(shellPOSIX, cmd, keys); err != nil {
			b.Fatal(err)
		}
	}
}
