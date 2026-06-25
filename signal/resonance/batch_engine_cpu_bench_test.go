package resonance

import (
	"fmt"
	"testing"

	"github.com/theapemachine/nomagique/learning"
	"golang.org/x/sync/errgroup"
)

func benchmarkCPUParallelSettleEntries(b *testing.B, entries []batchEntry, arch []int, alpha float64) {
	manifolds := make([]*learning.ResonanceManifold, len(entries))

	for index := range manifolds {
		manifold, err := learning.NewResonanceManifold(arch, 0, alpha)

		if err != nil {
			b.Fatal(err)
		}

		manifolds[index] = manifold
	}

	outcomes := make([]settleOutcome, len(entries))

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		waitGroup := errgroup.Group{}

		for entryIndex, entry := range entries {
			entryIndex := entryIndex
			entry := entry

			waitGroup.Go(func() error {
				manifold := manifolds[entryIndex]

				if settleErr := manifold.Settle(entry.input, true); settleErr != nil {
					return settleErr
				}

				manifold.Learn(nil)

				outcomes[entryIndex] = settleOutcome{
					symbol:   entry.symbol,
					input:    entry.input,
					latent:   manifold.LatentState(),
					surprise: manifold.ReconstructionError(),
					energy:   manifold.Energy(),
				}

				return nil
			})
		}

		if waitErr := waitGroup.Wait(); waitErr != nil {
			b.Fatal(waitErr)
		}
	}
}

func BenchmarkCPUParallelManifoldSettle(b *testing.B) {
	arch := DefaultArchitecture(8)
	alpha := 0.01
	entries := make([]batchEntry, 128)

	for index := range entries {
		entries[index] = batchEntry{
			slot:   index,
			symbol: fmt.Sprintf("SYM%d/USD", index),
			input:  make([]float64, SensoryChannelCount),
		}

		for channel := range entries[index].input {
			entries[index].input[channel] = 0.1 + float64(index)*0.001 + float64(channel)*0.01
		}
	}

	benchmarkCPUParallelSettleEntries(b, entries, arch, alpha)
}
