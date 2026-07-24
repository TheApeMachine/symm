package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNoteLifecycle(t *testing.T) {
	Convey("Given a thesis phase transition", t, func() {
		thesis := NewThesis(nil)
		at := time.Unix(1, 0).UTC()
		thesis.NoteLifecycle("BTC/USD", LifecycleEntered, at)

		Convey("It should store the phase without a parallel journal", func() {
			phase, ok := thesis.Lifecycle.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			So(phase, ShouldEqual, LifecycleEntered)
		})
	})
}

func TestThesisSaveCheckpoint(t *testing.T) {
	Convey("Given a finalized immutable cut", t, func() {
		dir := t.TempDir()
		thesis := NewThesis(nil)
		cut := &ImmutableCut{
			ID:   2,
			Tick: 4,
			At:   time.Unix(1, 0).UTC(),
		}

		Convey("Save delegates to cut checkpoint", func() {
			So(thesis.Save(dir, cut), ShouldBeNil)
			So(thesis.Save(dir, nil), ShouldNotBeNil)
		})
	})
}

func TestPublish(t *testing.T) {
	Convey("Given a thesis already carrying published source rows", t, func() {
		thesis := NewThesis(nil)
		first := time.Unix(1, 0).UTC()
		second := time.Unix(2, 0).UTC()
		thesis.Publish(SourceHawkes, []*Measurement{
			{
				Source: SourceHawkes, Metric: MetricEventCount,
				Side: SideBuy, Symbol: "SIM1/USD", Raw: 1, At: first,
			},
			{
				Source: SourceHawkes, Metric: MetricArrivalRate,
				Side: SideBuy, Symbol: "SIM1/USD", Raw: 0.5, At: first,
			},
		})
		thesis.Publish(SourcePumpDump, []*Measurement{{
			Source: SourcePumpDump, Metric: MetricRVOL,
			Side: SideNone, Symbol: "SIM1/USD", Raw: 2, At: first,
		}})

		Convey("It should upsert only matching identities", func() {
			thesis.Publish(SourceHawkes, []*Measurement{{
				Source: SourceHawkes, Metric: MetricEventCount,
				Side: SideBuy, Symbol: "SIM1/USD", Raw: 3, At: second,
			}})

			So(thesis.Measurements, ShouldHaveLength, 3)

			byMetric := map[MetricType]float64{}

			for _, row := range thesis.Measurements {
				byMetric[row.Metric] = row.Raw
			}

			So(byMetric[MetricRVOL], ShouldEqual, 2)
			So(byMetric[MetricArrivalRate], ShouldEqual, 0.5)
			So(byMetric[MetricEventCount], ShouldEqual, 3)
		})
	})
}

func BenchmarkPublish(b *testing.B) {
	thesis := NewThesis(nil)
	at := time.Unix(1, 0).UTC()
	rows := []*Measurement{{
		Source: SourceHawkes, Metric: MetricEventCount,
		Side: SideBuy, Symbol: "SIM1/USD", Raw: 1, At: at,
	}}

	b.ReportAllocs()

	for b.Loop() {
		thesis.Publish(SourceHawkes, rows)
	}
}

func BenchmarkNoteLifecycle(b *testing.B) {
	thesis := NewThesis(nil)
	at := time.Unix(1, 0).UTC()

	b.ReportAllocs()

	for b.Loop() {
		thesis.NoteLifecycle("BTC/USD", LifecycleManaging, at)
	}
}
