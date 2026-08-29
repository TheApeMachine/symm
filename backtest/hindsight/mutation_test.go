package hindsight

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	. "github.com/smartystreets/goconvey/convey"
)

/*
walkAsksBestPriceOnly is the deliberately broken mutation of the entry walk: it
fills the whole requested quantity at the best ask alone, ignoring deeper depth.
The honest walk must instead walk every level, so any test that requires the
gross VWAP invariant fails against this mutation.
*/
func walkAsksBestPriceOnly(asks *book.Side, quantity float64) WalkResult {
	if asks == nil || asks.Low == nil || quantity <= 0 {
		return WalkResult{}
	}

	best := asks.Low

	if best.Price == nil {
		return WalkResult{}
	}

	return WalkResult{
		FilledQty: quantity,
		Gross:     best.Price.Float64() * quantity,
		VWAP:      best.Price.Float64(),
	}
}

/*
TestWalkMutationKills demonstrates that the walking correctness tests are
adversarial: swapping the honest walk for a best-price-only walk breaks the
gross-VWAP invariant the base tests assert.
*/
func TestWalkMutationKills(t *testing.T) {
	Convey("Mutation: depth walking reduced to best-price-only", t, func() {
		store := bookStoreFromOrders(
			"TEST/USD",
			[]orderSpec{{price: 100, qty: 1}, {price: 110, qty: 2}},
			nil,
		)
		entryBook, ok := store.BookAt("TEST/USD", time.Unix(0, 0).UTC())
		So(ok, ShouldBeTrue)

		honest := WalkAsks(entryBook.Asks, 3)
		So(honest.Gross, ShouldAlmostEqual, 1*100+2*110, 1e-9)
		So(honest.VWAP, ShouldAlmostEqual, float64(1*100+2*110)/3, 1e-9)

		broken := walkAsksBestPriceOnly(entryBook.Asks, 3)

		Convey("The best-price-only walk must fail the gross VWAP invariant", func() {
			So(broken.Gross, ShouldEqual, 3*100)
			So(broken.Gross, ShouldNotAlmostEqual, 1*100+2*110, 1e-9)
			So(broken.VWAP, ShouldNotAlmostEqual, float64(1*100+2*110)/3, 1e-9)
		})
	})

	Convey("Mutation: undefined depth collapsed to zero fill", t, func() {
		store := bookStoreFromOrders(
			"TEST/USD",
			[]orderSpec{{price: 100, qty: 0.5}},
			nil,
		)
		entryBook, ok := store.BookAt("TEST/USD", time.Unix(0, 0).UTC())
		So(ok, ShouldBeTrue)

		walk := WalkAsks(entryBook.Asks, 10)

		Convey("Insufficient depth reports a partial fill, never zero", func() {
			So(walk.FilledQty, ShouldEqual, 0.5)
			So(walk.FilledQty, ShouldNotEqual, 0.0)
		})
	})

	Convey("Mutation: infinite quantity allowed through the cap", t, func() {
		store := bookStoreFromOrders(
			"TEST/USD",
			[]orderSpec{{price: 100, qty: 0.5}},
			[]orderSpec{{price: 200, qty: 0.5}},
		)
		leg := Leg{
			Symbol:         "TEST/USD",
			BuyAt:          time.Unix(0, 0).UTC(),
			SellAt:         time.Unix(0, 0).UTC(),
			BuyPrice:       100,
			SellPrice:      200,
			GrossProfitPct: 1.0,
		}

		Convey("A $10,000 request against $50 depth is not executable", func() {
			_, ok := ExecutableCounterfactual(store, leg, 100, 0)
			So(ok, ShouldBeFalse)
		})

		Convey("An infinite-quantity mutation would fabricate the full move", func() {
			// If the cap were removed and the walk trusted top-of-book, the
			// full 100% move would appear executable at any size. The honest
			// counterfactual refuses it, so the two cannot agree.
			honestWalk := WalkAsks(mustAskSide(store), 100)
			So(honestWalk.FilledQty, ShouldEqual, 0.5)
			So(honestWalk.FilledQty, ShouldNotEqual, 100.0)
		})
	})
}

func mustAskSide(store *BookStore) *book.Side {
	entryBook, _ := store.BookAt("TEST/USD", time.Unix(0, 0).UTC())
	return entryBook.Asks
}

func TestObserverGateMutationKill(t *testing.T) {
	Convey("Mutation: observer start gate removed (capture all history)", t, func() {
		reducer := NewReducer()
		So(reducer.Ingest([]byte(`{"channel":"trade","type":"update","data":[
			{"symbol":"TEST/USD","side":"buy","price":100,"qty":1,"timestamp":"1970-01-01T00:00:00Z"},
			{"symbol":"TEST/USD","side":"buy","price":120,"qty":1,"timestamp":"1970-01-01T00:00:02Z"}
		]}`), epoch), ShouldBeNil)

		observerStarted := epoch.Add(time.Second)

		Convey("The gate excludes the pre-observer leg from missed value", func() {
			reports, err := Analyze(reducer, []Decision{}, nil, observerStarted)
			So(err, ShouldBeNil)
			So(reports[0].MissedLegs, ShouldEqual, 0)
			So(reports[0].MissedPct, ShouldAlmostEqual, 0.0, 1e-9)
		})

		Convey("Removing the gate WOULD count it as missed — the mutation", func() {
			reports, err := Analyze(reducer, []Decision{}, nil, time.Time{})
			So(err, ShouldBeNil)
			So(reports[0].MissedLegs, ShouldNotEqual, 0)
			So(reports[0].MissedPct, ShouldNotAlmostEqual, 0.0, 1e-9)
		})
	})
}
