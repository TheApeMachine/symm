package category

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestEvidenceIndexUpdateFrom(t *testing.T) {
	Convey("Given a fresh evidence index", t, func() {
		index := newEvidenceIndex()

		Convey("When updated from a thesis with valid measurements", func() {
			thesis := types.NewThesis()
			mass := 0.75
			thesis.Measurements.Store(types.SourcePumpDump, []*types.Measurement{
				{
					Source:       types.SourcePumpDump,
					Symbol:       "SIM/USD",
					At:           time.Unix(10, 0),
					ObservedFrom: time.Unix(9, 0),
					Horizon:      2 * time.Second,
					Validity: types.MeasurementValidity{
						State:     types.ValidityValid,
						Readiness: types.ReadinessObservation,
					},
					Metrics: map[string]types.MetricSample{
						types.MetricKey(types.MetricIgnition, types.SideNone): {
							Raw: mass, Normalized: &mass,
						},
					},
				},
			})

			index.UpdateFrom(thesis)

			Convey("It should index the metric mass for the symbol", func() {
				So(index.metricMass("SIM/USD", types.MetricIgnition), ShouldAlmostEqual, 0.75)
			})

			Convey("It should store temporal envelope", func() {
				clock := index.clockFor("SIM/USD", []string{string(types.MetricIgnition)})
				So(clock.ok, ShouldBeTrue)
				So(clock.mass, ShouldBeGreaterThan, 0)
				So(clock.horizon, ShouldEqual, 2*time.Second)
			})
		})

		Convey("When updated from a nil thesis", func() {
			Convey("It should not panic", func() {
				So(func() { index.UpdateFrom(nil) }, ShouldNotPanic)
			})
		})
	})
}

func TestMetricMass(t *testing.T) {
	Convey("Given a nil evidence index", t, func() {
		var index *evidenceIndex

		Convey("It should return zero without panicking", func() {
			So(index.metricMass("SIM/USD", types.MetricIgnition), ShouldEqual, 0)
		})
	})

	Convey("Given an index with no data for the queried symbol", t, func() {
		index := newEvidenceIndex()

		Convey("It should return zero", func() {
			So(index.metricMass("UNKNOWN/USD", types.MetricIgnition), ShouldEqual, 0)
		})
	})
}

func TestClockFor(t *testing.T) {
	Convey("Given a nil evidence index", t, func() {
		var index *evidenceIndex

		Convey("It should return an empty clock without panicking", func() {
			clock := index.clockFor("SIM/USD", []string{"ignition"})
			So(clock.ok, ShouldBeFalse)
		})
	})

	Convey("Given an index with multiple metrics", t, func() {
		index := newEvidenceIndex()
		thesis := types.NewThesis()
		mass1 := 0.5
		mass2 := 0.8

		thesis.Measurements.Store(types.SourcePumpDump, []*types.Measurement{
			{
				Source: types.SourcePumpDump, Symbol: "SIM/USD",
				At:           time.Unix(10, 0),
				ObservedFrom: time.Unix(8, 0),
				Horizon:      3 * time.Second,
				Validity: types.MeasurementValidity{
					State: types.ValidityValid, Readiness: types.ReadinessObservation,
				},
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricIgnition, types.SideNone): {
						Raw: mass1, Normalized: &mass1,
					},
				},
			},
			{
				Source: types.SourceLiquidity, Symbol: "SIM/USD",
				At:           time.Unix(15, 0),
				ObservedFrom: time.Unix(12, 0),
				Horizon:      5 * time.Second,
				Validity: types.MeasurementValidity{
					State: types.ValidityValid, Readiness: types.ReadinessObservation,
				},
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricTrend, types.SideNone): {
						Raw: mass2, Normalized: &mass2,
					},
				},
			},
		})

		index.UpdateFrom(thesis)

		Convey("When querying a clock spanning both metrics", func() {
			clock := index.clockFor("SIM/USD", []string{
				string(types.MetricIgnition),
				string(types.MetricTrend),
			})

			Convey("It should merge temporal envelopes and take max horizon", func() {
				So(clock.ok, ShouldBeTrue)
				So(clock.horizon, ShouldEqual, 5*time.Second)
				So(clock.mass, ShouldAlmostEqual, mass1+mass2)
			})
		})
	})
}

func TestSampleMass(t *testing.T) {
	Convey("Given a sample with normalized value", t, func() {
		normalized := 0.6
		sample := types.MetricSample{Raw: 0.3, Normalized: &normalized}
		mass, ok := sampleMass(sample)

		Convey("It should prefer the normalized value", func() {
			So(ok, ShouldBeTrue)
			So(mass, ShouldAlmostEqual, 0.6)
		})
	})

	Convey("Given a sample with only raw value", t, func() {
		sample := types.MetricSample{Raw: -0.4}
		mass, ok := sampleMass(sample)

		Convey("It should use abs(raw)", func() {
			So(ok, ShouldBeTrue)
			So(mass, ShouldAlmostEqual, 0.4)
		})
	})

	Convey("Given a sample with zero raw and no normalized", t, func() {
		sample := types.MetricSample{Raw: 0}
		_, ok := sampleMass(sample)

		Convey("It should report not ok", func() {
			So(ok, ShouldBeFalse)
		})
	})
}

func TestSymbolEvidenceClear(t *testing.T) {
	Convey("Given a symbolEvidence with populated maps", t, func() {
		evidence := &symbolEvidence{
			mass:    map[types.MetricType]float64{types.MetricIgnition: 0.5},
			from:    map[types.MetricType]time.Time{types.MetricIgnition: time.Unix(1, 0)},
			through: map[types.MetricType]time.Time{types.MetricIgnition: time.Unix(2, 0)},
			horizon: map[types.MetricType]time.Duration{types.MetricIgnition: time.Second},
		}

		Convey("When clear is called", func() {
			evidence.clear()

			Convey("It should empty all maps but keep them allocated", func() {
				So(len(evidence.mass), ShouldEqual, 0)
				So(len(evidence.from), ShouldEqual, 0)
				So(len(evidence.through), ShouldEqual, 0)
				So(len(evidence.horizon), ShouldEqual, 0)
				So(evidence.mass, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkEvidenceIndexUpdateFrom(b *testing.B) {
	thesis := types.NewThesis()
	symbols := []string{"AAA/USD", "BBB/USD", "CCC/USD", "DDD/USD"}
	metrics := []types.MetricType{
		types.MetricIgnition, types.MetricTrend,
		types.MetricDecoupled, types.MetricNoiseScore,
	}
	base := time.Unix(100, 0).UTC()

	for _, symbol := range symbols {
		for index, metric := range metrics {
			mass := 0.2 + float64(index)/10
			thesis.Measurements.Store(types.SourcePumpDump, []*types.Measurement{
				{
					Source: types.SourcePumpDump, Symbol: symbol,
					At:           base.Add(time.Duration(index) * time.Second),
					ObservedFrom: base.Add(time.Duration(index-1) * time.Second),
					Horizon:      time.Second,
					Validity: types.MeasurementValidity{
						State: types.ValidityValid, Readiness: types.ReadinessObservation,
					},
					Metrics: map[string]types.MetricSample{
						types.MetricKey(metric, types.SideNone): {
							Raw: mass, Normalized: &mass,
						},
					},
				},
			})
		}
	}

	index := newEvidenceIndex()

	b.ReportAllocs()

	for b.Loop() {
		index.UpdateFrom(thesis)
	}
}

func BenchmarkClockFor(b *testing.B) {
	index := newEvidenceIndex()
	thesis := types.NewThesis()
	mass := 0.5
			thesis.Measurements.Store(types.SourcePumpDump, []*types.Measurement{
		{
			Source: types.SourcePumpDump, Symbol: "SIM/USD",
			At:           time.Unix(10, 0),
			ObservedFrom: time.Unix(9, 0),
			Horizon:      time.Second,
			Validity: types.MeasurementValidity{
				State: types.ValidityValid, Readiness: types.ReadinessObservation,
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricIgnition, types.SideNone): {
					Raw: mass, Normalized: &mass,
				},
			},
		},
	})
	index.UpdateFrom(thesis)
	supporting := []string{string(types.MetricIgnition)}

	b.ReportAllocs()

	for b.Loop() {
		index.clockFor("SIM/USD", supporting)
	}
}

func BenchmarkUpdateMeasurements(b *testing.B) {
	measurements := &sync.Map{}
	symbols := []string{"AAA/USD", "BBB/USD", "CCC/USD"}
	metrics := []types.MetricType{types.MetricIgnition, types.MetricTrend}

	for _, symbol := range symbols {
		for index, metric := range metrics {
			mass := 0.3 + float64(index)/10
			measurements.Store(symbol+string(metric), &types.Measurement{
				Source: types.SourcePumpDump, Symbol: symbol,
				At:           time.Unix(10, 0),
				ObservedFrom: time.Unix(9, 0),
				Horizon:      time.Second,
				Validity: types.MeasurementValidity{
					State: types.ValidityValid, Readiness: types.ReadinessObservation,
				},
				Metrics: map[string]types.MetricSample{
					types.MetricKey(metric, types.SideNone): {
						Raw: mass, Normalized: &mass,
					},
				},
			})
		}
	}

	index := newEvidenceIndex()

	b.ReportAllocs()

	for b.Loop() {
		index.UpdateMeasurements(measurements)
	}
}
