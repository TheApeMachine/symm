package types

import (
	"fmt"
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
		thesis.Publish(SourceHawkes, []*Measurement{{
			Source: SourceHawkes, Symbol: "SIM1/USD", At: first,
			Metrics: map[string]MetricSample{
				MetricKey(MetricEventCount, SideBuy):   {Raw: 1},
				MetricKey(MetricArrivalRate, SideBuy):  {Raw: 0.5},
			},
		}})
		thesis.Publish(SourcePumpDump, []*Measurement{{
			Source: SourcePumpDump, Symbol: "SIM1/USD", At: first,
			Metrics: map[string]MetricSample{
				MetricKey(MetricRVOL, SideNone): {Raw: 2},
			},
		}})

		Convey("It should upsert only matching identities", func() {
			thesis.Publish(SourceHawkes, []*Measurement{{
				Source: SourceHawkes, Symbol: "SIM1/USD", At: second,
				Metrics: map[string]MetricSample{
					MetricKey(MetricEventCount, SideBuy):  {Raw: 3},
					MetricKey(MetricArrivalRate, SideBuy): {Raw: 0.5},
				},
			}})

			So(thesis.Measurements, ShouldHaveLength, 2)

			byMetric := map[MetricType]float64{}

			for _, row := range thesis.Measurements {
				row.EachMetric(func(metric MetricType, side MeasurementSide, sample MetricSample) {
					byMetric[metric] = sample.Raw
				})
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
						Source: SourceToxicity, Symbol: "SIM1/USD", At: second,
						Metrics: map[string]MetricSample{
							MetricKey(MetricStrength, SideNone): {Raw: float64(index)},
						},
					}})
				}
				done <- struct{}{}
			}()

			go func() {
				<-wait
				for index := range 64 {
					thesis.Publish(SourceExhaustion, []*Measurement{{
						Source: SourceExhaustion, Symbol: "SIM1/USD", At: second,
						Metrics: map[string]MetricSample{
							MetricKey(MetricStrength, SideNone): {Raw: float64(index) + 100},
						},
					}})
				}
				done <- struct{}{}
			}()

			close(wait)
			<-done
			<-done

			bySource := map[SourceType]float64{}

			for _, row := range thesis.SnapshotMeasurements() {
				sample, ok := row.Sample(MetricStrength, SideNone)

				if ok {
					bySource[row.Source] = sample.Raw
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
			thesis.Publish(SourceResonance, []*Measurement{{
				Source: SourceResonance, Symbol: "SIM1/USD",
				At: at.Add(time.Duration(epoch)),
				Metrics: map[string]MetricSample{
					MetricKey(MetricResonanceEnergy, SideNone):    {Raw: float64(epoch)},
					MetricKey(MetricResonanceSurprise, SideNone): {Raw: float64(epoch) + 0.5},
				},
			}})
		}

		Convey("Then the published surface stays one row for that symbol", func() {
			So(thesis.Measurements, ShouldHaveLength, 1)
			So(thesis.Measurements[0].Source, ShouldEqual, SourceResonance)

			energy, ok := thesis.Measurements[0].Sample(MetricResonanceEnergy, SideNone)
			So(ok, ShouldBeTrue)
			So(energy.Raw, ShouldEqual, 9999)

			surprise, ok := thesis.Measurements[0].Sample(MetricResonanceSurprise, SideNone)
			So(ok, ShouldBeTrue)
			So(surprise.Raw, ShouldEqual, 9999.5)
		})
	})
}

/*
TestPublishSourceSymbolBound proves the Thesis bag is one row per source×symbol
even when each publish carries many Metrics samples.
*/
func TestPublishSourceSymbolBound(t *testing.T) {
	Convey("Given many sources publishing multi-metric rows for many symbols", t, func() {
		thesis := NewThesis()
		at := time.Unix(1, 0).UTC()
		sources := []SourceType{
			SourceHawkes, SourcePumpDump, SourceCVD, SourceDepthFlow,
			SourceExhaustion, SourceToxicity, SourceLiquidity, SourceLeadLag,
			SourceCorrelation, SourceSentiment,
		}
		symbols := make([]string, 64)

		for index := range symbols {
			symbols[index] = fmt.Sprintf("SIM%d/USD", index)
		}

		for _, source := range sources {
			rows := make([]*Measurement, 0, len(symbols))

			for _, symbol := range symbols {
				row := &Measurement{Source: source, Symbol: symbol, At: at}

				for metricIndex := range 8 {
					row.PutMetric(
						MetricType("metric_"+string(rune('a'+metricIndex))),
						SideNone,
						MetricSample{Raw: float64(metricIndex + 1)},
					)
				}

				rows = append(rows, row)
			}

			thesis.Publish(source, rows)
		}

		Convey("Then the bag size equals sources times symbols, not metrics", func() {
			So(thesis.Measurements, ShouldHaveLength, len(sources)*len(symbols))
			So(thesis.Measurements, ShouldHaveLength, 640)

			totalSamples := 0

			for _, row := range thesis.Measurements {
				totalSamples += len(row.Metrics)
			}

			So(totalSamples, ShouldEqual, 640*8)
		})
	})
}

/*
TestPublishRetractsAbsentSymbolMetrics proves Replace drops prior same-source
identities for symbols in the incoming batch while leaving other symbols alone.
*/
func TestPublishRetractsAbsentSymbolMetrics(t *testing.T) {
	Convey("Given a trade volume row already on the thesis", t, func() {
		thesis := NewThesis()
		at := time.Unix(1, 0).UTC()
		thesis.Publish(SourceToxicity, []*Measurement{
			{
				Source: SourceToxicity, Symbol: "SIM1/USD", At: at,
				Metrics: map[string]MetricSample{
					MetricKey(MetricTradeVolume, SideNone):      {Raw: 4},
					MetricKey(MetricTouchQuantity, SideBuy):     {Raw: 2},
				},
			},
			{
				Source: SourceToxicity, Symbol: "SIM2/USD", At: at,
				Metrics: map[string]MetricSample{
					MetricKey(MetricTouchQuantity, SideBuy): {Raw: 3},
				},
			},
		})

		Convey("When a book-only replace arrives for the traded symbol", func() {
			thesis.Replace(SourceToxicity, []*Measurement{{
				Source: SourceToxicity, Symbol: "SIM1/USD", At: at.Add(time.Second),
				Metrics: map[string]MetricSample{
					MetricKey(MetricTouchQuantity, SideBuy): {Raw: 1},
				},
			}})

			Convey("Then trade volume is retracted while other symbols remain", func() {
				So(thesis.Measurements, ShouldHaveLength, 2)

				byKey := map[string]float64{}

				for _, row := range thesis.Measurements {
					row.EachMetric(func(metric MetricType, side MeasurementSide, sample MetricSample) {
						byKey[string(metric)+"|"+row.Symbol] = sample.Raw
					})
				}

				_, hasTrade := byKey[string(MetricTradeVolume)+"|SIM1/USD"]
				So(hasTrade, ShouldBeFalse)
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
		Source: SourceHawkes, Symbol: "SIM1/USD", At: at,
		Metrics: map[string]MetricSample{
			MetricKey(MetricEventCount, SideBuy): {Raw: 1},
		},
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
