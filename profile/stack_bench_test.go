package profile

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
)

func BenchmarkProfileStack(b *testing.B) {
	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()

	pool := qpool.NewQ(ctx, 1, DefaultWorkerCount(), qpool.NewConfig())
	b.Cleanup(func() {
		cancel()
		pool.Close()
	})

	qpool.SuppressLogging()

	stack, err := NewStack(ctx, pool)

	if err != nil {
		b.Fatalf("new stack: %v", err)
	}

	b.Cleanup(func() { stack.Close() })

	stack.StartTicks()

	lines, err := LoadLines(16, 8)

	if err != nil {
		b.Fatalf("load lines: %v", err)
	}

	// Prime instrument catalog and symbol routing.
	for _, line := range lines[:min(64, len(lines))] {
		stack.PublicClient.IngestReplayLine(line)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, line := range lines {
			stack.PublicClient.IngestReplayLine(line)
		}
	}
}
