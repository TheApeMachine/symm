package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNoteLifecycle(t *testing.T) {
	Convey("Given a thesis phase transition", t, func() {
		thesis := NewThesis()
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
		thesis := NewThesis()
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
		thesis := NewThesis()
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

		Convey("Concurrent source publishes keep both surfaces", func() {
			wait := make(chan struct{})
			done := make(chan struct{}, 2)

			go func() {
				<-wait
				for index := range 64 {
					thesis.Publish(SourceToxicity, []*Measurement{{
						Source: SourceToxicity, Metric: MetricStrength,
						Symbol: "SIM1/USD", Raw: float64(index), At: second,
					}})
				}
				done <- struct{}{}
			}()

			go func() {
				<-wait
				for index := range 64 {
					thesis.Publish(SourceExhaustion, []*Measurement{{
						Source: SourceExhaustion, Metric: MetricStrength,
						Symbol: "SIM1/USD", Raw: float64(index) + 100, At: second,
					}})
				}
				done <- struct{}{}
			}()

			close(wait)
			<-done
			<-done

			bySource := map[SourceType]float64{}

			for _, row := range thesis.SnapshotMeasurements() {
				if row.Metric == MetricStrength {
					bySource[row.Source] = row.Raw
				}
			}

			So(bySource[SourceToxicity], ShouldBeGreaterThanOrEqualTo, 0)
			So(bySource[SourceExhaustion], ShouldBeGreaterThanOrEqualTo, 100)
		})
	})
}

/*
TestPublishResonanceStaysBounded proves repeated resonance publishes replace
identities in place instead of appending a new pair every Hawkes epoch.
*/
func TestPublishResonanceStaysBounded(t *testing.T) {
	Convey("Given resonance energy and surprise for one symbol", t, func() {
		thesis := NewThesis()
		at := time.Unix(1, 0).UTC()

		for epoch := range 10_000 {
			thesis.Publish(SourceResonance, []*Measurement{
				{
					Metric: MetricResonanceEnergy, Symbol: "SIM1/USD",
					Raw: float64(epoch), At: at.Add(time.Duration(epoch)),
				},
				{
					Metric: MetricResonanceSurprise, Symbol: "SIM1/USD",
					Raw: float64(epoch) + 0.5, At: at.Add(time.Duration(epoch)),
				},
			})
		}

		Convey("Then the published surface stays two rows for that symbol", func() {
			So(thesis.Measurements, ShouldHaveLength, 2)
			So(thesis.Measurements[0].Source, ShouldEqual, SourceResonance)
			So(thesis.Measurements[0].Raw, ShouldEqual, 9999)
			So(thesis.Measurements[1].Raw, ShouldEqual, 9999.5)
		})
	})
}

/*
TestPublishRetractsAbsentSymbolMetrics proves a later publish for the same
source and symbol drops identities that are no longer in the incoming batch.
*/
func TestPublishRetractsAbsentSymbolMetrics(t *testing.T) {
	Convey("Given a trade volume row already on the thesis", t, func() {
		thesis := NewThesis()
		at := time.Unix(1, 0).UTC()
		thesis.Publish(SourceToxicity, []*Measurement{
			{
				Metric: MetricTradeVolume, Symbol: "SIM1/USD",
				Raw: 4, At: at,
			},
			{
				Metric: MetricTouchQuantity, Side: SideBuy, Symbol: "SIM1/USD",
				Raw: 2, At: at,
			},
			{
				Metric: MetricTouchQuantity, Side: SideBuy, Symbol: "SIM2/USD",
				Raw: 3, At: at,
			},
		})

		Convey("When a book-only publish arrives for the traded symbol", func() {
			thesis.Publish(SourceToxicity, []*Measurement{{
				Metric: MetricTouchQuantity, Side: SideBuy, Symbol: "SIM1/USD",
				Raw: 1, At: at.Add(time.Second),
			}})

			Convey("Then trade volume is retracted while other symbols remain", func() {
				So(thesis.Measurements, ShouldHaveLength, 2)

				byKey := map[string]float64{}

				for _, row := range thesis.Measurements {
					byKey[string(row.Metric)+"|"+row.Symbol] = row.Raw
				}

				So(byKey[string(MetricTradeVolume)+"|SIM1/USD"], ShouldEqual, 0)
				So(byKey[string(MetricTouchQuantity)+"|SIM1/USD"], ShouldEqual, 1)
				So(byKey[string(MetricTouchQuantity)+"|SIM2/USD"], ShouldEqual, 3)
			})
		})
	})
}

func BenchmarkPublish(b *testing.B) {
	thesis := NewThesis()
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
	thesis := NewThesis()
	at := time.Unix(1, 0).UTC()

	b.ReportAllocs()

	for b.Loop() {
		thesis.NoteLifecycle("BTC/USD", LifecycleManaging, at)
	}
}
