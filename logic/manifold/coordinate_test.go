package manifold

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

func TestOrdersForSymbol(t *testing.T) {
	source := newTestBookSource("BTC/USD")
	orders, midPrice, ok := ordersForSymbol(source, "BTC/USD")

	if !ok || len(orders) == 0 || midPrice <= 0 {
		t.Fatalf("ordersForSymbol = %d mid=%v ok=%v", len(orders), midPrice, ok)
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

	if epoch == nil {
		t.Fatal("epoch is nil")
	}

	if len(epoch.positions) != 2 {
		t.Fatalf("epoch positions = %d, want 2", len(epoch.positions))
	}

	if epoch.spread <= 0 || epoch.buyCapacity != 404 || epoch.sellCapacity != 200 {
		t.Fatalf("execution boundary = %+v", epoch)
	}

	if mapped[0].mass <= 0 || mapped[0].omega <= 0 {
		t.Fatalf("mapped oscillator fields are invalid: %+v", mapped[0])
	}

	if mapped[0].posX >= config.DomainX/2 || mapped[1].posX <= config.DomainX/2 {
		t.Fatalf("book sides collapsed onto one coordinate side: %+v", mapped)
	}

	if rank := survivalCoordinate(5, []float64{5, 5, 10}); rank != 2.0/3.0 {
		t.Fatalf("upper-tie survival rank = %v, want %v", rank, 2.0/3.0)
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
			phase:   2*math.Pi - 0.1,
			heat:    0.25,
		},
		{
			orderID: "b",
			mass:    0.75,
			posX:    1.01,
			posY:    1.01,
			posZ:    1.01,
			omega:   4,
			phase:   0.1,
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

	if math.Abs(oscillators[0].Phase) > 0.11 {
		t.Fatalf("circular phase = %v, want near zero", oscillators[0].Phase)
	}

	if torusCell(config, config.DomainX, config.DomainX, config.GridX) != 0 ||
		torusCell(
			config, -config.DomainX/float64(config.GridX), config.DomainX, config.GridX,
		) != config.GridX-1 {
		t.Fatal("toroidal cell mapping did not wrap")
	}

	limited := config
	limited.MaxModes = 1
	equalMass := cohortsFromMappedOrders(limited, []mappedOrder{
		{mass: 0.5, posX: 0.9, posY: 0.9, posZ: 0.9},
		{mass: 0.5, posX: 0.1, posY: 0.1, posZ: 0.1},
	})

	if len(equalMass) != 1 || equalMass[0].PosX != 0.1 {
		t.Fatalf("equal-mass cohort selection is not deterministic: %+v", equalMass)
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

	oneSided := ordersFromBook(&book.Book{Bids: symbolBook.Bids})

	if len(oneSided) != 1 {
		t.Fatalf("one-sided orders = %d, want 1", len(oneSided))
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
