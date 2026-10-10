package broker

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
fakeDepth is a book history of hand-built versions in venue-time order.
*/
type fakeDepth struct {
	mu    sync.Mutex
	views []*BookView
}

func (depth *fakeDepth) push(view *BookView) {
	depth.mu.Lock()
	defer depth.mu.Unlock()
	depth.views = append(depth.views, view)
}

func (depth *fakeDepth) index(at time.Time) int {
	found := -1

	for i, view := range depth.views {
		if !view.At.After(at) {
			found = i
		}
	}

	return found
}

func (depth *fakeDepth) BookAt(_ string, at time.Time, read func(*BookView)) {
	depth.mu.Lock()
	defer depth.mu.Unlock()

	if i := depth.index(at); i >= 0 {
		read(depth.views[i])
	}
}

func (depth *fakeDepth) BookWindow(_ string, from, to time.Time, read func(*BookView)) bool {
	depth.mu.Lock()
	defer depth.mu.Unlock()

	first := depth.index(from)

	for _, view := range depth.views[max(first, 0):] {
		if view.At.After(to) {
			break
		}

		read(view)
	}

	return first >= 0
}

func (depth *fakeDepth) Latest(_ string, read func(*BookView)) {
	depth.mu.Lock()
	defer depth.mu.Unlock()

	if len(depth.views) > 0 {
		read(depth.views[len(depth.views)-1])
	}
}

/*
placed is one market order the venue received.
*/
type placed struct {
	side   string
	pair   string
	id     string
	volume *decimal.Decimal
}

/*
venue fills market orders at a fixed unit price per side, like Paper fills
at the ticker: it ignores depth. Sides in hold are not filled until the test
calls fill.
*/
type venue struct {
	desk   *Desk
	unit   map[string]float64
	fee    float64
	hold   map[string]bool
	orders chan placed
}

func (venue *venue) Write(buf []byte) error {
	message := kraken.AddOrderMessage{}

	if err := sonic.Unmarshal(buf, &message); err != nil {
		return err
	}

	volume, err := decimal.NewFromString(message.Params.Volume)

	if err != nil {
		return err
	}

	order := placed{side: message.Params.Type, pair: message.Params.Pair, id: message.Params.ClOrdId, volume: volume}

	if !venue.hold[order.side] {
		venue.fill(order)
	}

	venue.orders <- order
	return nil
}

func (venue *venue) fill(order placed) {
	venue.desk.Apply(&kraken.Execution{Data: []kraken.ExecutionData{{
		ClientOrderID: order.id,
		Symbol:        order.pair,
		Side:          order.side,
		LastQty:       order.volume,
		Cost:          decimal.NewFromFloat64(order.volume.Float64() * venue.unit[order.side]),
		FeeUsdEquiv:   decimal.NewFromFloat64(venue.fee),
	}}})
}

func (venue *venue) next() placed {
	select {
	case order := <-venue.orders:
		return order
	case <-time.After(3 * time.Second):
		So("an order was placed", ShouldEqual, "no order arrived")
		return placed{}
	}
}

func (venue *venue) none() {
	select {
	case order := <-venue.orders:
		So(order.side+" "+order.volume.String(), ShouldEqual, "no order")
	case <-time.After(150 * time.Millisecond):
	}
}

var deskNow = time.Unix(1_700_000_000, 0)

/*
deskFixture is a Desk for BTC/USD at a 0.8% taker fee with cash, a flow
history of nine one-minute windows whose sums alternate +-swing, and the hand
book (mid 100.25) as both the version 30s ago and the newest one.
*/
func deskFixture(cash, swing float64) (*Desk, *fakeDepth, *venue) {
	normalizer := spot.NewNormalizer()
	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{"BTC": {AltName: "XBT"}, "USD": {AltName: "USD"}},
		NewPairs: map[string]spot.AssetPair{
			"BTC/USD": {WSName: "BTC/USD", Base: "BTC", Quote: "USD", LotDecimals: 8, LotMultiplier: 1},
		},
	})

	price := NewPrice(context.Background(), nil, nil, nil, normalizer)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.8)})
	price.SetReferenceCash(decimal.NewFromFloat64(cash))
	price.Flow = NewFlow()
	price.Flow.Record("BTC/USD", deskNow.Add(-10*time.Minute), "buy", 1)

	for k := 0; k < 9; k++ {
		side := "buy"

		if k%2 == 1 {
			side = "sell"
		}

		price.Flow.Record("BTC/USD", deskNow.Add(-time.Duration(9-k)*time.Minute+30*time.Second), side, swing)
	}

	depth := &fakeDepth{}
	older := handBook()
	older.At = deskNow.Add(-30 * time.Second)
	depth.push(older)
	latest := handBook()
	latest.At = deskNow
	depth.push(latest)

	transport := &venue{
		unit:   map[string]float64{"buy": 100.5, "sell": 100},
		fee:    0.1,
		hold:   map[string]bool{},
		orders: make(chan placed, 16),
	}

	desk := NewDesk(context.Background(), transport, price)
	transport.desk = desk
	desk.UseDepth(depth)

	return desk, depth, transport
}

/*
matched is the edge of a 5% median gain over a one-minute hold: a per-side
budget of (0.05 - 0.016)/2 = 1.7%.
*/
var matched = Edge{Gains: []float64{0.05}, Holds: []time.Duration{time.Minute}}

func sellsOf(desk *Desk, symbol string) []Sell {
	desk.mu.Lock()
	defer desk.mu.Unlock()

	if held, ok := desk.positions[symbol]; ok {
		return append([]Sell{}, held.sells...)
	}

	return nil
}

func shadowOf(desk *Desk, symbol string) ShadowLedger {
	desk.mu.Lock()
	defer desk.mu.Unlock()

	return desk.positions[symbol].shadow
}

/*
floored is quantity rounded down to 8 lot decimals.
*/
func floored(quantity float64) float64 {
	return math.Floor(quantity*1e8) / 1e8
}

/*
thinBids is the hand book with bids 0.5@100 and 100@50: within a 1.7% budget
the bids absorb a little over 0.5.
*/
func thinBids(at time.Time) *BookView {
	view := handBook()
	view.At = at
	view.Bids = []BookLevel{{Price: 100, Quantity: 0.5}, {Price: 50, Quantity: 100}}
	view.Asks = []BookLevel{{Price: 100.5, Quantity: 0.31}, {Price: 101, Quantity: 0.51}, {Price: 101.5, Quantity: 10.1}}
	return view
}

/*
persist shows the monitor a material shortfall that lasts its persistence
window (core.Tolerance of the one-minute matched hold): view at at, which must
not sell, then the same book a window later.
*/
func persist(desk *Desk, depth *fakeDepth, venue *venue, book func(time.Time) *BookView, at time.Time) *BookView {
	depth.push(book(at))
	desk.Wake("BTC/USD")
	venue.none()

	later := book(at.Add(time.Duration(core.Tolerance * float64(time.Minute))))
	depth.push(later)
	desk.Wake("BTC/USD")

	return later
}

func TestDesk_EntrySizing(t *testing.T) {
	Convey("Given a Desk sizing entries from a 1.7% budget", t, func() {
		Convey("When cash is the smallest limit, the entry spends the cash at the budget's entry limit", func() {
			desk, _, venue := deskFixture(200, 1000)
			So(desk.Enter("BTC/USD", matched), ShouldBeNil)

			order := venue.next()
			So(order.side, ShouldEqual, "buy")
			So(order.volume.Float64(), ShouldAlmostEqual, floored(200/(100.25*1.017*1.008)), 1e-9)
			So(desk.State("BTC/USD"), ShouldEqual, HOLDING)
		})

		Convey("When the bids were thinner earlier in the expected hold, that weakest exit capacity binds", func() {
			desk, depth, venue := deskFixture(1_000_000, 1000)
			thin := handBook()
			thin.At = deskNow.Add(-20 * time.Second)
			thin.Bids = []BookLevel{{Price: 100, Quantity: 1}, {Price: 90, Quantity: 50}}
			depth.views = []*BookView{depth.views[0], thin, depth.views[1]}

			So(desk.Enter("BTC/USD", matched), ShouldBeNil)

			order := venue.next()
			So(order.volume.Float64(), ShouldAlmostEqual, floored(ExitCapacity(thin, 0.017).Quantity), 1e-9)
			So(order.volume.Float64(), ShouldBeLessThan, ExitCapacity(handBook(), 0.017).Quantity)
		})

		Convey("When signed flow is quiet, its noise over one expected hold binds", func() {
			desk, _, venue := deskFixture(1_000_000, 0.2)
			noise, _, ok := desk.price.Flow.Noise("BTC/USD", deskNow, time.Minute)
			So(ok, ShouldBeTrue)

			So(desk.Enter("BTC/USD", matched), ShouldBeNil)
			So(venue.next().volume.Float64(), ShouldAlmostEqual, floored(noise), 1e-9)
		})

		Convey("When the asks within budget are thin, each child takes only the resting asks", func() {
			desk, depth, venue := deskFixture(1_000_000, 1000)
			depth.views[1].Asks = []BookLevel{{Price: 100.5, Quantity: 0.3}, {Price: 101, Quantity: 0.5}}

			So(desk.Enter("BTC/USD", matched), ShouldBeNil)
			So(venue.next().volume.Float64(), ShouldAlmostEqual, 0.8, 1e-12)

			// The shadow took both levels; the same version has nothing left.
			desk.Wake("BTC/USD")
			venue.none()

			// A new version where 100.5 refreshed to 0.4 and 101 still shows
			// the 0.5 the first child took: only 0.4 rests.
			refreshed := handBook()
			refreshed.At = deskNow.Add(time.Second)
			refreshed.Asks = []BookLevel{{Price: 100.5, Quantity: 0.4}, {Price: 101, Quantity: 0.5}}
			depth.push(refreshed)
			desk.Wake("BTC/USD")

			So(venue.next().volume.Float64(), ShouldAlmostEqual, 0.4, 1e-12)
		})

		Convey("Every venue fill is walked through the as-of book into the shadow ledger", func() {
			desk, _, venue := deskFixture(200, 1000)
			So(desk.Enter("BTC/USD", matched), ShouldBeNil)
			order := venue.next()

			expected := NewShadow().Fill("BTC/USD", BUY, handBook(), order.volume.Float64())
			ledger := shadowOf(desk, "BTC/USD")
			So(ledger.Defined(), ShouldBeTrue)
			So(ledger.Bought, ShouldAlmostEqual, order.volume.Float64(), 1e-12)
			So(ledger.Cost, ShouldAlmostEqual, expected.Gross, 1e-9)
			So(ledger.Fees, ShouldAlmostEqual, expected.Gross*0.008, 1e-9)
			// The venue filled everything at 100.5; the book walk paid more.
			So(ledger.Cost, ShouldBeGreaterThan, order.volume.Float64()*100.5)
		})

		Convey("An entry is refused when participation is undefined", func() {
			desk, _, _ := deskFixture(200, 1000)
			desk.price.Flow = NewFlow()

			err := desk.Enter("BTC/USD", matched)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "participation undefined")
			So(desk.State("BTC/USD"), ShouldEqual, FLAT)
		})

		Convey("An entry is refused when the edge does not clear the round-trip fee", func() {
			desk, _, _ := deskFixture(200, 1000)
			So(desk.Enter("BTC/USD", Edge{Gains: []float64{0.01}, Holds: []time.Duration{time.Minute}}), ShouldNotBeNil)
		})

		Convey("An entry is refused when no matched statistics were readable", func() {
			// Unreadable blobs leave the edge empty. A short live path span
			// must not stand in for the hold: it would size participation
			// from a window of seconds.
			desk, _, venue := deskFixture(200, 1000)

			err := desk.Enter("BTC/USD", Edge{Gains: []float64{0.05}})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "no readable matched durations")
			So(desk.Enter("BTC/USD", Edge{}), ShouldNotBeNil)
			So(desk.State("BTC/USD"), ShouldEqual, FLAT)
			venue.none()
		})

		Convey("A second entry on an open symbol is refused", func() {
			desk, _, venue := deskFixture(200, 1000)
			So(desk.Enter("BTC/USD", matched), ShouldBeNil)
			venue.next()
			So(desk.Enter("BTC/USD", matched), ShouldNotBeNil)
		})
	})
}

func TestDesk_CapacityMonitor(t *testing.T) {
	Convey("Given a filled position of the cash-bound size", t, func() {
		desk, depth, venue := deskFixture(200, 1000)
		So(desk.Enter("BTC/USD", matched), ShouldBeNil)
		bought := venue.next().volume

		Convey("When exit capacity falls below the open quantity, the monitor trims to capacity", func() {
			thin := persist(desk, depth, venue, thinBids, deskNow.Add(time.Second))
			order := venue.next()
			capacity := ExitCapacity(thin, 0.017)
			keep := floored(capacity.Quantity)

			So(order.side, ShouldEqual, "sell")
			So(order.volume.Float64(), ShouldAlmostEqual, bought.Float64()-keep, 1e-9)

			sells := sellsOf(desk, "BTC/USD")
			So(sells, ShouldHaveLength, 1)
			So(sells[0].Trigger, ShouldEqual, TriggerCapacityTrim)
			So(sells[0].Capacity.Quantity, ShouldAlmostEqual, capacity.Quantity, 1e-12)
			So(sells[0].Ratio, ShouldAlmostEqual, capacity.Quantity/bought.Float64(), 1e-12)
			So(sells[0].Budget, ShouldAlmostEqual, 0.017, 1e-15)
			So(desk.State("BTC/USD"), ShouldEqual, HOLDING)

			Convey("and the trim's shadow proceeds walk the thin bids", func() {
				ledger := shadowOf(desk, "BTC/USD")
				So(ledger.Proceeds, ShouldAlmostEqual, NewShadow().Fill("BTC/USD", SELL, thin, order.volume.Float64()).Gross, 1e-9)
			})
		})

		Convey("When capacity is ample, the monitor does nothing", func() {
			desk.Wake("BTC/USD")
			venue.none()
			So(sellsOf(desk, "BTC/USD"), ShouldBeEmpty)
		})
	})
}

func TestDesk_ExitPrecedence(t *testing.T) {
	Convey("Given a filled position whose sells the venue holds unfilled", t, func() {
		desk, depth, venue := deskFixture(200, 1000)
		venue.hold["sell"] = true
		So(desk.Enter("BTC/USD", matched), ShouldBeNil)
		bought := venue.next().volume

		var closures []Closure
		desk.OnClose(func(closure Closure) { closures = append(closures, closure) })

		Convey("A learned exit during an in-flight capacity trim sells the rest under learned_exit", func() {
			persist(desk, depth, venue, thinBids, deskNow.Add(time.Second))
			trim := venue.next()

			So(desk.Exit("BTC/USD"), ShouldBeNil)
			rest := venue.next()

			So(rest.volume.Add(trim.volume).Cmp(bought), ShouldEqual, 0)
			So(desk.State("BTC/USD"), ShouldEqual, EXITING)

			venue.fill(trim)
			venue.fill(rest)

			So(closures, ShouldHaveLength, 1)
			So(closures[0].Sells[0].Trigger, ShouldEqual, TriggerCapacityTrim)
			So(closures[0].Sells[1].Trigger, ShouldEqual, TriggerLearnedExit)
		})

		Convey("After a learned exit the monitor never sells", func() {
			So(desk.Exit("BTC/USD"), ShouldBeNil)
			exit := venue.next()
			So(exit.volume.Cmp(bought), ShouldEqual, 0)

			depth.push(thinBids(deskNow.Add(time.Second)))
			desk.Wake("BTC/USD")
			venue.none()

			sells := sellsOf(desk, "BTC/USD")
			So(sells, ShouldHaveLength, 1)
			So(sells[0].Trigger, ShouldEqual, TriggerLearnedExit)

			venue.fill(exit)
			So(closures, ShouldHaveLength, 1)
			closure := closures[0]
			So(closure.Realized.Cmp(closure.Proceeds.Sub(closure.Cost).Sub(closure.Fees)), ShouldEqual, 0)
		})
	})
}

func TestDesk_PositionsWire(t *testing.T) {
	Convey("Given a filled position and a closed one", t, func() {
		desk, _, venue := deskFixture(200, 1000)
		So(desk.Enter("BTC/USD", matched), ShouldBeNil)
		venue.next()
		desk.Wake("BTC/USD")
		venue.none()

		frame := desk.PositionsWire()

		Convey("The open row carries capacity, budget, and venue beside shadow P&L", func() {
			So(frame.Rows, ShouldHaveLength, 1)
			holding := frame.Rows[0].Holding
			So(holding.CapacityDefined, ShouldBeTrue)
			So(holding.CapacityRatio, ShouldBeGreaterThan, 1)
			So(holding.SlippageBudget, ShouldAlmostEqual, 0.017, 1e-15)
			So(holding.BudgetSource, ShouldEqual, BudgetMatchedEdge)
			So(holding.HoldSeconds, ShouldEqual, 60)
			So(holding.VenuePnl, ShouldNotBeEmpty)
			So(holding.ShadowDefined, ShouldBeTrue)
			So(holding.ShadowPnl, ShouldNotBeEmpty)
		})

		Convey("A closed round trip is listed with its trigger", func() {
			So(desk.Exit("BTC/USD"), ShouldBeNil)
			venue.next()

			closed := desk.PositionsWire().Closed
			So(closed, ShouldHaveLength, 1)
			So(closed[0].Sells[0].Trigger, ShouldEqual, TriggerLearnedExit)
			So(closed[0].ShadowDefined, ShouldBeTrue)
		})
	})
}

func TestDesk_CapacityHysteresis(t *testing.T) {
	Convey("Given a filled position of the cash-bound size", t, func() {
		desk, depth, venue := deskFixture(200, 1000)
		So(desk.Enter("BTC/USD", matched), ShouldBeNil)
		bought := venue.next().volume
		window := time.Duration(core.Tolerance * float64(time.Minute))

		Convey("A shortfall that flickers back to ample within the window never sells", func() {
			for i, book := range []func(time.Time) *BookView{thinBids, handBookAt, thinBids, handBookAt, thinBids} {
				depth.push(book(deskNow.Add(time.Second + time.Duration(i)*window/2)))
				desk.Wake("BTC/USD")
				venue.none()
			}

			So(sellsOf(desk, "BTC/USD"), ShouldBeEmpty)
		})

		Convey("A shortfall within core.Tolerance of the open quantity never sells, however long it lasts", func() {
			slight := func(at time.Time) *BookView {
				view := thinBids(at)
				view.Bids = []BookLevel{{Price: 100, Quantity: 0.9 * bought.Float64()}, {Price: 50, Quantity: 100}}
				return view
			}

			persist(desk, depth, venue, slight, deskNow.Add(time.Second))
			venue.none()
			depth.push(slight(deskNow.Add(time.Second + 4*window)))
			desk.Wake("BTC/USD")
			venue.none()

			So(sellsOf(desk, "BTC/USD"), ShouldBeEmpty)
		})

		Convey("A material shortfall sells once the window has passed, and only once per window", func() {
			persist(desk, depth, venue, thinBids, deskNow.Add(time.Second))
			So(venue.next().side, ShouldEqual, "sell")

			thinner := func(at time.Time) *BookView {
				view := thinBids(at)
				view.Bids = []BookLevel{{Price: 100, Quantity: 0.2}, {Price: 50, Quantity: 100}}
				return view
			}

			// The first version after the trim starts a new run; it cannot sell.
			depth.push(thinner(deskNow.Add(time.Second + window + time.Second)))
			desk.Wake("BTC/USD")
			venue.none()

			So(sellsOf(desk, "BTC/USD"), ShouldHaveLength, 1)
		})
	})
}

func handBookAt(at time.Time) *BookView {
	view := handBook()
	view.At = at
	return view
}

func TestDesk_NoRepeatEntry(t *testing.T) {
	Convey("Given a Desk with a position being entered", t, func() {
		desk, depth, venue := deskFixture(200, 1000)
		So(desk.Enter("BTC/USD", matched), ShouldBeNil)
		first := venue.next()

		Convey("Another enter for the symbol is refused and places nothing", func() {
			So(desk.Enter("BTC/USD", matched), ShouldNotBeNil)
			venue.none()
		})

		Convey("Further book versions never buy past the sized target", func() {
			for i := 1; i <= 5; i++ {
				depth.push(handBookAt(deskNow.Add(time.Duration(i) * time.Second)))
				desk.Wake("BTC/USD")
			}

			desk.mu.Lock()
			held := desk.positions["BTC/USD"]
			So(held.bought.Add(held.buying).Cmp(first.volume) >= 0, ShouldBeTrue)

			if held.plan != nil {
				So(held.plan.submitted.Cmp(held.plan.target) <= 0, ShouldBeTrue)
			}

			desk.mu.Unlock()
		})
	})
}
