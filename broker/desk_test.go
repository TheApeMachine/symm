package broker

import (
	"fmt"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/mock"
	"github.com/theapemachine/symm/types"
)

func TestDeskExecute(t *testing.T) {
	Convey("Given normal capacity already occupied", t, func() {
		desk := deskFixture(t)
		desk.positions.Store("NORMAL1/USD", &Position{Status: types.OPEN})
		desk.positions.Store("NORMAL2/USD", &Position{Status: types.OPEN})

		Convey("It should reject a normal entry and admit only two reserve opportunities", func() {
			normal := deskDecisionFixture(t, desk, "NORMAL3/USD", false)
			So(desk.Execute(normal), ShouldNotBeNil)
			So(desk.OpenPositions(), ShouldEqual, desk.maxPositions)

			firstReserve := deskDecisionFixture(t, desk, "RESERVE1/USD", true)
			secondReserve := deskDecisionFixture(t, desk, "RESERVE2/USD", true)
			overflow := deskDecisionFixture(t, desk, "RESERVE3/USD", true)
			So(desk.Execute(firstReserve), ShouldBeNil)
			So(desk.Execute(secondReserve), ShouldBeNil)
			So(desk.Execute(overflow), ShouldNotBeNil)
			So(desk.OpenPositions(), ShouldEqual, desk.maxPositions+desk.maxReserved)

			stored, found := desk.positions.Load(firstReserve.Symbol)
			So(found, ShouldBeTrue)
			So(stored.(*Position).Holding.IsOpportunity, ShouldBeTrue)
		})
	})

	Convey("Given simultaneous opportunity entries", t, func() {
		desk := deskFixture(t)
		candidateCount := desk.maxPositions + desk.maxReserved + 1
		decisions := make([]types.Decision, 0, candidateCount)

		for index := range candidateCount {
			decisions = append(decisions, deskDecisionFixture(
				t, desk, fmt.Sprintf("FAST%d/USD", index), true,
			))
		}

		Convey("The atomic execution boundary should cap the book at total capacity", func() {
			var waitGroup sync.WaitGroup
			errors := make(chan error, candidateCount)

			for _, decision := range decisions {
				waitGroup.Add(1)

				go func(decision types.Decision) {
					defer waitGroup.Done()
					errors <- desk.Execute(decision)
				}(decision)
			}

			waitGroup.Wait()
			close(errors)
			rejected := 0

			for err := range errors {
				if err != nil {
					rejected++
				}
			}

			So(rejected, ShouldEqual, candidateCount-(desk.maxPositions+desk.maxReserved))
			So(desk.OpenPositions(), ShouldEqual, desk.maxPositions+desk.maxReserved)
		})
	})
}

func TestDeskOpenSlots(t *testing.T) {
	Convey("Given two normal slots and two reserve slots", t, func() {
		desk := deskFixture(t)
		desk.positions.Store("NORMAL1/USD", &Position{Status: types.OPEN})
		desk.positions.Store("NORMAL2/USD", &Position{Status: types.PENDING})

		Convey("Normal entries should stop while opportunities retain the reserve", func() {
			So(desk.OpenSlots(false), ShouldEqual, 0)
			So(desk.OpenSlots(true), ShouldEqual, desk.maxReserved)
		})

		Convey("Slot counts should never become negative above capacity", func() {
			for index := range desk.maxReserved + 1 {
				symbol := fmt.Sprintf("EXCESS%d/USD", index)
				desk.positions.Store(symbol, &Position{Status: types.OPEN})
			}

			So(desk.OpenSlots(false), ShouldEqual, 0)
			So(desk.OpenSlots(true), ShouldEqual, 0)
		})
	})
}

func BenchmarkDeskOpenSlots(b *testing.B) {
	desk := deskFixture(b)
	desk.positions.Store("NORMAL1/USD", &Position{Status: types.OPEN})
	desk.positions.Store("NORMAL2/USD", &Position{Status: types.OPEN})

	for b.Loop() {
		_ = desk.OpenSlots(true)
	}
}

func deskFixture(t testing.TB) *Desk {
	t.Helper()
	api := websocket.NewAPI(t.Context(), mock.NewConn(), mock.NewConn())

	return &Desk{
		ctx:          t.Context(),
		api:          api,
		instrument:   &Instrument{cache: &sync.Map{}},
		positions:    &sync.Map{},
		maxPositions: 2,
		maxReserved:  2,
	}
}

func deskDecisionFixture(
	t testing.TB,
	desk *Desk,
	symbol string,
	opportunity bool,
) types.Decision {
	t.Helper()
	pair := kraken.InstrumentPair{
		Symbol:   symbol,
		Base:     symbol,
		Quote:    "USD",
		TickSize: *decimal.NewFromFloat64(0.01),
	}
	desk.instrument.cache.Store(symbol, pair)
	forecast, err := types.NewResonanceForecast(
		[]float64{-0.01, 0.03},
		[]float64{1, 1},
		2,
		0.90,
	)

	if err != nil {
		t.Fatalf("forecast: %v", err)
	}

	entry := decimal.NewFromFloat64(100.02)
	mark := decimal.NewFromFloat64(100)
	zeroRate := decimal.NewFromInt64(0)
	stoploss, err := types.NewStoploss(
		t.Context(),
		symbol,
		entry,
		mark,
		forecast,
		&pair.TickSize,
		zeroRate,
		zeroRate,
	)

	if err != nil {
		t.Fatalf("stoploss: %v", err)
	}

	decision := types.NewDecision(types.ActionEnter, symbol)
	decision.Opportunity = opportunity
	decision.ProposedQuantity = decimal.NewFromInt64(1)
	decision.EntryPrice = entry
	decision.Mark = mark
	decision.Stoploss = stoploss

	return *decision
}
