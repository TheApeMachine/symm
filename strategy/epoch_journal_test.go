package strategy

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestEpochJournalRecord(t *testing.T) {
	Convey("Given an empty typed evidence journal", t, func() {
		journal := NewEpochJournal()
		first := epochJournalValue(time.Unix(1, 0), 1)
		second := epochJournalValue(time.Unix(2, 0), 2)

		Convey("When successive logic epochs are recorded", func() {
			err := journal.Record(first, second)
			first.Measurements[0].Raw = 99

			Convey("Then the causal order and original values are retained", func() {
				So(err, ShouldBeNil)
				epochs := journal.Epochs("BTC/USD")
				So(epochs, ShouldHaveLength, 2)
				So(epochs[0].At, ShouldEqual, time.Unix(1, 0))
				So(epochs[0].Measurements[0].Raw, ShouldEqual, 1.0)
				So(epochs[1].At, ShouldEqual, time.Unix(2, 0))
			})
		})
	})
}

func TestEpochJournalEpochs(t *testing.T) {
	Convey("Given a retained logic epoch", t, func() {
		journal := NewEpochJournal()
		So(journal.Record(epochJournalValue(time.Unix(1, 0), 1)), ShouldBeNil)

		Convey("When a reader mutates the returned measurement", func() {
			epochs := journal.Epochs("BTC/USD")
			epochs[0].Measurements[0].Raw = 99

			Convey("Then the append-only record is unchanged", func() {
				loaded := journal.Epochs("BTC/USD")
				So(loaded[0].Measurements[0].Raw, ShouldEqual, 1.0)
			})
		})
	})
}

func TestEpochJournalSymbols(t *testing.T) {
	Convey("Given typed epochs for two symbols", t, func() {
		journal := NewEpochJournal()
		first := epochJournalValue(time.Unix(1, 0), 1)
		first.Symbol = "Z/USD"
		first.Measurements[0].Symbol = "Z/USD"
		second := epochJournalValue(time.Unix(1, 0), 1)
		second.Symbol = "A/USD"
		second.Measurements[0].Symbol = "A/USD"
		So(journal.Record(first, second), ShouldBeNil)

		Convey("When the active symbols are requested", func() {
			Convey("Then typed-only symbols remain visible in deterministic order", func() {
				So(journal.Symbols(), ShouldResemble, []string{"A/USD", "Z/USD"})
			})
		})
	})
}

func TestEpochJournalRecordAvailabilityOrder(t *testing.T) {
	Convey("Given a journal whose latest available evidence has a newer event time", t, func() {
		journal := NewEpochJournal()
		So(journal.Record(epochJournalValue(time.Unix(2, 0), 2)), ShouldBeNil)
		older := epochJournalValue(time.Unix(1, 0), 1)

		Convey("When an older event-time epoch arrives", func() {
			err := journal.Record(older)

			Convey("Then causal availability is retained for later staleness analysis", func() {
				So(err, ShouldBeNil)
				epochs := journal.Epochs("BTC/USD")
				So(epochs, ShouldHaveLength, 2)
				So(epochs[0].At, ShouldEqual, time.Unix(2, 0))
				So(epochs[1].At, ShouldEqual, time.Unix(1, 0))
			})
		})
	})
}

func BenchmarkEpochJournalRecord(b *testing.B) {
	const symbols = 1455
	epochs := make([]types.LogicEpoch, symbols)

	for index := range symbols {
		epochs[index] = epochJournalValue(time.Unix(1, 0), float64(index))
		epochs[index].Symbol = fmt.Sprintf("ASSET-%04d/USD", index)
		epochs[index].Measurements[0].Symbol = epochs[index].Symbol
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		journal := NewEpochJournal()

		if err := journal.Record(epochs...); err != nil {
			b.Fatal(err)
		}
	}
}

func epochJournalValue(at time.Time, raw float64) types.LogicEpoch {
	measurement := composerEpochMeasurement(at, raw)

	return types.LogicEpoch{
		Symbol:       measurement.Symbol,
		At:           at,
		Measurements: []types.Measurement{measurement},
	}
}

func composerEpochMeasurement(at time.Time, raw float64) types.Measurement {
	from := at.Add(-time.Second)

	return types.Measurement{
		Source:       types.SourceHawkes,
		Metric:       types.MetricConditionalIntensity,
		Subject:      types.SubjectHawkesProcess,
		Stream:       "trades",
		Symbol:       "BTC/USD",
		Side:         types.SideBuy,
		At:           at,
		ObservedFrom: from,
		Horizon:      at.Sub(from),
		Unit:         types.UnitEventsPerSecond,
		Raw:          raw,
		Maturity:     0.5,
		Validity: types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessIntensity,
		},
		Scale: types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    from,
			Through: at,
		},
	}
}
