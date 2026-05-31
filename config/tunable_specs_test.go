package config

import "testing"

func BenchmarkMutateTunables(b *testing.B) {
	base := NewConfig()

	for b.Loop() {
		_ = MutateTunables(base, nil)
	}
}
