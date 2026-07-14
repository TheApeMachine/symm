package manifold

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSlotObserveBook(t *testing.T) {
	Convey("Given an SDK-managed Level3 book", t, func() {
		config := &pmanifold.Config{
			GridX: 2, GridY: 1, GridZ: 1,
			DomainX: 2, DomainY: 1, DomainZ: 1,
		}
		slot, err := newSlot(
			"BTC/USD",
			config,
			ForecastConfig{InitialVariance: 1, ForgettingFactor: 1},
			256,
			time.Second,
			1e-9,
		)
		So(err, ShouldBeNil)
		managed := book.New()
		managed.Name = "BTC/USD"
		at := time.Unix(1, 0)
		managed.Update(&book.UpdateOptions{
			Direction: book.Bid, ID: "bid-1",
			Price: decimal.NewFromFloat64(99.5), Quantity: decimal.NewFromFloat64(2),
			Timestamp: at,
		})
		managed.Update(&book.UpdateOptions{
			Direction: book.Ask, ID: "ask-1",
			Price: decimal.NewFromFloat64(100.5), Quantity: decimal.NewFromFloat64(3),
			Timestamp: at,
		})

		Convey("When the same book is observed twice", func() {
			first := slot.ObserveBook(managed)
			second := slot.ObserveBook(managed)

			Convey("Then the SDK touch is scheduled once without inventing another epoch", func() {
				So(first.AdvanceReady, ShouldBeTrue)
				So(second.AdvanceReady, ShouldBeFalse)
				So(slot.pending.bestBid, ShouldEqual, managed.BestBid().Price.Float64())
				So(slot.pending.bestAsk, ShouldEqual, managed.BestAsk().Price.Float64())
				So(slot.pending.bestBidQuantity, ShouldEqual, managed.BestBid().Quantity.Float64())
				So(slot.pending.bestAskQuantity, ShouldEqual, managed.BestAsk().Quantity.Float64())
				So(slot.pending.midPrice, ShouldEqual, managed.Midpoint().Float64())
			})
		})

		Convey("When an existing order quantity changes", func() {
			So(slot.ObserveBook(managed).AdvanceReady, ShouldBeTrue)
			managed.Update(&book.UpdateOptions{
				Direction: book.Bid, ID: "bid-1",
				Price: decimal.NewFromFloat64(99.5), Quantity: decimal.NewFromFloat64(4),
				Timestamp: at.Add(time.Second),
			})
			changed := slot.ObserveBook(managed)

			Convey("Then the exact carrier and amendment ledger are updated", func() {
				So(changed.AdvanceReady, ShouldBeTrue)
				So(slot.population.orders["bid-1"].Quantity, ShouldEqual, 4.0)
				So(changed.Accounting.Amended, ShouldEqual, 2.0)
			})
		})
	})
}

func TestPopulationReconcileBook(t *testing.T) {
	Convey("Given a two-level SDK book and its carrier population", t, func() {
		managed := book.New()
		managed.Name = "BTC/USD"
		managed.MaxDepth = 2
		at := time.Unix(1, 0)

		for _, update := range []*book.UpdateOptions{
			{Direction: book.Bid, ID: "bid-1", Price: decimal.NewFromFloat64(100), Quantity: decimal.NewFromFloat64(1), Timestamp: at},
			{Direction: book.Bid, ID: "bid-2", Price: decimal.NewFromFloat64(99), Quantity: decimal.NewFromFloat64(2), Timestamp: at},
			{Direction: book.Ask, ID: "ask-1", Price: decimal.NewFromFloat64(101), Quantity: decimal.NewFromFloat64(1), Timestamp: at},
			{Direction: book.Ask, ID: "ask-2", Price: decimal.NewFromFloat64(102), Quantity: decimal.NewFromFloat64(2), Timestamp: at},
		} {
			managed.Update(update)
		}

		population := NewPopulation("BTC/USD", NewLifetimeEstimator(256))
		So(population.ReconcileBook(managed), ShouldBeTrue)

		Convey("When the SDK removes a level outside its configured depth", func() {
			managed.Update(&book.UpdateOptions{
				Direction: book.Bid, ID: "bid-new",
				Price: decimal.NewFromFloat64(100.5), Quantity: decimal.NewFromFloat64(3),
				Timestamp: at.Add(time.Second),
			})
			changed := population.ReconcileBook(managed)

			Convey("Then manifold consumes that book without a second depth calculation", func() {
				So(changed, ShouldBeTrue)
				So(managed.Bids.Levels, ShouldHaveLength, 2)
				So(population.orders, ShouldNotContainKey, "bid-2")
				So(population.Accounting().Removed, ShouldEqual, 2.0)
				So(population.Accounting().Final(), ShouldEqual, 7.0)
			})
		})

		Convey("When an order disappears inside the visible SDK depth", func() {
			managed.Update(&book.UpdateOptions{
				Direction: book.Bid, ID: "bid-1",
				Price: decimal.NewFromFloat64(100), Quantity: decimal.NewFromFloat64(0),
				Timestamp: at.Add(time.Second),
			})
			changed := population.ReconcileBook(managed)

			Convey("Then the ledger does not invent cancellation or fill semantics", func() {
				So(changed, ShouldBeTrue)
				So(population.Accounting().Removed, ShouldEqual, 1.0)
			})
		})
	})
}

func BenchmarkSlotObserveBook(b *testing.B) {
	config := &pmanifold.Config{
		GridX: 2, GridY: 1, GridZ: 1,
		DomainX: 2, DomainY: 1, DomainZ: 1,
	}
	slot, err := newSlot(
		"BTC/USD",
		config,
		ForecastConfig{InitialVariance: 1, ForgettingFactor: 1},
		256,
		time.Second,
		1e-9,
	)

	if err != nil {
		b.Fatal(err)
	}

	managed := book.New()
	managed.Name = "BTC/USD"
	managed.Update(&book.UpdateOptions{
		Direction: book.Ask, ID: "ask-1",
		Price: decimal.NewFromFloat64(100.5), Quantity: decimal.NewFromFloat64(3),
		Timestamp: time.Unix(1, 0),
	})

	for index := 0; b.Loop(); index++ {
		managed.Update(&book.UpdateOptions{
			Direction: book.Bid, ID: "bid-1",
			Price:     decimal.NewFromFloat64(99.5),
			Quantity:  decimal.NewFromFloat64(float64(index%2) + 1),
			Timestamp: time.Unix(int64(index)+1, 0),
		})
		slot.ObserveBook(managed)
	}
}
