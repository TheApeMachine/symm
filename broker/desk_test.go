package broker

import (
	"errors"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestDeskExecute(t *testing.T) {
	Convey("An entry racing an existing lot is explicitly refused", t, func() {
		desk, position := saturatedPosition(t)
		err := desk.Execute(position.Decision)
		var refusal *types.ExecutionRefusal
		So(errors.As(err, &refusal), ShouldBeTrue)
		So(refusal.State, ShouldEqual, "account changed")
		stored, found := desk.positions.Load("TEST/USD")
		So(found, ShouldBeTrue)
		So(stored, ShouldEqual, position)
	})
}

/*
saturatedPosition builds a Position whose guardian transport is unavailable —
the disruptor is nil, so publishGuardian deterministically fails to reserve a
slot. This is the "priority event cannot enter the guardian" boundary the desk
must surface as a non-nil error rather than a log line.
*/
func saturatedPosition(t *testing.T) (*Desk, *Position) {
	t.Helper()

	conn := newExecutingConn(nil)
	desk, workload := newDeliveryDesk(t, conn)
	conn.workload = workload

	decision := types.Decision{
		ID:               "saturated-1",
		Action:           types.ActionEnter,
		Symbol:           "TEST/USD",
		ProposedQuantity: mustDecimal("100000"),
		ProposedNotional: mustDecimal("200.00"),
		ForecastHorizon:  1,
	}

	position := &Position{
		Holding:  &types.Holding{Symbol: "TEST/USD"},
		Decision: decision,
	}
	position.setStatus(types.OPEN)
	desk.positions.Store("TEST/USD", position)

	return desk, position
}

/*
TestDeskGuardianSaturationError proves the guardian-saturation failure contract:
when a priority event cannot reserve a guardian slot, the caller receives a
non-nil error (never a log-and-success), and no emergency action is enqueued.
*/
func TestDeskGuardianSaturationError(t *testing.T) {
	Convey("Given a position whose guardian transport cannot reserve a slot", t, func() {
		desk, _ := saturatedPosition(t)

		Convey("StepTicker returns the saturation error rather than success", func() {
			err := desk.StepTicker(kraken.TickerData{
				Symbol: "TEST/USD",
				Bid:    decimal.NewFromFloat64(2.00),
			})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "guardian")
		})

		Convey("StepExecution returns the saturation error rather than success", func() {
			err := desk.StepExecution(kraken.ExecutionData{Symbol: "TEST/USD"})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "guardian")
		})

		Convey("StepLevel3 returns the saturation error rather than success", func() {
			err := desk.StepLevel3(kraken.Level3Data{Symbol: "TEST/USD"})

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "guardian")
		})
	})
}

func TestDeskPositionQuantity(t *testing.T) {
	Convey("Given a desk with open positions", t, func() {
		desk := &Desk{positions: &sync.Map{}}

		Convey("with no positions, position quantity is zero", func() {
			So(desk.PositionQuantity("TEST/USD"), ShouldEqual, 0)
		})

		Convey("with open positions, position quantity aggregates held base units", func() {
			pos := &Position{
				pair: kraken.InstrumentPair{Symbol: "TEST/USD"},
				Holding: &types.Holding{
					Qty: mustDecimal("15"),
				},
			}
			pos.setStatus(types.OPEN)
			desk.positions.Store("TEST/USD", pos)

			So(desk.PositionQuantity("TEST/USD"), ShouldEqual, 15.0)

			pos.setStatus(types.CLOSED)
			So(desk.PositionQuantity("TEST/USD"), ShouldEqual, 0.0)
		})
	})
}

/*
newLevel3GuardianDesk builds a desk with one open lot on a priced instrument,
so guardian-ring measurements run against a position the L3 path will actually
mark. The lot carries no protective geometry: exits are commanded by whoever
opened it, and nothing in this path closes a position.
*/
func newLevel3GuardianDesk(t testing.TB) (*Desk, *Position) {
	t.Helper()

	conn := newExecutingConn(nil)
	desk, workload := newDeliveryDesk(t, conn)
	conn.workload = workload

	// Seed a taker-fee row so the executable surface can be priced; without it
	// the book reports incomplete and the lot is never marked.
	desk.price.fees.Store("TEST/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.25),
	})

	entry := mustDecimal("2.00")
	decision := types.Decision{
		ID:               "l3-guardian-1",
		Action:           types.ActionEnter,
		Symbol:           "TEST/USD",
		ProposedQuantity: mustDecimal("100000"),
		ProposedNotional: mustDecimal("200.00"),
	}

	pair := kraken.InstrumentPair{Symbol: "TEST/USD", TickSize: *decimal.NewFromFloat64(0.01)}
	position := NewPosition(
		t.Context(), desk.api, desk.instrument, desk.price, desk.balance,
		desk.PositionStore, pair, decision,
	)
	position.Holding.Qty = mustDecimal("100000")
	position.Holding.SellableQty = mustDecimal("100000")
	position.Holding.EntryPrice = entry
	position.Holding.EntryFee = mustDecimal("0")
	position.setStatus(types.OPEN)
	position.Holding.Status = types.OPEN

	desk.positions.Store("TEST/USD", position)

	return desk, position
}

func coherentL3(symbol, bid, ask string) kraken.Level3Data {
	return kraken.Level3Data{
		Symbol: symbol,
		Type:   "snapshot",
		Bids:   []kraken.Level3Order{mustDecimalOrder("b1", bid, "100000")},
		Asks:   []kraken.Level3Order{mustDecimalOrder("a1", ask, "100000")},
	}
}

/*
BenchmarkGuardianEventLatency measures the round-trip cost of publishing one
priority L3 event through the guardian ring and having the dedicated consumer
handle it — the guardian event latency under a steady single-producer load.
*/
func BenchmarkGuardianEventLatency(b *testing.B) {
	desk, _ := newLevel3GuardianDesk(b)
	frame := coherentL3("TEST/USD", "2.50", "2.51")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := desk.StepLevel3(frame); err != nil {
			b.Fatal(err)
		}
	}
}

/*
BenchmarkGuardianBurstCapacity measures the cost of publishing a full burst of
guardianCapacity priority events against a live consumer — the configured
1024-slot saturation boundary — to establish whether the ring and handler keep
up without dropping or coalescing.
*/
func BenchmarkGuardianBurstCapacity(b *testing.B) {
	desk, _ := newLevel3GuardianDesk(b)
	frame := coherentL3("TEST/USD", "2.50", "2.51")
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for range guardianCapacity {
			if err := desk.StepLevel3(frame); err != nil {
				b.Fatal(err)
			}
		}
	}
}
