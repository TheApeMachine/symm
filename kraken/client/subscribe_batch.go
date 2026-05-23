package client

import "fmt"

const defaultSubscribeBatch = 50

/*
SubscribeSymbolsBatched sends one subscribe frame per symbol chunk.
Kraken rejects or drops single frames listing hundreds of symbols.
*/
func SubscribeSymbolsBatched(
	publicClient *PublicClient,
	symbols []string,
	batchSize int,
	build func([]string) any,
) error {
	if publicClient == nil {
		return fmt.Errorf("public websocket client is nil")
	}

	if len(symbols) == 0 {
		return fmt.Errorf("subscribe requires at least one symbol")
	}

	if batchSize <= 0 {
		batchSize = defaultSubscribeBatch
	}

	for start := 0; start < len(symbols); start += batchSize {
		end := start + batchSize

		if end > len(symbols) {
			end = len(symbols)
		}

		chunk := symbols[start:end]

		if err := publicClient.SubscribeTo(build(chunk)); err != nil {
			return fmt.Errorf("subscribe symbols [%d:%d]: %w", start, end, err)
		}
	}

	return nil
}
