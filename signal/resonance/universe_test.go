package resonance

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/qpool"
)

func TestSignalPublishUniverseSnapshot(t *testing.T) {
	Convey("Given a resonance signal with market state", t, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		signal := NewSignal(ctx, pool, dmt.NewTree(""), nil, 0.02, 64)

		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		scope := "PF_XBTUSD"
		observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		insertFeedArtifact(signal, "ticker", scope, []tickerFixture{{
			Symbol:    scope,
			Last:      50000,
			Volume:    1200,
			ChangePct: 0.015,
			Timestamp: observedAt,
		}})
		insertFeedArtifact(signal, "book", scope, []bookFixture{{
			Symbol: scope,
			Bids:   []bookLevelFixture{{Price: 49990, Qty: 1}},
			Asks:   []bookLevelFixture{{Price: 50010, Qty: 1}},
		}})

		probe := measurementQuery(scope)

		Convey("It should publish a classified measurement artifact", func() {
			result := signal.Measure(probe)

			So(result, ShouldNotBeNil)
			So(datura.Peek[int](result, "classifier", "category"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](result, "classifier", "confidence"), ShouldBeGreaterThan, 0)
			result.Release()
		})
	})
	Convey("Given a resonance signal with multiple settled symbols", t, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		signal := NewSignal(ctx, pool, dmt.NewTree(""), nil, 0.02, 8)

		defer func() {
			_ = signal.Close()
		}()

		observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		scopes := []string{"BTC/USD", "ETH/USD", "SOL/USD"}

		for index, scope := range scopes {
			last := 100.0 + float64(index)*10

			insertFeedArtifact(signal, "ticker", scope, []tickerFixture{{
				Symbol:    scope,
				Last:      last,
				Volume:    1000 + float64(index),
				ChangePct: 0.01 * float64(index+1),
				Timestamp: observedAt,
			}})
			insertFeedArtifact(signal, "book", scope, []bookFixture{{
				Symbol: scope,
				Bids:   []bookLevelFixture{{Price: last - 1, Qty: 1}},
				Asks:   []bookLevelFixture{{Price: last + 1, Qty: 1}},
			}})
		}

		Convey("It should publish a universe snapshot after batch settlement", func() {
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
			So(len(results), ShouldBeGreaterThan, 0)

			select {
			case payload := <-received:
				So(payload["type"], ShouldEqual, "resonance_universe")
				So(payload["symbol_count"], ShouldBeGreaterThan, 0)
				snapshotCount := 0

				switch snapshots := payload["snapshots"].(type) {
				case []map[string]any:
					snapshotCount = len(snapshots)
				case []any:
					snapshotCount = len(snapshots)
				default:
					So(fmt.Sprintf("%T", payload["snapshots"]), ShouldEqual, "snapshot slice")
				}

				So(snapshotCount, ShouldEqual, len(scopes))
			case <-time.After(500 * time.Millisecond):
				So("ui resonance universe snapshot", ShouldEqual, "published")
			}
		})
	})
}

func TestFocusSymbolIndex(testingTB *testing.T) {
	Convey("Given settled symbols with different surprise values", testingTB, func() {
		settled := []settledSymbolEntry{
			{
				surprise: 0.2,
				measurement: datura.Acquire(
					"test", datura.APPJSON,
				).WithRole(
					"measurement",
				).WithScope(
					"BTC/USD",
				),
			},
			{
				surprise: 0.9,
				measurement: datura.Acquire(
					"test", datura.APPJSON,
				).WithRole(
					"measurement",
				).WithScope(
					"ETH/USD",
				),
			},
			{
				surprise: 0.4,
				measurement: datura.Acquire(
					"test", datura.APPJSON,
				).WithRole(
					"measurement",
				).WithScope(
					"SOL/USD",
				),
			},
		}

		Convey("It should pick the highest surprise symbol for x-ray focus", func() {
			focusScope, _ := settled[focusSymbolIndex(settled)].measurement.Scope()

			So(focusScope, ShouldEqual, "ETH/USD")
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
			measurement: datura.Acquire(
				"test", datura.APPJSON,
			).WithRole(
				"measurement",
			).WithScope(
				fmt.Sprintf("SYM%d/USD", index),
			).WithPayload(
				[]byte(`{}`),
			).Poke(
				CategoryFlow, "category",
			).Poke(datura.Map[float64]{
				"value":      1,
				"confidence": 0.8,
				"strength":   0.5,
			}, "output"),
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
