package resonance

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func TestSignalPublishUniverseSnapshot(t *testing.T) {
	Convey("Given a resonance signal with market state", t, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		signal := NewSignal(ctx, pool, nil, 0.02, 64)

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

		Convey("It should publish one resonance_universe snapshot on the ui channel", func() {
			received := make(chan map[string]any, 1)

			pool.Subscribe("ui", func(artifact *datura.Artifact) error {
				payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

				if decodeErr != nil || payload["type"] != "resonance_universe" {
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
				So("ui resonance universe snapshot", ShouldEqual, "received")
			}

			So(frame["type"], ShouldEqual, "resonance_universe")
			So(frame["focus_symbol"], ShouldEqual, scope)
			So(frame["symbol_count"], ShouldEqual, 1)

			focus, ok := frame["focus"].(map[string]any)

			So(ok, ShouldBeTrue)
			So(focus["symbol"], ShouldEqual, scope)

			layers, ok := focus["layers"].([]any)

			So(ok, ShouldBeTrue)
			So(len(layers), ShouldEqual, 3)
		})
	})

	Convey("Given a resonance signal with multiple settled symbols", t, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		signal := NewSignal(ctx, pool, nil, 0.02, 8)

		defer func() {
			_ = signal.Close()
		}()

		observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		scopes := []string{"BTC/USD", "ETH/USD", "SOL/USD"}

		for index, scope := range scopes {
			last := 100.0 + float64(index)*10

			signal.ticker.Update(krakenmarket.TickerUpdates{{
				Symbol:    scope,
				Last:      last,
				Volume:    1000 + float64(index),
				ChangePct: 0.01 * float64(index+1),
				Timestamp: observedAt,
			}})

			signal.book.Update(krakenmarket.BookUpdates{{
				Symbol: scope,
				Bids:   []krakenmarket.BookLevel{{Price: last - 1, Qty: 1}},
				Asks:   []krakenmarket.BookLevel{{Price: last + 1, Qty: 1}},
			}})
		}

		Convey("It should publish one resonance_universe frame for the batch", func() {
			received := make(chan map[string]any, 1)

			pool.Subscribe("ui", func(artifact *datura.Artifact) error {
				payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

				if decodeErr != nil || payload["type"] != "resonance_universe" {
					return nil
				}

				received <- payload

				return nil
			})

			_, settleErr := signal.SettleScopes(scopes)

			So(settleErr, ShouldBeNil)

			var frame map[string]any

			select {
			case frame = <-received:
			case <-time.After(2 * time.Second):
				So("ui resonance universe snapshot", ShouldEqual, "received")
			}

			So(frame["symbol_count"], ShouldEqual, len(scopes))

			symbols, ok := frame["symbols"].([]any)

			So(ok, ShouldBeTrue)
			So(len(symbols), ShouldEqual, len(scopes))

			focusSymbol, ok := frame["focus_symbol"].(string)

			So(ok, ShouldBeTrue)
			So(focusSymbol, ShouldNotBeBlank)
		})
	})
}

func TestFocusSymbolIndex(testingTB *testing.T) {
	Convey("Given settled symbols with different surprise values", testingTB, func() {
		settled := []settledSymbolEntry{
			{
				surprise: 0.2,
				measurement: logic.Measurement{
					Symbol:     "BTC/USD",
					ObservedAt: time.Now(),
				},
			},
			{
				surprise: 0.9,
				measurement: logic.Measurement{
					Symbol:     "ETH/USD",
					ObservedAt: time.Now(),
				},
			},
			{
				surprise: 0.4,
				measurement: logic.Measurement{
					Symbol:     "SOL/USD",
					ObservedAt: time.Now(),
				},
			},
		}

		Convey("It should pick the highest surprise symbol for x-ray focus", func() {
			So(settled[focusSymbolIndex(settled)].measurement.Symbol, ShouldEqual, "ETH/USD")
		})
	})
}

func BenchmarkUniverseSnapshotPayload(b *testing.B) {
	arch := DefaultArchitecture()
	settled := make([]settledSymbolEntry, 0, 128)

	for index := range 128 {
		settled = append(settled, settledSymbolEntry{
			outcome: settleOutcome{
				symbol:   fmt.Sprintf("SYM%d/USD", index),
				latent:   []float64{0.1, 0.2, 0.3},
				surprise: float64(index) * 0.01,
				energy:   float64(index) * 0.02,
			},
			measurement: logic.Measurement{
				Symbol:     fmt.Sprintf("SYM%d/USD", index),
				Confidence: 0.5,
				Category:   CategoryFlow,
				Strength:   0.4,
				ObservedAt: time.Now(),
			},
			layers: []learning.ResonanceLayerWire{
				{State: make([]float64, arch[0]), Prediction: make([]float64, arch[0]), ErrorNorm: 0.01},
				{State: make([]float64, arch[1]), Prediction: make([]float64, arch[1]), ErrorNorm: 0.01},
				{State: []float64{0.1, 0.2, 0.3}, Prediction: []float64{0.1, 0.2, 0.3}, ErrorNorm: 0.01},
			},
			surprise: float64(index) * 0.01,
			energy:   float64(index) * 0.02,
		})
	}

	b.ResetTimer()

	for b.Loop() {
		_, err := universeSnapshotPayload(arch, settled)

		if err != nil {
			b.Fatal(err)
		}
	}
}
