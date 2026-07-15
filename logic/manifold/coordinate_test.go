package manifold

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

func TestBookForSymbol(t *testing.T) {
	source := newTestBookSource("BTC/USD")
	symbolBook, midPrice, ok := bookForSymbol(source, "BTC/USD")

	if !ok || symbolBook == nil || midPrice <= 0 {
		t.Fatalf("bookForSymbol = %v mid=%v ok=%v", symbolBook, midPrice, ok)
	}
}

func TestMapOrders(t *testing.T) {
	config := testPhysicsConfig()
	at := time.Unix(100, 0)

	orders := []physicalOrder{
		{
			orderID:   "bid-1",
			side:      book.Bid,
			price:     100,
			quantity:  2,
			timestamp: at.Add(-5 * time.Second),
		},
		{
			orderID:   "ask-1",
			side:      book.Ask,
			price:     101,
			quantity:  4,
			timestamp: at.Add(-10 * time.Second),
		},
	}

	mapped, epoch, ready := mapOrders(config, orders, 100.5, at, nil)

	if !ready {
		t.Fatal("mapOrders returned not ready")
	}

	if len(mapped) != 2 {
		t.Fatalf("mapped orders = %d, want 2", len(mapped))
	}

	if epoch == nil || len(epoch.positions) != 2 {
		t.Fatalf("epoch positions = %d, want 2", len(epoch.positions))
	}

	if epoch.spread <= 0 || epoch.buyCapacity != 404 || epoch.sellCapacity != 200 {
		t.Fatalf("execution boundary = %+v", epoch)
	}

	if mapped[0].mass <= 0 || mapped[0].omega <= 0 {
		t.Fatalf("mapped oscillator fields are invalid: %+v", mapped[0])
	}
}

func TestCohortsFromMappedOrders(t *testing.T) {
	config := testPhysicsConfig()
	orders := []mappedOrder{
		{
			orderID: "a",
			mass:    0.25,
			posX:    1,
			posY:    1,
			posZ:    1,
			omega:   2,
			phase:   1,
			heat:    0.25,
		},
		{
			orderID: "b",
			mass:    0.75,
			posX:    1.01,
			posY:    1.01,
			posZ:    1.01,
			omega:   4,
			phase:   2,
			heat:    0.75,
		},
	}

	oscillators := cohortsFromMappedOrders(config, orders)

	if len(oscillators) != 1 {
		t.Fatalf("cohorts = %d, want 1", len(oscillators))
	}

	if oscillators[0].Amplitude <= 0 {
		t.Fatalf("amplitude = %v, want positive", oscillators[0].Amplitude)
	}
}

func TestOrdersFromBook(t *testing.T) {
	symbolBook := book.New()
	symbolBook.Update(&book.UpdateOptions{
		Direction: book.Bid,
		ID:        "bid-1",
		Price:     decimal.NewFromFloat64(100),
		Quantity:  decimal.NewFromFloat64(1.5),
		Timestamp: time.Unix(1, 0),
	})
	symbolBook.Update(&book.UpdateOptions{
		Direction: book.Ask,
		ID:        "ask-1",
		Price:     decimal.NewFromFloat64(101),
		Quantity:  decimal.NewFromFloat64(2.5),
		Timestamp: time.Unix(2, 0),
	})

	orders := ordersFromBook(symbolBook)

	if len(orders) != 2 {
		t.Fatalf("orders = %d, want 2", len(orders))
	}
}

func BenchmarkMapOrders(b *testing.B) {
	config := testPhysicsConfig()
	at := time.Unix(100, 0)
	orders := make([]physicalOrder, 128)

	for index := range orders {
		direction := book.BookDirection(book.Bid)
		price := 100 - float64(index)*0.01

		if index%2 != 0 {
			direction = book.Ask
			price = 101 + float64(index)*0.01
		}

		orders[index] = physicalOrder{
			orderID:   "order",
			side:      direction,
			price:     price,
			quantity:  1 + float64(index)*0.1,
			timestamp: at.Add(-time.Duration(index) * time.Second),
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _, ready := mapOrders(config, orders, 100.5, at, nil)

		if !ready {
			b.Fatal("mapOrders returned not ready")
		}
	}
}
