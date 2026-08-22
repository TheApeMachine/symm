package toxicity

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func toxicityLevel3(at time.Time, fills float64, deletes float64) kraken.Level3Data {
	bids := []kraken.Level3Order{}

	if fills > 0 {
		bids = append(bids, kraken.Level3Order{
			OrderID:    "fill1",
			Event:      "fill",
			LimitPrice: decimal.NewFromFloat64(100.0),
			OrderQty:   decimal.NewFromFloat64(fills),
		})
	}

	if deletes > 0 {
		bids = append(bids, kraken.Level3Order{
			OrderID:    "del1",
			Event:      "delete",
			LimitPrice: decimal.NewFromFloat64(100.0),
			OrderQty:   decimal.NewFromFloat64(deletes),
		})
	}

	return kraken.Level3Data{
		Symbol:    "BTC/USD",
		Type:      "update",
		Timestamp: at,
		Bids:      bids,
	}
}

func TestToxicityNumber(t *testing.T) {
	Convey("Given level3 order frames on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("BTC/USD")
		base := time.Unix(1_700_004_000, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		market.AppendLevel3(toxicityLevel3(base, 10.0, 2.0))

		Convey("It should emit an honesty reading per frame", func() {
			measurements := drainToxicityMeasurements(market, 1)

			So(len(measurements), ShouldEqual, 1)
			So(measurements[0].Source, ShouldEqual, string(types.SourceToxicity))

			_, hasZScore := measurements[0].Metrics["honesty_zscore"]
			So(hasZScore, ShouldBeTrue)

			_, hasIntensity := measurements[0].Metrics["toxicity_intensity"]
			So(hasIntensity, ShouldBeTrue)
		})
	})

	Convey("Adversarial: Heavy cancel/pull asymmetry flags toxicity", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("BTC/USD")
		base := time.Unix(1_700_004_000, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		market.AppendLevel3(toxicityLevel3(base, 0, 100.0))
		measurements := drainToxicityMeasurements(market, 1)

		So(len(measurements), ShouldEqual, 1)
		intensity, found := measurements[0].Metrics["toxicity_intensity"]
		So(found, ShouldBeTrue)
		So(intensity.Raw, ShouldBeGreaterThan, 0)
	})
}

func drainToxicityMeasurements(symbol *types.Symbol, expected int) []*nmtypes.Measurement {
	readings := []*nmtypes.Measurement{}
	deadline := time.Now().Add(2 * time.Second)

	for len(readings) < expected && time.Now().Before(deadline) {
		for measurement := range symbol.MarketMeasurements(
			symbol.MeasurementConsumers[types.MeasurementConsumerCategory],
		) {
			readings = append(readings, measurement)
		}

		if len(readings) >= expected {
			break
		}

		time.Sleep(time.Millisecond)
	}

	return readings
}

func BenchmarkToxicityPipeline(b *testing.B) {
	thesis := types.NewThesis(context.Background(), nil)
	market := thesis.Symbol("BTC/USD")
	signal := NewSignal(context.Background(), thesis)
	defer signal.Close()

	base := time.Unix(1_700_004_000, 0).UTC()
	frame := toxicityLevel3(base, 10.0, 2.0)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		market.AppendLevel3(frame)
	}
}
