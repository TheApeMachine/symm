package cooccurrence

import (
	"testing"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/replay"
)

func BenchmarkChainReachabilityScoreCached(b *testing.B) {
	rows := make([]perspectives.Measurement, 0, 512)

	for tickIndex := range 512 {
		rows = append(rows, perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      float64(tickIndex%8 + 1),
			Last:     100 + float64(tickIndex),
		})

		if tickIndex%3 == 0 {
			rows = append(rows, perspectives.Measurement{
				Symbol:   "BTC/EUR",
				Source:   perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      float64(tickIndex%5 + 1),
				Last:     100 + float64(tickIndex),
			})
		}
	}

	index := NewCoOccurrenceIndex(replay.PrecompileTape(rows), 0)
	chains := [][]perspectives.CategoryType{
		{perspectives.CategoryLaminar, perspectives.CategoryExhaustion},
		{perspectives.CategoryLaminar, perspectives.CategoryToxicBluff},
		{perspectives.CategoryLaminar, perspectives.CategoryExhaustion, perspectives.CategoryTurbulent},
	}

	b.ResetTimer()

	for b.Loop() {
		for _, chain := range chains {
			_ = index.ChainReachabilityScore(chain)
		}
	}
}
