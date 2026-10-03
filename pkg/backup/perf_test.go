package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/fernandoris/blindenv/pkg/crypto"
	"github.com/fernandoris/blindenv/pkg/db"
)

// BenchmarkExportImport measures a round trip over 100 definitions with mixed
// sensitivity, so the extra metadata map's cost is observable.
func BenchmarkExportImport(b *testing.B) {
	ctx := context.Background()
	key, err := crypto.NewSalt(crypto.KeySize)
	if err != nil {
		b.Fatalf("key: %v", err)
	}
	source, err := db.Open(filepath.Join(b.TempDir(), "source.db"), key)
	if err != nil {
		b.Fatalf("Open: %v", err)
	}
	defer source.Close()
	_, _ = source.CreateProject(ctx, "my-api")
	_, _ = source.CreateEnvironment(ctx, "my-api", "staging")
	for i := 0; i < 100; i++ {
		secret := i%2 == 0
		meta := db.SecretMeta{Sensitive: &secret}
		if !secret {
			meta.Kind = db.KindURL
		}
		if _, err := source.PutSecretMeta(ctx, "my-api", "staging", fmt.Sprintf("KEY_%03d", i), fmt.Sprintf("value-%03d-abcdef", i), meta); err != nil {
			b.Fatalf("PutSecretMeta: %v", err)
		}
	}
	blob, err := Export(ctx, source, "bench-pass")
	if err != nil {
		b.Fatalf("Export: %v", err)
	}

	b.Run("export", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := Export(ctx, source, "bench-pass"); err != nil {
				b.Fatalf("Export: %v", err)
			}
		}
	})

	b.Run("import", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			dir, err := os.MkdirTemp("", "blindenv-bench-")
			if err != nil {
				b.Fatalf("temp dir: %v", err)
			}
			target, err := db.Open(filepath.Join(dir, "target.db"), key)
			if err != nil {
				b.Fatalf("Open target: %v", err)
			}
			b.StartTimer()
			if err := Import(ctx, target, "bench-pass", blob); err != nil {
				b.Fatalf("Import: %v", err)
			}
			b.StopTimer()
			_ = target.Close()
			_ = os.RemoveAll(dir)
			b.StartTimer()
		}
	})
}
