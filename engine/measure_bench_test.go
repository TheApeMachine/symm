package engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/asset"
)

func BenchmarkMeasureSymbols_serial(b *testing.B) {
	ctx := context.Background()

	scanner := SymbolScanner{
		Source:  "bench",
		Symbols: benchSymbols(32),
		Pairs:   benchPairs(32),
	}

	now := time.Now().UTC()
	evaluate := benchEvaluate

	b.ResetTimer()

	for b.Loop() {
		for measurement := range MeasureSymbols(ctx, scanner, now, evaluate) {
			_ = measurement
		}
	}
}

func BenchmarkMeasureSymbols_parallel(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 8, qpool.NewConfig())
	b.Cleanup(func() { pool.Close() })

	scanner := SymbolScanner{
		Source:  "bench",
		Symbols: benchSymbols(32),
		Pairs:   benchPairs(32),
		Pool:    pool,
	}

	now := time.Now().UTC()
	evaluate := benchEvaluate

	b.ResetTimer()

	for b.Loop() {
		for measurement := range MeasureSymbols(ctx, scanner, now, evaluate) {
			_ = measurement
		}
	}
}

func benchSymbols(count int) []string {
	symbols := make([]string, count)

	for index := range count {
		symbols[index] = fmt.Sprintf("SYM%d/EUR", index)
	}

	return symbols
}

func benchPairs(count int) map[string]asset.Pair {
	pairs := make(map[string]asset.Pair, count)

	for index := range count {
		name := fmt.Sprintf("SYM%d/EUR", index)
		pairs[name] = asset.Pair{Wsname: name}
	}

	return pairs
}

func benchEvaluate(_ string, _ Snapshot) (Measurement, bool, error) {
	return Measurement{Confidence: 0.5}, true, nil
}
