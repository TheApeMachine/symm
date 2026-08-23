package resonance

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
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

func (conn stubConn) Books() *sync.Map { return &sync.Map{} }

func (conn stubConn) Book(_ string, _ func(*book.Book)) {}

func (conn stubConn) Level3Divergences() <-chan string { return nil }

func (conn stubConn) ResubscribeL3(_ string) {}

func (conn stubConn) SubInstrument(_ chan any) {}

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
		ui := transport.NewMapReduce[*types.UIFrame](nil, nil, nil)
		thesis := types.NewThesis(t.Context(), ui)
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

func TestRunDrainsTickerBurst(t *testing.T) {
	Convey("Given a running solver on an idle transport", t, func() {
		consumer := transport.NewConsumer[*types.UIFrame](
			resonanceTestConsumer, func() {},
		)
		ui := transport.NewMapReduce[*types.UIFrame](
			[]*transport.Consumer[*types.UIFrame]{consumer},
			nil,
			nil,
		)
		thesis := types.NewThesis(t.Context(), ui)
		solver := NewSolver(
			t.Context(), testAlpha,
			websocket.NewAPI(t.Context(), stubConn{}, stubConn{}),
			ui, thesis,
		)
		defer solver.Close()
		symbol := thesis.Symbol("BTC/USD")

		const burstCount = 64

		for index := range burstCount {
			price := decimal.NewFromInt64(int64(index + 1))
			symbol.AppendTicker(kraken.TickerData{
				Symbol: "BTC/USD", Timestamp: time.Unix(int64(index+1), 0).UTC(),
				Ask: price, Bid: price, High: price, Low: price, Last: price,
				Change: decimal.NewFromInt64(0),
			})
		}

		Convey("It should drain a burst of ticker observations through the real run loop", func() {
			drained := make(chan error, 1)

			go func() {
				count := 0
				deadline := time.Now().Add(5 * time.Second)

				for {
					select {
					case <-solver.ctx.Done():
						drained <- fmt.Errorf("solver stopped before draining")
						return
					default:
					}

					if _, ok := ui.Pop(consumer); ok {
						count++

						if count == burstCount {
							drained <- nil
							return
						}

						continue
					}

					if time.Now().After(deadline) {
						drained <- fmt.Errorf("drained %d of %d ticker events", count, burstCount)
						return
					}

					time.Sleep(time.Millisecond)
				}
			}()
			So(<-drained, ShouldBeNil)
		})
	})
}

func TestUpdate(t *testing.T) {
	Convey("Given a solver over an idle transport", t, func() {
		consumer := transport.NewConsumer[*types.UIFrame](
			resonanceTestConsumer, func() {},
		)
		ui := transport.NewMapReduce[*types.UIFrame](
			[]*transport.Consumer[*types.UIFrame]{consumer},
			nil,
			nil,
		)
		thesis := types.NewThesis(t.Context(), ui)
		solver := NewSolver(
			t.Context(), testAlpha,
			websocket.NewAPI(t.Context(), stubConn{}, stubConn{}),
			ui, thesis,
		)
		at := time.Unix(1_786_099_200, 0).UTC()

		Convey("It should step the coder and publish the frontend frame", func() {
			So(solver.Update("BTC/USD", at, make([]float64, 11)), ShouldBeNil)
			So(solver.Update("BTC/USD", at.Add(time.Second), make([]float64, 11)), ShouldBeNil)

			var payload *types.UIFrame

			for {
				frame, ok := ui.Pop(consumer)

				if !ok {
					break
				}

				payload = frame
			}

			So(payload, ShouldNotBeEmpty)

			envelope := payload
			So(envelope.Type, ShouldEqual, wire.FrameResonanceFrame)
			rows := envelope.Value.(*wire.ResonanceFrameT).Rows
			So(rows, ShouldHaveLength, 1)
			So(rows[0].Source, ShouldEqual, string(types.SourceResonance))
			So(rows[0].Symbol, ShouldEqual, "BTC/USD")
			So(rows[0].At, ShouldNotEqual, int64(0))
			So(rows[0].Latent, ShouldNotBeEmpty)
			So(rows[0].Layers, ShouldNotBeEmpty)
		})

		Convey("It should publish the manifold and dynamics onto the symbol", func() {
			features := []float64{100, 99, 1, 1, 0, 0, 101, 98, 100, 10, 99.5}

			So(solver.Update("BTC/USD", at, features), ShouldBeNil)

			symbol := thesis.Symbol("BTC/USD")
			var manifold *learning.ResonanceManifold
			var hasDynamics bool

			for stored := range symbol.MarketResonance(
				symbol.ResonanceConsumers[types.ResonanceConsumerGraph],
			) {
				switch value := stored.(type) {
				case *learning.ResonanceManifold:
					manifold = value
				case types.Frame:
					hasDynamics = true
				}
			}

			So(manifold, ShouldNotBeNil)
			So(hasDynamics, ShouldBeTrue)
		})

		Convey("It should supervise the task head from the previous midpoint", func() {
			first := []float64{100, 99, 1, 1, 0, 0, 101, 98, 100, 10, 99.5}
			second := []float64{100.5, 99.5, 1, 1, 0, 0, 102, 99, 100.5, 15, 100}

			thesis.Tick = 1
			thesis.Symbol("BTC/USD").Tick = thesis.Tick
			So(solver.Update("BTC/USD", at, first), ShouldBeNil)

			stored, found := solver.references.Load("BTC/USD")

			So(found, ShouldBeTrue)
			So(stored, ShouldEqual, 99.5)

			// The first pass has no prior, so the coder receives no reference;
			// the second pass resolves the first pending prediction, and the
			// head is calibrated once a reliability scale exists.
			thesis.Tick = 2
			thesis.Symbol("BTC/USD").Tick = thesis.Tick
			So(solver.Update("BTC/USD", at.Add(time.Second), second), ShouldBeNil)

			detector, _ := solver.detectors.Load("BTC/USD")
			coder := detector.(*learning.PredictiveCoder)
			skill, skillReady := coder.Manifold().TaskSkill()

			So(skillReady, ShouldBeFalse)

			thesis.Tick = 3
			thesis.Symbol("BTC/USD").Tick = thesis.Tick
			So(solver.Update("BTC/USD", at.Add(2*time.Second), second), ShouldBeNil)

			skill, skillReady = coder.Manifold().TaskSkill()

			So(skillReady, ShouldBeTrue)
			So(skill, ShouldBeGreaterThan, 0)
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
			transport.NewMapReduce[*types.UIFrame](nil, nil, nil),
			types.NewThesis(context.Background(), nil),
		)
		Convey("It should close without error", func() {
			So(solver.Close(), ShouldBeNil)
		})
	})
}

const resonanceTestConsumer = "resonance-test"
