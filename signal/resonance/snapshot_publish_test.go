package resonance

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestSignalPublishSnapshot(t *testing.T) {
	Convey("Given a resonance signal with market state", t, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		signal := NewSignal(ctx, pool, []int{4, 8, 3}, 0.02)

		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		scope := "PF_XBTUSD"
		observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		signal.ticker.Update(krakenmarket.TickerUpdates{{
			Symbol:    scope,
			Last:      50000,
			Volume:    1200,
			ChangePct: 0.015,
			Timestamp: observedAt,
		}})

		signal.book.Update(krakenmarket.BookUpdates{{
			Symbol: scope,
			Bids:   []krakenmarket.BookLevel{{Price: 49990, Qty: 1}},
			Asks:   []krakenmarket.BookLevel{{Price: 50010, Qty: 1}},
		}})

		probe := datura.Acquire("probe", datura.Artifact_Type_json).
			WithRole("measurement").
			WithScope(scope)

		Convey("It should publish a resonance x-ray snapshot on the ui channel", func() {
			received := make(chan map[string]any, 1)

			pool.Subscribe("ui", func(artifact *datura.Artifact) error {
				payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

				if decodeErr != nil || payload["type"] != "resonance" {
					return nil
				}

				received <- payload

				return nil
			})

			measurement, measureErr := signal.Measure(probe)

			So(measureErr, ShouldBeNil)
			So(string(measurement.Source), ShouldEqual, "resonance")
			So(measurement.Symbol, ShouldEqual, scope)

			var frame map[string]any

			select {
			case frame = <-received:
			case <-time.After(2 * time.Second):
				So("ui resonance snapshot", ShouldEqual, "received")
			}

			So(frame["type"], ShouldEqual, "resonance")
			So(frame["symbol"], ShouldEqual, scope)

			layers, ok := frame["layers"].([]any)

			So(ok, ShouldBeTrue)
			So(len(layers), ShouldEqual, 3)
		})
	})
}
