package market

import (
	"github.com/theapemachine/symm/config"
)

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
