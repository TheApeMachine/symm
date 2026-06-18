package resonance

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/qpool"
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

		rawTicker, marshalErr := json.Marshal([]tickerFixture{{
			Symbol:    scope,
			Last:      50000,
			Volume:    1200,
			ChangePct: 0.015,
			Timestamp: observedAt,
		}})

		So(marshalErr, ShouldBeNil)

		ticker := datura.Acquire("kraken", datura.Artifact_Type_json)
		ticker.WithRole("ticker")
		ticker.WithScope(scope)
		ticker.WithPayload(rawTicker)
		_ = signal.Update(ticker)
		ticker.Release()

		rawBook, bookErr := json.Marshal([]bookFixture{{
			Symbol: scope,
			Bids:   []bookLevelFixture{{Price: 49990, Qty: 1}},
			Asks:   []bookLevelFixture{{Price: 50010, Qty: 1}},
		}})

		So(bookErr, ShouldBeNil)

		book := datura.Acquire("kraken", datura.Artifact_Type_json)
		book.WithRole("book")
		book.WithScope(scope)
		book.WithPayload(rawBook)
		_ = signal.Update(book)
		book.Release()

		probe := datura.Acquire("probe", datura.Artifact_Type_json).
			WithRole("measurement").
			WithScope(scope)

		Convey("It should withhold measurement until sensory vectors are available", func() {
			measurement, measureErr := signal.Measure(probe)

			So(measureErr, ShouldBeNil)
			So(string(measurement.Source), ShouldBeBlank)
			So(measurement.Symbol, ShouldBeBlank)
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

			updateFeed(signal, "ticker", scope, []tickerFixture{{
				Symbol:    scope,
				Last:      last,
				Volume:    1000 + float64(index),
				ChangePct: 0.01 * float64(index+1),
				Timestamp: observedAt,
			}})
			updateFeed(signal, "book", scope, []bookFixture{{
				Symbol: scope,
				Bids:   []bookLevelFixture{{Price: last - 1, Qty: 1}},
				Asks:   []bookLevelFixture{{Price: last + 1, Qty: 1}},
			}})
		}

		Convey("It should withhold universe publish until batch settlement succeeds", func() {
			received := make(chan map[string]any, 1)

			pool.Subscribe("ui", func(artifact *datura.Artifact) error {
				payload, decodeErr := qpool.ArtifactValue[map[string]any](artifact)

				if decodeErr != nil || payload["type"] != "resonance_universe" {
					return nil
				}

				received <- payload

				return nil
			})

			results, settleErr := signal.SettleScopes(scopes)

			So(settleErr, ShouldBeNil)
			So(len(results), ShouldEqual, 0)

			select {
			case <-received:
				So("ui resonance universe snapshot", ShouldEqual, "withheld")
			case <-time.After(200 * time.Millisecond):
			}
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
