package broker

import (
	"fmt"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/mock"
	"github.com/theapemachine/symm/types"
)

func brokerForecast(value float64) *learning.RLSOutput {
	return &learning.RLSOutput{Value: value, Ready: true, Scale: 0.01, DegreesOfFreedom: 1}
}

func TestDeskExecute(t *testing.T) {
	Convey("Given a decision whose long thesis is no longer supported", t, func() {
		desk := deskFixture(t)
		decision := deskDecisionFixture(t, desk, "MARGINAL/USD", false)
		decision.Direction = -1
		decision.ThesisScore = -0.2

		Convey("It should refuse the order at the final atomic execution boundary", func() {
			err := desk.Execute(decision)

			So(err, ShouldNotBeNil)
			So(desk.OpenPositions(), ShouldEqual, 0)
		})
	})

	Convey("Given an order larger than the current complete best quotes", t, func() {
		desk := deskFixture(t)
		decision := deskDecisionFixture(t, desk, "DEPTH/USD", false)
		decision.ProposedQuantity = decimal.NewFromFloat64(2)
		tick := desk.price.Tick(decision.Symbol)
		tick.AskQty = 1
		tick.BidQty = 1

		Convey("It should refuse to treat unknown depth impact as zero", func() {
			err := desk.Execute(decision)

			So(err, ShouldNotBeNil)
			So(desk.OpenPositions(), ShouldEqual, 0)
		})
	})

	Convey("Given normal capacity already occupied", t, func() {
		desk := deskFixture(t)
		normalOne := &Position{}
		normalOne.setStatus(types.OPEN)
		normalTwo := &Position{}
		normalTwo.setStatus(types.OPEN)
		desk.positions.Store("NORMAL1/USD", normalOne)
		desk.positions.Store("NORMAL2/USD", normalTwo)

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
			position := stored.(*Position)
			So(position.Holding.IsOpportunity, ShouldBeTrue)
			So(position.Decision.OpportunityMargin, ShouldBeGreaterThan, 0)
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

	Convey("Given one accepted entry", t, func() {
		desk := deskFixture(t)
		decision := deskDecisionFixture(t, desk, "RETAINED/USD", false)

		Convey("It should retain the submitted position on the thesis symbol", func() {
			So(desk.Execute(decision), ShouldBeNil)
			retained, found := desk.thesis.Symbol(decision.Symbol).Positions.Load(
				decision.ID,
			)

			So(found, ShouldBeTrue)
			So(retained, ShouldEqual, mustDeskPosition(desk, decision.Symbol))
		})
	})

	Convey("Given repeated admission of the same still-open symbol", t, func() {
		desk := deskFixture(t)
		decision := deskDecisionFixture(t, desk, "ONCE/USD", false)

		Convey("It should checkpoint and submit the accepted entry exactly once", func() {
			So(desk.Execute(decision), ShouldBeNil)
			So(desk.Execute(decision), ShouldBeNil)
			var checkpoints int
			So(desk.database.QueryRow(
				"SELECT COUNT(*) FROM thesis_checkpoints",
			).Scan(&checkpoints), ShouldBeNil)
			So(checkpoints, ShouldEqual, 1)
		})
	})
}

func TestDeskHolding(t *testing.T) {
	Convey("Given a desk whose position registry is not initialized", t, func() {
		Convey("It should report no inventory rather than panic", func() {
			So((&Desk{}).Holding("BTC/USD"), ShouldEqual, 0)
			So((*Desk)(nil).Holding("BTC/USD"), ShouldEqual, 0)
		})
	})

	Convey("Given open lots in two symbols", t, func() {
		desk := deskFixture(t)
		bitcoin := &Position{pair: kraken.InstrumentPair{Symbol: "BTC/USD"}}
		bitcoin.setStatus(types.OPEN)
		ether := &Position{pair: kraken.InstrumentPair{Symbol: "ETH/USD"}}
		ether.setStatus(types.OPEN)
		desk.positions.Store("BTC/USD", bitcoin)
		desk.positions.Store("ETH/USD", ether)

		Convey("It should report only the requested symbol inventory", func() {
			So(desk.Holding("BTC/USD"), ShouldEqual, 1)
			So(desk.Holding("ETH/USD"), ShouldEqual, 1)
			So(desk.Holding("ADA/USD"), ShouldEqual, 0)
		})
	})
}

func TestDeskQueued(t *testing.T) {
	Convey("Given a desk with no pending market messages", t, func() {
		desk := &Desk{}

		Convey("It should report an empty queue", func() {
			So(desk.Queued(), ShouldEqual, 0)
			So((*Desk)(nil).Queued(), ShouldEqual, 0)
		})
	})
}

func TestDeskPublishEquity(t *testing.T) {
	Convey("Given a refreshed complete broker valuation", t, func() {
		desk, thesis, observer := equityDeskFixture(t)

		err := desk.PublishEquity()
		equity, exists := thesis.Equity()

		Convey("It should retain equity and notify the regulator", func() {
			So(err, ShouldBeNil)
			So(exists, ShouldBeTrue)
			So(equity.Equity.Float64(), ShouldEqual, 200.0)
			So(equity.UnrealizedPnL.Float64(), ShouldEqual, -5.0)
			So(observer.updates, ShouldEqual, 1)
			So(observer.exposure, ShouldBeFalse)
		})
	})

	Convey("Given a broker valuation while an execution owns mutable position state", t, func() {
		desk, _, observer := equityDeskFixture(t)
		position := &Position{}
		position.setStatus(types.PENDING)
		desk.positions.Store("SIM/USD", position)
		stop := make(chan struct{})
		stopped := make(chan struct{})

		go func() {
			defer close(stopped)

			for {
				select {
				case <-stop:
					return
				default:
					position.setStatus(types.OPEN)
					position.setStatus(types.PENDING)
				}
			}
		}()

		err := desk.PublishEquity()
		close(stop)
		<-stopped

		Convey("It should read lifecycle exposure atomically while the lot changes", func() {
			So(err, ShouldBeNil)
			So(observer.exposure, ShouldBeTrue)
		})
	})
}

func TestDeskOpenSlots(t *testing.T) {
	Convey("Given two normal slots and two reserve slots", t, func() {
		desk := deskFixture(t)
		normalOne := &Position{}
		normalOne.setStatus(types.OPEN)
		normalTwo := &Position{}
		normalTwo.setStatus(types.PENDING)
		desk.positions.Store("NORMAL1/USD", normalOne)
		desk.positions.Store("NORMAL2/USD", normalTwo)

		Convey("Normal entries should stop while opportunities retain the reserve", func() {
			So(desk.OpenSlots(false), ShouldEqual, 0)
			So(desk.OpenSlots(true), ShouldEqual, desk.maxReserved)
		})

		Convey("Slot counts should never become negative above capacity", func() {
			for index := range desk.maxReserved + 1 {
				symbol := fmt.Sprintf("EXCESS%d/USD", index)
				position := &Position{}
				position.setStatus(types.OPEN)
				desk.positions.Store(symbol, position)
			}

			So(desk.OpenSlots(false), ShouldEqual, 0)
			So(desk.OpenSlots(true), ShouldEqual, 0)
		})
	})
}

func BenchmarkDeskOpenSlots(b *testing.B) {
	desk := deskFixture(b)
	normalOne := &Position{}
	normalOne.setStatus(types.OPEN)
	normalTwo := &Position{}
	normalTwo.setStatus(types.OPEN)
	desk.positions.Store("NORMAL1/USD", normalOne)
	desk.positions.Store("NORMAL2/USD", normalTwo)

	for b.Loop() {
		_ = desk.OpenSlots(true)
	}
}

func BenchmarkDeskPublishEquity(b *testing.B) {
	desk, _, _ := equityDeskFixture(b)
	b.ReportAllocs()

	for b.Loop() {
		if err := desk.PublishEquity(); err != nil {
			b.Fatal(err)
		}
	}
}

type equityConn struct {
	*mock.Conn
	tradeBalance kraken.TradeBalanceResult
}

type equityObserver struct {
	updates  int
	exposure bool
}

func (observer *equityObserver) Update(_ *types.Thesis, exposure bool) error {
	observer.updates++
	observer.exposure = exposure
	return nil
}

func (conn *equityConn) TradeBalance() (kraken.TradeBalanceResult, error) {
	return conn.tradeBalance, nil
}

func equityDeskFixture(
	testingTB testing.TB,
) (*Desk, *types.Thesis, *equityObserver) {
	testingTB.Helper()
	public := mock.NewConn()
	private := &equityConn{
		Conn: mock.NewConn(),
		tradeBalance: kraken.TradeBalanceResult{
			Equity:        decimal.NewFromInt64(200),
			UnrealizedPnL: decimal.NewFromInt64(-5),
		},
	}
	api := websocket.NewAPI(testingTB.Context(), public, private)
	thesis := types.NewThesis(testingTB.Context(), nil)
	observer := &equityObserver{}
	balance := &Balance{wallet: &sync.Map{}, quote: "USD"}
	balance.replace(map[string]*decimal.Decimal{
		"USD": decimal.NewFromInt64(150),
	})
	desk := &Desk{
		api:            api,
		balance:        balance,
		thesis:         thesis,
		equityObserver: observer,
		positions:      &sync.Map{},
		ui:             make(chan []byte, 1),
	}

	return desk, thesis, observer
}

func deskFixture(t testing.TB) *Desk {
	t.Helper()
	api := websocket.NewAPI(t.Context(), mock.NewConn(), mock.NewConn())
	store := newPositionStoreFixture(t)

	return &Desk{
		PositionStore: store,
		ctx:           t.Context(),
		api:           api,
		instrument:    &Instrument{cache: &sync.Map{}},
		price:         NewPrice(api),
		thesis:        types.NewThesis(t.Context(), nil),
		positions:     &sync.Map{},
		maxPositions:  2,
		maxReserved:   2,
	}
}

func mustDeskPosition(desk *Desk, symbol string) *Position {
	value, found := desk.positions.Load(symbol)

	if !found {
		panic("broker test: expected desk position")
	}

	return value.(*Position)
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
	desk.price.fees.Store(symbol, kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.25),
	})
	desk.price.Update(&kraken.TickerData{
		Symbol: symbol,
		Ask:    decimal.NewFromFloat64(100.02),
		AskQty: 10,
		Bid:    decimal.NewFromFloat64(100),
		BidQty: 10,
	})
	forecast := brokerForecast(0.05)

	entry := decimal.NewFromFloat64(100.02)
	mark := decimal.NewFromFloat64(100)
	zeroRate := decimal.NewFromInt64(0)
	stoploss, err := types.NewStoploss(
		t.Context(),
		symbol,
		entry,
		mark,
		forecast,
		nil,
		&pair.TickSize,
		zeroRate,
		zeroRate,
	)

	if err != nil {
		t.Fatalf("stoploss: %v", err)
	}

	decision := types.NewDecision(types.ActionEnter, symbol)
	decision.Opportunity = opportunity
	decision.Direction = 1
	decision.ThesisScore = 0.6
	decision.ThesisConfidence = 0.8
	decision.ThesisSupport = 0.8
	decision.ThesisContradiction = 0.2
	decision.Forecast = forecast
	decision.PerspectiveReturn = forecast.Value
	decision.AdmissionUtilityThreshold = -1
	decision.ProposedQuantity = decimal.NewFromInt64(1)
	decision.EntryPrice = entry
	decision.Mark = mark
	decision.Stoploss = stoploss

	return *decision
}
