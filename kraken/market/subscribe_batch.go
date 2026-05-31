package market

import (
	"github.com/theapemachine/symm/config"
)

/*
symbolBatches splits symbols into subscribe batches sized by config.System.SubscribeBatch.
*/
func symbolBatches(symbols []string) [][]string {
	batchSize := config.System.SubscribeBatch

	if batchSize <= 0 || len(symbols) <= batchSize {
		if len(symbols) == 0 {
			return nil
		}

		return [][]string{symbols}
	}

	batches := make([][]string, 0, (len(symbols)+batchSize-1)/batchSize)

	for start := 0; start < len(symbols); start += batchSize {
		end := start + batchSize

		if end > len(symbols) {
			end = len(symbols)
		}

		batches = append(batches, symbols[start:end])
	}

	return batches
}

/*
LimitSymbols caps a discovered universe to max when max > 0.
*/
func LimitSymbols(symbols []string, max int) []string {
	if max <= 0 || len(symbols) <= max {
		return symbols
	}

	capped := make([]string, max)
	copy(capped, symbols[:max])

	return capped
}
