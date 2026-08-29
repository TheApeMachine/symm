package hindsight

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
bookStoreFromOrders builds a BookStore with a single snapshot carrying the given
ask orders (entry book) and bid orders (exit support), timed at the venue epoch
so the counterfactual boundary can query the reconstructed book exactly.
*/
func bookStoreFromOrders(symbol string, asks, bids []orderSpec) *BookStore {
	store := NewBookStore()
	payload := l3SnapshotPayload(symbol, asks, bids)
	//nolint:errcheck // fixture payloads are well-formed; a decode error is a test bug.
	_ = store.Apply(payload, time.Unix(0, 0).UTC())

	return store
}

type orderSpec struct {
	price float64
	qty   float64
}

func l3SnapshotPayload(symbol string, asks, bids []orderSpec) []byte {
	body := `{"channel":"level3","type":"snapshot","data":[{"symbol":"%s","bids":[%s],"asks":[%s],"timestamp":"1970-01-01T00:00:00Z"}]}`

	return []byte(fmt.Sprintf(body, symbol, bidJSON(bids), askJSON(asks)))
}

func bidJSON(specs []orderSpec) string {
	out := ""
	for index, spec := range specs {
		if index > 0 {
			out += ","
		}
		out += fmt.Sprintf(
			`{"order_id":"bid-%d","limit_price":%g,"order_qty":%g}`,
			index,
			spec.price,
			spec.qty,
		)
	}

	return out
}

func askJSON(specs []orderSpec) string {
	out := ""
	for index, spec := range specs {
		if index > 0 {
			out += ","
		}
		out += fmt.Sprintf(
			`{"order_id":"ask-%d","limit_price":%g,"order_qty":%g}`,
			index,
			spec.price,
			spec.qty,
		)
	}

	return out
}

func TestWalkAsks(t *testing.T) {
	Convey("Given an ask ladder 1@100, 2@110", t, func() {
		store := bookStoreFromOrders(
			"TEST/USD",
			[]orderSpec{{price: 100, qty: 1}, {price: 110, qty: 2}},
			nil,
		)

		Convey("Buying quantity 3 walks both levels for the gross VWAP", func() {
			entryBook, ok := store.BookAt("TEST/USD", time.Unix(0, 0).UTC())
			So(ok, ShouldBeTrue)

			walk := WalkAsks(entryBook.Asks, 3)

			So(walk.FilledQty, ShouldEqual, 3)
			So(walk.Gross, ShouldAlmostEqual, 1*100+2*110, 1e-9)
			So(walk.VWAP, ShouldAlmostEqual, float64(1*100+2*110)/3, 1e-9)
		})

		Convey("Requesting more than available depth never fabricates liquidity", func() {
			entryBook, ok := store.BookAt("TEST/USD", time.Unix(0, 0).UTC())
			So(ok, ShouldBeTrue)

			walk := WalkAsks(entryBook.Asks, 100)

			So(walk.FilledQty, ShouldEqual, 3)
		})
	})
}

func TestWalkBids(t *testing.T) {
	Convey("Given a bid ladder 2@110, 1@100", t, func() {
		store := bookStoreFromOrders(
			"TEST/USD",
			nil,
			[]orderSpec{{price: 110, qty: 2}, {price: 100, qty: 1}},
		)

		Convey("Selling quantity 3 walks best bid first for the gross VWAP", func() {
			exitBook, ok := store.BookAt("TEST/USD", time.Unix(0, 0).UTC())
			So(ok, ShouldBeTrue)

			walk := WalkBids(exitBook.Bids, 3)

			So(walk.FilledQty, ShouldEqual, 3)
			So(walk.Gross, ShouldAlmostEqual, 2*110+1*100, 1e-9)
			So(walk.VWAP, ShouldAlmostEqual, float64(2*110+1*100)/3, 1e-9)
		})
	})
}

func TestBookStoreTemporalSemantics(t *testing.T) {
	epoch := time.Unix(0, 0).UTC()

	Convey("Given a snapshot, then rich liquidity one frame later", t, func() {
		store := NewBookStore()

		thin := []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"TEST/USD","bids":[],"asks":[{"order_id":"a0","limit_price":100,"order_qty":0.5}],"timestamp":"1970-01-01T00:00:00Z"}]}`)
		rich := []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"TEST/USD","bids":[],"asks":[{"order_id":"b0","limit_price":100,"order_qty":100}],"timestamp":"1970-01-01T00:00:03Z"}]}`)

		So(store.Apply(thin, epoch), ShouldBeNil)
		So(store.Apply(rich, epoch.Add(3*time.Second)), ShouldBeNil)

		Convey("A counterfactual at t=0 must not see the later liquidity", func() {
			book, ok := store.BookAt("TEST/USD", epoch)
			So(ok, ShouldBeTrue)

			walk := WalkAsks(book.Asks, 1)
			So(walk.FilledQty, ShouldEqual, 0.5)
		})

		Convey("A counterfactual at t=3 sees the full later book", func() {
			book, ok := store.BookAt("TEST/USD", epoch.Add(3*time.Second))
			So(ok, ShouldBeTrue)

			walk := WalkAsks(book.Asks, 1)
			So(walk.FilledQty, ShouldEqual, 1)
		})
	})

	Convey("Given a boundary before any snapshot", t, func() {
		store := NewBookStore()

		So(store.Apply([]byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"TEST/USD","bids":[],"asks":[],"timestamp":"1970-01-01T00:00:05Z"}]}`), epoch), ShouldBeNil)

		Convey("It must report the book uninitialized, not an empty-but-walkable book", func() {
			_, ok := store.BookAt("TEST/USD", epoch)
			So(ok, ShouldBeFalse)
		})
	})
}

func TestExecutableCounterfactual(t *testing.T) {
	epoch := time.Unix(0, 0).UTC()

	Convey("Given a $10,000 request against only $50 of entry depth", t, func() {
		store := NewBookStore()

		// Entry asks total exactly $50 of notional (0.5 qty at 100).
		thinEntry := []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"TEST/USD","bids":[],"asks":[{"order_id":"a0","limit_price":100,"order_qty":0.5}],"timestamp":"1970-01-01T00:00:00Z"}]}`)
		// Exit bids are ample.
		exitBook := []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"TEST/USD","bids":[{"order_id":"b0","limit_price":200,"order_qty":10000}],"asks":[],"timestamp":"1970-01-01T00:00:10Z"}]}`)

		So(store.Apply(thinEntry, epoch), ShouldBeNil)
		So(store.Apply(exitBook, epoch.Add(10*time.Second)), ShouldBeNil)

		leg := Leg{
			Symbol:         "TEST/USD",
			BuyAt:          epoch,
			SellAt:         epoch.Add(10 * time.Second),
			BuyPrice:       100,
			SellPrice:      200,
			GrossProfitPct: 1.0,
		}

		Convey("The 100%% move on $10,000 must not be reported executable", func() {
			outcome, ok := ExecutableCounterfactual(store, leg, 100, 0)
			So(ok, ShouldBeFalse)
			So(outcome.FullyExecutable, ShouldBeFalse)
		})
	})

	Convey("Given a fully executable round trip", t, func() {
		store := NewBookStore()

		entry := []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"TEST/USD","bids":[],"asks":[{"order_id":"a0","limit_price":100,"order_qty":1},{"order_id":"a1","limit_price":110,"order_qty":2}],"timestamp":"1970-01-01T00:00:00Z"}]}`)
		exitBids := []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"TEST/USD","bids":[{"order_id":"b0","limit_price":200,"order_qty":3}],"asks":[],"timestamp":"1970-01-01T00:00:10Z"}]}`)

		So(store.Apply(entry, epoch), ShouldBeNil)
		So(store.Apply(exitBids, epoch.Add(10*time.Second)), ShouldBeNil)

		leg := Leg{
			Symbol:         "TEST/USD",
			BuyAt:          epoch,
			SellAt:         epoch.Add(10 * time.Second),
			BuyPrice:       100,
			SellPrice:      200,
			GrossProfitPct: 1.0,
		}

		Convey("It walks asks for entry and bids for exit", func() {
			outcome, ok := ExecutableCounterfactual(store, leg, 3, 0)
			So(ok, ShouldBeTrue)
			So(outcome.FullyExecutable, ShouldBeTrue)
			So(outcome.ExecutableEntryVWAP, ShouldAlmostEqual, float64(1*100+2*110)/3, 1e-9)
			So(outcome.ExecutableExitVWAP, ShouldAlmostEqual, 200, 1e-9)
			So(outcome.ExecutablePnL, ShouldAlmostEqual, 3*200-(1*100+2*110), 1e-9)
		})
	})

	Convey("Given entry fills fully but exit depth is insufficient", t, func() {
		store := NewBookStore()

		entry := []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"TEST/USD","bids":[],"asks":[{"order_id":"a0","limit_price":100,"order_qty":3}],"timestamp":"1970-01-01T00:00:00Z"}]}`)
		exitBids := []byte(`{"channel":"level3","type":"snapshot","data":[{"symbol":"TEST/USD","bids":[{"order_id":"b0","limit_price":200,"order_qty":1}],"asks":[],"timestamp":"1970-01-01T00:00:10Z"}]}`)

		So(store.Apply(entry, epoch), ShouldBeNil)
		So(store.Apply(exitBids, epoch.Add(10*time.Second)), ShouldBeNil)

		leg := Leg{
			Symbol:         "TEST/USD",
			BuyAt:          epoch,
			SellAt:         epoch.Add(10 * time.Second),
			BuyPrice:       100,
			SellPrice:      200,
			GrossProfitPct: 1.0,
		}

		Convey("The round trip is undefined — best bid is not the exit", func() {
			outcome, ok := ExecutableCounterfactual(store, leg, 3, 0)
			So(ok, ShouldBeFalse)
			So(outcome.ExecutableExitQty, ShouldEqual, 0)
		})
	})
}
