package resonance

import (
	"context"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/learning"
	"github.com/theapemachine/symm/logic"
)

func TestSignalPublishUniverseSnapshot(t *testing.T) {
	Convey("Given a resonance signal with market state", t, func() {
		signal := NewSignal(context.Background(), nil, 0.02, 64)
		defer func() { _ = signal.Close() }()

		scope := "PF_XBTUSD"
		seedMarket(t, signal, scope, 50000, 1200, 0.015, 0.0004, startAt(0))

		Convey("When the scope is settled", func() {
			result := settledMeasurement(t, signal, scope)

			Convey("Then it should publish a classified measurement", func() {
				So(result, ShouldNotBeNil)
				So(result.DominantCategory(), ShouldNotEqual, logic.CategoryTypeNone)
				So(result.Confidence, ShouldBeGreaterThan, 0)
			})
		})
	})

	Convey("Given a resonance signal with multiple settled symbols", t, func() {
		signal := NewSignal(context.Background(), nil, 0.02, 8)
		defer func() { _ = signal.Close() }()

		scopes := []string{"BTC/USD", "ETH/USD", "SOL/USD"}
		seedUniverse(t, signal, scopes, startAt(0))

		Convey("When a universe snapshot is built", func() {
			results, settleErr := signal.SettleScopes(scopes)
			payload, payloadErr := universeSnapshotPayload(signal.arch, signal.lastSettled)

			Convey("Then it should include every settled symbol", func() {
				So(settleErr, ShouldBeNil)
				So(results, ShouldHaveLength, len(scopes))
				So(payloadErr, ShouldBeNil)
				So(payload["type"], ShouldEqual, "resonance_universe")
				So(payload["symbol_count"], ShouldEqual, len(scopes))
				So(payload["snapshots"], ShouldHaveLength, len(scopes))
			})
		})
	})
}

func TestFocusSymbolIndex(t *testing.T) {
	Convey("Given settled symbols with different surprise values", t, func() {
		settled := []settledSymbolEntry{
			settledEntry("BTC/USD", 0.2),
			settledEntry("ETH/USD", 0.9),
			settledEntry("SOL/USD", 0.4),
		}

		Convey("When the focus index is selected", func() {
			focus := settled[focusSymbolIndex(settled)]

			Convey("Then it should pick the highest surprise symbol", func() {
				So(focus.measurement.Symbol, ShouldEqual, "ETH/USD")
			})
		})
	})
}

func BenchmarkUniverseSnapshotPayload(b *testing.B) {
	arch := DefaultArchitecture()
	settled := make([]settledSymbolEntry, 0, 128)

	for index := range 128 {
		entry := settledEntry(fmt.Sprintf("SYM%d/USD", index), float64(index)*0.01)
		entry.outcome = settleOutcome{
			symbol:   entry.measurement.Symbol,
			latent:   []float64{0.1, 0.2, 0.3},
			surprise: float64(index) * 0.01,
			energy:   float64(index) * 0.02,
		}
		entry.layers = wireLayers(arch)
		entry.energy = float64(index) * 0.02
		settled = append(settled, entry)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := universeSnapshotPayload(arch, settled); err != nil {
			b.Fatal(err)
		}
	}
}

func settledEntry(symbol string, surprise float64) settledSymbolEntry {
	measurement := logic.NewMeasurement(logic.SourceResonance, symbol, startAt(0))
	_ = measurement.ApplyClassifier(
		1,
		0.8,
		1.0/float64(resonanceLatentWidth),
		1.0/float64(resonanceLatentWidth),
		0.5,
		map[string]float64{CategoryFlow: 1},
	)

	arch := DefaultArchitecture()

	return settledSymbolEntry{
		outcome: settleOutcome{
			symbol:   symbol,
			latent:   []float64{0.1, 0.2, 0.3},
			surprise: surprise,
			energy:   surprise * 2,
		},
		measurement: measurement,
		layers:      wireLayers(arch),
		surprise:    surprise,
		energy:      surprise * 2,
	}
}

func wireLayers(arch []int) []learning.ResonanceLayerWire {
	layers := make([]learning.ResonanceLayerWire, 0, len(arch))

	for _, width := range arch {
		layers = append(layers, learning.ResonanceLayerWire{
			State:      make([]float64, width),
			Prediction: make([]float64, width),
			ErrorNorm:  0.01,
		})
	}

	return layers
}
