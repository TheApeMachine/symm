package manifold

import (
	"testing"
	"time"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

type observationBook struct {
	bestBid      float64
	bestAsk      float64
	reject       bool
	reason       InvalidReason
	applications int
}

func (book *observationBook) Apply(kraken.Level3Data, int, int) bool {
	book.applications++
	return !book.reject
}

func (book *observationBook) InvalidReason(string) InvalidReason {
	return book.reason
}

func (book *observationBook) TopOfBook(string) (float64, float64, bool) {
	return book.bestBid, book.bestAsk, book.bestBid > 0 && book.bestAsk > 0
}

func TestSlotObserve(t *testing.T) {
	Convey("Given a slot with an authoritative L3 book", t, func() {
		config := &pmanifold.Config{
			GridX: 1, GridY: 1, GridZ: 1,
			DomainX: 1, DomainY: 1, DomainZ: 1,
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
		book := &observationBook{bestBid: 99, bestAsk: 101}
		snapshot := kraken.Level3Data{
			Symbol: "BTC/USD", Type: "snapshot", Timestamp: time.Unix(2, 0),
			Bids: []kraken.Level3Order{{
				OrderID: "bid-1", LimitPrice: 99, OrderQty: 1, Timestamp: time.Unix(2, 0),
			}},
			Asks: []kraken.Level3Order{{
				OrderID: "ask-1", LimitPrice: 101, OrderQty: 1, Timestamp: time.Unix(2, 0),
			}},
		}

		Convey("When a row regresses behind the latest valid observation", func() {
			first := slot.Observe(snapshot, 1, 8, book)
			regressed := slot.Observe(kraken.Level3Data{
				Symbol: "BTC/USD", Type: "update", Timestamp: time.Unix(1, 0),
				Bids: []kraken.Level3Order{{
					Event: "modify", OrderID: "bid-1", LimitPrice: 99,
					OrderQty: 2, Timestamp: time.Unix(1, 0),
				}},
			}, 1, 8, book)

			Convey("Then it invalidates scheduling before mutating the book or population", func() {
				So(first.AdvanceReady, ShouldBeTrue)
				So(regressed.AdvanceReady, ShouldBeFalse)
				So(regressed.State.InvalidReason, ShouldEqual, TimestampRegress)
				So(book.applications, ShouldEqual, 1)
				So(slot.advanceReady, ShouldBeFalse)
				So(slot.population.InvalidReason(), ShouldEqual, TimestampRegress)
			})
		})

		Convey("When a checksum failure is followed by updates and a fresh snapshot", func() {
			book.reject = true
			book.reason = ChecksumFailed
			failed := slot.Observe(snapshot, 1, 8, book)
			book.reject = false
			book.reason = Valid
			update := slot.Observe(kraken.Level3Data{
				Symbol: "BTC/USD", Type: "update", Timestamp: time.Unix(3, 0),
				Bids: []kraken.Level3Order{{
					Event: "add", OrderID: "bid-2", LimitPrice: 98,
					OrderQty: 2, Timestamp: time.Unix(3, 0),
				}},
			}, 1, 8, book)
			snapshot.Timestamp = time.Unix(4, 0)
			snapshot.Bids[0].Timestamp = snapshot.Timestamp
			snapshot.Asks[0].Timestamp = snapshot.Timestamp
			recovered := slot.Observe(snapshot, 1, 8, book)

			Convey("Then no untrusted row is schedulable and the snapshot restores readiness", func() {
				So(failed.AdvanceReady, ShouldBeFalse)
				So(failed.State.InvalidReason, ShouldEqual, ChecksumFailed)
				So(update.AdvanceReady, ShouldBeFalse)
				So(recovered.AdvanceReady, ShouldBeTrue)
				So(recovered.Observation.At, ShouldEqual, time.Unix(4, 0))
				So(slot.lastObservedAt, ShouldEqual, time.Unix(4, 0))
				So(slot.population.Ready(), ShouldBeTrue)
				So(slot.advanceReady, ShouldBeTrue)
			})
		})
	})
}

func BenchmarkSlotObserve(b *testing.B) {
	config := &pmanifold.Config{
		GridX: 1, GridY: 1, GridZ: 1,
		DomainX: 1, DomainY: 1, DomainZ: 1,
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

	book := &observationBook{bestBid: 99, bestAsk: 101}
	slot.Observe(kraken.Level3Data{
		Symbol: "BTC/USD", Type: "snapshot", Timestamp: time.Unix(0, 1),
		Bids: []kraken.Level3Order{{
			OrderID: "bid-1", LimitPrice: 99, OrderQty: 1, Timestamp: time.Unix(0, 1),
		}},
		Asks: []kraken.Level3Order{{
			OrderID: "ask-1", LimitPrice: 101, OrderQty: 1, Timestamp: time.Unix(0, 1),
		}},
	}, 1, 8, book)
	update := kraken.Level3Data{
		Symbol: "BTC/USD", Type: "update",
		Bids: []kraken.Level3Order{{
			Event: "modify", OrderID: "bid-1", LimitPrice: 99,
		}},
	}

	for index := 0; b.Loop(); index++ {
		at := time.Unix(0, int64(index)+2)
		update.Timestamp = at
		update.Bids[0].OrderQty = float64(index%2) + 1
		update.Bids[0].Timestamp = at
		slot.Observe(update, 1, 8, book)
	}
}
