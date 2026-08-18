package resonance

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

const testAlpha = 0.05

/*
stubConn satisfies the transport contract without a venue behind it; its
channels stay nil so the solver's event loop simply idles and Update is driven
directly.
*/
type stubConn struct{}

func (conn stubConn) Status() types.Status { return types.READY }

func (conn stubConn) Subscribe(
	_ string, subscription *types.Subscription[any],
) *types.Subscription[any] {
	return subscription
}

func (conn stubConn) Books() *sync.Map { return &sync.Map{} }

func (conn stubConn) Book(_ string, _ func(*book.Book)) {}

func (conn stubConn) Level3Divergences() <-chan string { return nil }

func (conn stubConn) ResubscribeL3(_ string) {}

func (conn stubConn) SubInstrument(_ types.Subscription[any]) {}

func (conn stubConn) SubTicker(_ []string) {}

func (conn stubConn) SubBook(_ []string) {}

func (conn stubConn) SubTrades(_ []string) {}

func (conn stubConn) SubL3(_ []string) {}

func (conn stubConn) SubCandles(_ []string) {}

func (conn stubConn) Balance() (map[string]*decimal.Decimal, error) { return nil, nil }

func (conn stubConn) TradesHistory() (spot.TradesHistoryResult, error) {
	return spot.TradesHistoryResult{}, nil
}

func (conn stubConn) TradeBalance() (kraken.TradeBalanceResult, error) {
	return kraken.TradeBalanceResult{}, nil
}

func (conn stubConn) TradeVolume(_ []string) (*kraken.TradeVolumeResult, error) {
	return nil, nil
}

func (conn stubConn) AddOrder(_ *spot.AddOrderRequest) (spot.AddOrderResult, error) {
	return spot.AddOrderResult{}, nil
}

func (conn stubConn) OpenOrders() (spot.OpenOrdersResult, error) {
	return spot.OpenOrdersResult{}, nil
}

func (conn stubConn) CancelOrder(_ *spot.CancelOrderRequest) (spot.CancelResult, error) {
	return spot.CancelResult{}, nil
}

func (conn stubConn) Write(_ json.Marshaler, _ ...websocket.Callback[any]) error {
	return nil
}

func (conn stubConn) Post(_ string, _ json.Marshaler) ([]byte, error) {
	return nil, nil
}

func (conn stubConn) Client() *spot.WebSocket { return nil }

func (conn stubConn) Close() {}

func TestNewSolver(t *testing.T) {
	Convey("Given a configured pace and idle transport", t, func() {
		ui := make(chan []byte, 16)
		thesis := types.NewThesis(t.Context(), nil)
		solver := NewSolver(
			t.Context(), testAlpha,
			websocket.NewAPI(t.Context(), stubConn{}, stubConn{}),
			ui, thesis,
		)

		Convey("It should retain the pace and create an empty detector registry", func() {
			So(solver.pace, ShouldEqual, testAlpha)

			_, found := solver.detectors.Load("BTC/USD")
			So(found, ShouldBeFalse)
		})
	})
}

func TestUpdate(t *testing.T) {
	Convey("Given a solver over an idle transport", t, func() {
		ui := make(chan []byte, 16)
		thesis := types.NewThesis(t.Context(), nil)
		solver := NewSolver(
			t.Context(), testAlpha,
			websocket.NewAPI(t.Context(), stubConn{}, stubConn{}),
			ui, thesis,
		)
		at := time.Unix(1_786_099_200, 0).UTC()

		Convey("It should step the coder and publish the frontend frame", func() {
			So(solver.Update("BTC/USD", at, make([]float64, 11)), ShouldBeNil)
			So(solver.Update("BTC/USD", at.Add(time.Second), make([]float64, 11)), ShouldBeNil)

			var payload []byte

			for len(ui) > 0 {
				payload = <-ui
			}

			So(payload, ShouldNotBeEmpty)

			var frame struct {
				Resonance struct {
					Source   string    `json:"source"`
					Symbol   string    `json:"symbol"`
					At       string    `json:"at"`
					Latent   []float64 `json:"latent"`
					Energy   *float64  `json:"energy"`
					Surprise *float64  `json:"surprise"`
				} `json:"resonance"`
			}
			So(json.Unmarshal(payload, &frame), ShouldBeNil)
			So(frame.Resonance.Source, ShouldEqual, string(types.SourceResonance))
			So(frame.Resonance.Symbol, ShouldEqual, "BTC/USD")
			So(frame.Resonance.At, ShouldNotBeEmpty)
			So(frame.Resonance.Latent, ShouldNotBeEmpty)
			So(frame.Resonance.Energy, ShouldNotBeNil)
			So(frame.Resonance.Surprise, ShouldNotBeNil)
		})

		Convey("A later observation of different width should fail the step", func() {
			So(solver.Update("BTC/USD", at, make([]float64, 11)), ShouldBeNil)
			So(solver.Update("BTC/USD", at.Add(time.Second), make([]float64, 12)), ShouldNotBeNil)
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Given a running solver", t, func() {
		solver := NewSolver(
			context.Background(), testAlpha,
			websocket.NewAPI(context.Background(), stubConn{}, stubConn{}),
			make(chan []byte, 16),
			types.NewThesis(context.Background(), nil),
		)

		Convey("It should close without error", func() {
			So(solver.Close(), ShouldBeNil)
		})
	})
}
