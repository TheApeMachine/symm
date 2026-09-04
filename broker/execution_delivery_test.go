package broker

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/learning"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

/*
deliveryNode is the broker-package analogue of the cmd package's executionNode:
it routes one EnvelopeExecution to Desk.StepExecution, so the test exercises the
exact production-shaped transport → workload → desk → position guardian feedback
path without importing the cmd wiring.
*/
type deliveryNode struct {
	desk *Desk
}

func (node deliveryNode) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || envelope.TypeID != types.EnvelopeExecution || node.desk == nil {
		return envelope
	}

	_ = node.desk.StepExecution(envelope.ExecutionData)
	return envelope
}

/*
executingConn is a mock transport whose AddOrder returns a synthetic order result
and — like the paper simulator — pushes a filled execution envelope into the
execution workload so the same delivery path the live wiring uses advances the
position. It captures submitted order requests so tests can assert the client
order ID survived the trip.
*/
type executingConn struct {
	*mockConn

	workload *nmruntime.Workload[*types.Envelope]

	mu        sync.Mutex
	submitted []*spot.AddOrderRequest
}

func (conn *executingConn) MarkReady() {}

func newExecutingConn(workload *nmruntime.Workload[*types.Envelope]) *executingConn {
	return &executingConn{
		mockConn: newMockConn(),
		workload: workload,
	}
}

/*
deliver pushes one filled execution record for the submitted order into the
execution workload, exactly as the paper transport publishes a fill.
*/
func (conn *executingConn) deliver(execution kraken.ExecutionData) {
	envelope := types.NewEnvelope(types.EnvelopeExecution)
	envelope.ExecutionData = execution
	conn.workload.Push(envelope)
}

func (conn *executingConn) AddOrder(order *spot.AddOrderRequest) (spot.AddOrderResult, error) {
	conn.mu.Lock()
	conn.submitted = append(conn.submitted, order)
	conn.mu.Unlock()

	return spot.AddOrderResult{
		OrderPlacementSingle: spot.OrderPlacementSingle{ID: []string{"venue-order-1"}},
	}, nil
}

func (conn *executingConn) SubInstrument(callback chan any) {
	// Seed one tradeable pair so the desk can resolve its instrument.
	callback <- &kraken.Instrument{Data: kraken.InstrumentData{
		Pairs: []kraken.InstrumentPair{
			{Symbol: "TEST/USD", Base: "TEST", Quote: "USD", Status: "online", TickSize: *decimal.NewFromFloat64(0.01)},
		},
	}}
}

func (conn *executingConn) Write(json.Marshaler, ...websocket.Callback[any]) error { return nil }

/*
newDeliveryDesk builds a Desk wired to a real execution workload that routes
fills through Desk.StepExecution, plus the position store the desk embeds.
*/
func newDeliveryDesk(t testing.TB, conn *executingConn) (*Desk, *nmruntime.Workload[*types.Envelope]) {
	t.Helper()

	// The desk's execution reducer reads the same configured L3 subscription
	// depth the websocket transport subscribes with. Tests that construct a
	// desk without booting the whole config get the documented default here.
	viper.SetDefault("market.l3_depth", 10)

	api := websocket.NewAPI(t.Context(), conn, conn)
	api.Normalizer().Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"TEST": {AltName: "TEST", Decimals: 8, DisplayDecimals: 8},
			"USD":  {AltName: "USD", Decimals: 2, DisplayDecimals: 2},
		},
		NewPairs: map[string]spot.AssetPair{
			"TESTUSD": {WSName: "TEST/USD", Base: "TEST", Quote: "USD", PairDecimals: 2, LotDecimals: 8, LotMultiplier: 1},
		},
	})

	price := NewPrice(api)
	instrument := NewInstrument(api, price)
	balance := NewBalance(api)
	store, err := NewPositionStore(
		t.TempDir()+"/positions.sqlite",
		testPositionStoreQueueDepth,
		testPositionStoreBatchSize,
	)
	if err != nil {
		t.Fatalf("open position store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	positions := &sync.Map{}
	desk, err := NewDesk(
		t.Context(), api, instrument, price, balance,
		nil, store, positions,
	)
	if err != nil {
		t.Fatalf("construct desk: %v", err)
	}

	workload := nmruntime.NewWorkload(t.Context(), "delivery", [][]nmruntime.Node[*types.Envelope]{
		{deliveryNode{desk: desk}},
	})
	workspace := nmruntime.NewWorkspace(
		t.Context(),
		"workspace",
		[][]nmruntime.Node[*types.Envelope]{{workload}},
	)
	workspace.Admit()

	// The test IS this workload's producer, so it declares the readiness a
	// transport would: a Workload holds WAITING from construction and drops
	// pushes until its producer admits it.
	t.Cleanup(func() { _ = workspace.Close() })

	return desk, workload
}

/*
entryExecution builds a filled entry fill whose ClientOrderID matches the
decision ID carried on the position's EntryOrder.
*/
func entryExecution(decisionID string) kraken.ExecutionData {
	return kraken.ExecutionData{
		OrderID:       "venue-order-1",
		ClientOrderID: decisionID,
		ExecID:        "entry-exec-1",
		Side:          "buy",
		Symbol:        "TEST/USD",
		OrderStatus:   "filled",
		LastQty:       mustDecimal("100000"),
		LastPrice:     mustDecimal("2.00"),
		CumQty:        mustDecimal("100000"),
		CumCost:       mustDecimal("200.00"),
		AvgPrice:      mustDecimal("2.00"),
		FeeUsdEquiv:   mustDecimal("0.80"),
		Timestamp:     time.Now(),
	}
}

/*
exitExecution builds a filled exit fill matching the exit order's ClOrdId.
*/
func exitExecution(exitClOrdID string) kraken.ExecutionData {
	return kraken.ExecutionData{
		OrderID:       "venue-order-2",
		ClientOrderID: exitClOrdID,
		ExecID:        "exit-exec-1",
		Side:          "sell",
		Symbol:        "TEST/USD",
		OrderStatus:   "filled",
		LastQty:       mustDecimal("100000"),
		LastPrice:     mustDecimal("2.10"),
		CumQty:        mustDecimal("100000"),
		CumCost:       mustDecimal("210.00"),
		AvgPrice:      mustDecimal("2.10"),
		FeeUsdEquiv:   mustDecimal("0.84"),
		Timestamp:     time.Now(),
	}
}

/*
TestExecutionDeliveryEndToEnd proves the transport → workload → desk → position
guardian feedback path: an entry fill delivered as an EnvelopeExecution reaches
Desk.StepExecution and opens the position, and the client order ID survives the
entire trip unchanged. It never calls onExecution directly and never marks the
position filled by hand.
*/
func TestExecutionDeliveryEndToEnd(t *testing.T) {
	Convey("Given a desk wired to a real execution workload", t, func() {
		conn := newExecutingConn(nil)
		desk, workload := newDeliveryDesk(t, conn)
		conn.workload = workload

		decision := types.Decision{
			ID:               "decision-1",
			Action:           types.ActionEnter,
			Symbol:           "TEST/USD",
			At:               time.Now(),
			ProposedQuantity: mustDecimal("100000"),
			ProposedNotional: mustDecimal("200.00"),
			ForecastHorizon:  1,
		}

		// A real, armed stoploss is required by the entry-fill path. It is
		// constructed from a ready forecast and an empty forward curve so the
		// geometry reduces to a one-tick floor below the entry mark.
		stoploss, err := types.NewStoplossWithPlan(
			t.Context(),
			"TEST/USD",
			mustDecimal("2.00"),
			mustDecimal("2.00"),
			&learning.RLSOutput{Ready: true},
			0,
			mustDecimal("0.01"),
			mustDecimal("0.008"),
			mustDecimal("0.008"),
			nil,
			time.Now(),
		)
		So(err, ShouldBeNil)
		decision.Stoploss = stoploss

		pair := kraken.InstrumentPair{Symbol: "TEST/USD", TickSize: *decimal.NewFromFloat64(0.01)}
		position := NewPosition(
			t.Context(), desk.api, desk.instrument, desk.price, desk.balance,
			desk.PositionStore, pair, decision,
		)
		desk.positions.Store(decision.Symbol, position)

		// Mirror the desk's Execute wiring: closing the position removes it
		// from the desk's open-position map.
		position.onClose = func() {
			desk.positions.CompareAndDelete(decision.Symbol, position)
		}

		Convey("an entry execution delivered through the workload opens the position", func() {
			_, err := position.Enter()
			So(err, ShouldBeNil)

			// Seed the executable mark before the fill lands, exactly as the
			// live path does: a ticker advances the position's mark before the
			// entry execution's RebindFill reads it.
			So(desk.StepTicker(kraken.TickerData{
				Symbol:    "TEST/USD",
				Bid:       decimal.NewFromFloat64(2.00),
				Ask:       decimal.NewFromFloat64(2.01),
				Timestamp: time.Now(),
			}), ShouldBeNil)

			conn.deliver(entryExecution(decision.ID))

			// The workload node routes the envelope to Desk.StepExecution,
			// which dispatches to the guardian ring asynchronously.
			deadline := time.Now().Add(3 * time.Second)
			for position.status() != types.OPEN && position.status() != types.FILLED && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}

			status := position.status()
			So(status == types.OPEN || status == types.FILLED, ShouldBeTrue)
			So(position.Holding.Qty.Cmp(mustDecimal("100000")), ShouldEqual, 0)

			Convey("the client order ID survived the trip from decision to fill", func() {
				conn.mu.Lock()
				defer conn.mu.Unlock()

				So(len(conn.submitted), ShouldEqual, 1)
				So(conn.submitted[0].ClOrdId, ShouldEqual, decision.ID)
			})

			Convey("a delivered exit execution closes the position and removes it from the desk", func() {
				exitOrder := &spot.AddOrderRequest{
					ClOrdId: "decision-1-exit",
					Type:    "sell",
					Volume:  "100000",
					Pair:    "TEST/USD",
				}
				position.ExitOrder = exitOrder

				conn.deliver(exitExecution(exitOrder.ClOrdId))

				deadline := time.Now().Add(3 * time.Second)
				for position.status() != types.CLOSED && time.Now().Before(deadline) {
					time.Sleep(5 * time.Millisecond)
				}

				So(position.status(), ShouldEqual, types.CLOSED)
				So(position.Holding.SellableQty.Sign(), ShouldEqual, 0)

				_, found := desk.positions.Load(decision.Symbol)
				So(found, ShouldBeFalse)
			})
		})
	})
}

/*
TestOpenPositionWire proves the positions panel's data source: OpenPositionWire
projects every open lot (recovered or live) and excludes closed lots, so the
wire snapshot is exactly what the frontend renders.
*/
func TestOpenPositionWire(t *testing.T) {
	Convey("Given a desk holding one open and one closed lot", t, func() {
		conn := newExecutingConn(nil)
		desk, workload := newDeliveryDesk(t, conn)
		conn.workload = workload

		openDecision := types.Decision{
			ID:               "open-1",
			Action:           types.ActionEnter,
			Symbol:           "TEST/USD",
			At:               time.Now(),
			ProposedQuantity: mustDecimal("100000"),
			ProposedNotional: mustDecimal("200.00"),
			ForecastHorizon:  1,
		}

		openPosition := NewPosition(
			t.Context(), desk.api, desk.instrument, desk.price, desk.balance,
			desk.PositionStore, kraken.InstrumentPair{Symbol: "TEST/USD", TickSize: *decimal.NewFromFloat64(0.01)},
			openDecision,
		)
		desk.positions.Store("TEST/USD", openPosition)

		closedDecision := types.Decision{
			ID:               "closed-1",
			Action:           types.ActionEnter,
			Symbol:           "TEST/USD",
			At:               time.Now(),
			ProposedQuantity: mustDecimal("100000"),
			ProposedNotional: mustDecimal("200.00"),
			ForecastHorizon:  1,
		}
		closedPosition := NewPosition(
			t.Context(), desk.api, desk.instrument, desk.price, desk.balance,
			desk.PositionStore, kraken.InstrumentPair{Symbol: "TEST/USD", TickSize: *decimal.NewFromFloat64(0.01)},
			closedDecision,
		)
		closedPosition.setStatus(types.CLOSED)
		desk.positions.Store("TEST/USD-closed", closedPosition)

		Convey("OpenPositionWire returns the open lot and its frozen entry decision", func() {
			wire := desk.OpenPositionWire()

			So(len(wire), ShouldEqual, 1)
			So(wire[0].Holding.Symbol, ShouldEqual, "TEST/USD")
			So(wire[0].Decision, ShouldNotBeNil)
			So(wire[0].Decision.Id, ShouldEqual, "open-1")
			So(wire[0].Decision.Action, ShouldEqual, "enter")
		})
	})
}
