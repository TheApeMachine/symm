package client

import (
	"context"
	"testing"
)

func TestSubscribeSymbolsBatchedReplay(t *testing.T) {
	publicClient := NewPublicClient(context.Background(), WithReplay([][]byte{[]byte(`{}`)}, 0))

	symbols := make([]string, 125)

	for index := range symbols {
		symbols[index] = "BTC/EUR"
	}

	err := SubscribeSymbolsBatched(publicClient, symbols, 50, func(chunk []string) any {
		return map[string]any{
			"channel": "ticker",
			"symbol":  chunk,
		}
	})

	if err != nil {
		t.Fatalf("batched subscribe: %v", err)
	}
}
