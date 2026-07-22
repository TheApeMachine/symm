package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
stanceMeasurement builds one valid normalized measurement whose metric carries
category affinity, so composed graphs grow real Supports/Contradicts edges.
*/
func stanceMeasurement(
	symbol string,
	metric MetricType,
	at time.Time,
) *Measurement {
	normalized := 1.0

	return &Measurement{
		Source:     SourceToxicity,
		Stream:     Toxicity,
		Metric:     metric,
		Subject:    SubjectLevel3Touch,
		Symbol:     symbol,
		Side:       SideBuy,
		At:         at,
		Unit:       UnitBaseCurrency,
		Raw:        1,
		Normalized: &normalized,
		Validity:   MeasurementValidity{State: ValidityValid},
	}
}

/*
TestLongEntryEvidence proves entry evidence separates ordinary opposition from
structural blockers and requires net support before either becomes active.
*/
func TestLongEntryEvidence(t *testing.T) {
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	Convey("Given a graph with each structural long blocker", t, func() {
		graph := NewGraph("MATIC/USD")
		So(graph.Evidence.AddMeasurements([]*Measurement{
			stanceMeasurement("MATIC/USD", MetricSpoofScore, at),
			stanceMeasurement("MATIC/USD", MetricThinScore, at),
			stanceMeasurement("MATIC/USD", MetricMechanical, at),
			stanceMeasurement("MATIC/USD", MetricReversal, at),
		}), ShouldBeNil)
		graph.Compose()

		Convey("Then every established blocker vetoes the entry", func() {
			reading := graph.LongEntryEvidence()

			So(reading.Favors, ShouldBeEmpty)
			So(reading.Vetoes, ShouldContain, string(SpoofTrap))
			So(reading.Vetoes, ShouldContain, string(ToxicBluff))
			So(reading.Vetoes, ShouldContain, string(LiquidityVacuum))
			So(reading.Vetoes, ShouldContain, string(MechanicalCollapse))
			So(reading.Vetoes, ShouldContain, string(ActiveReversal))
		})
	})

	Convey("Given contested evidence that nets to zero", t, func() {
		graph := NewGraph("MATIC/USD")

		// FillVolume supports HardSupport and contradicts ToxicBluff/SpoofTrap,
		// while RetreatingQuantity supports both deception categories and
		// contradicts HardSupport: each contested category nets to zero.
		So(graph.Evidence.AddMeasurements([]*Measurement{
			stanceMeasurement("MATIC/USD", MetricRetreatingQuantity, at),
			stanceMeasurement("MATIC/USD", MetricFillVolume, at),
		}), ShouldBeNil)
		graph.Compose()

		Convey("Then contested deception and support stay inactive", func() {
			reading := graph.LongEntryEvidence()

			So(reading.Favors, ShouldNotContain, string(HardSupport))
			So(reading.Opposes, ShouldNotContain, string(ToxicBluff))
			So(reading.Vetoes, ShouldNotContain, string(ToxicBluff))
			So(reading.Vetoes, ShouldNotContain, string(SpoofTrap))
		})
	})

	Convey("Given an empty graph", t, func() {
		graph := NewGraph("MATIC/USD")

		Convey("Then no direction is invented", func() {
			reading := graph.LongEntryEvidence()

			So(reading.Favors, ShouldBeEmpty)
			So(reading.Opposes, ShouldBeEmpty)
			So(reading.Vetoes, ShouldBeEmpty)
		})
	})
}

/*
BenchmarkLongEntryEvidence measures the per-cut cost of the stance walk over a
composed graph carrying a realistic mixed evidence population.
*/
func BenchmarkLongEntryEvidence(b *testing.B) {
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	graph := NewGraph("MATIC/USD")
	metrics := []MetricType{
		MetricRetreatingQuantity, MetricCancelledQuantity, MetricFillVolume,
		MetricIgnition, MetricRVOL, MetricExhaustion, MetricThinScore,
		MetricScarcityScore, MetricDrive, MetricHerdScore,
	}
	measurements := make([]*Measurement, 0, len(metrics))

	for _, metric := range metrics {
		measurements = append(
			measurements, stanceMeasurement("MATIC/USD", metric, at),
		)
	}

	if err := graph.Evidence.AddMeasurements(measurements); err != nil {
		b.Fatal(err)
	}

	graph.Compose()
	b.ResetTimer()

	for b.Loop() {
		graph.LongEntryEvidence()
	}
}
