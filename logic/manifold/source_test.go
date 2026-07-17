package manifold

import (
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
TestOrdersForSymbolConcurrentUpdates proves the BookSource read lease keeps
ordersForSymbol from ranging Side.Levels while writers mutate the same map.
*/
func TestOrdersForSymbolConcurrentUpdates(t *testing.T) {
	source := newTestBookSource("BTC/USD")
	symbolBook := source.manager.GetBook("BTC/USD")
	done := make(chan struct{})
	var wait sync.WaitGroup

	wait.Add(1)

	go func() {
		defer wait.Done()

		at := time.Unix(2, 0)

		for index := 0; index < 2_000; index++ {
			price := 100.0 + float64(index%20)*0.01
			source.apply(symbolBook, &book.UpdateOptions{
				Direction: book.Bid,
				ID:        "bid-race",
				Price:     decimal.NewFromFloat64(price),
				Quantity:  decimal.NewFromFloat64(1),
				Timestamp: at.Add(time.Duration(index) * time.Millisecond),
			})
		}

		close(done)
	}()

	for {
		select {
		case <-done:
			wait.Wait()
			orders, mid, ok := ordersForSymbol(source, "BTC/USD")

			if !ok || len(orders) == 0 || mid <= 0 {
				t.Fatalf("final ordersForSymbol = %d mid=%v ok=%v", len(orders), mid, ok)
			}

			return
		default:
			orders, mid, ok := ordersForSymbol(source, "BTC/USD")

			if !ok || len(orders) == 0 || mid <= 0 {
				t.Fatalf("ordersForSymbol = %d mid=%v ok=%v", len(orders), mid, ok)
			}
		}
	}
}
