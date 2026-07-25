package types

import (
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestObservationValidity(t *testing.T) {
	Convey("Given observation-window evidence counts", t, func() {
		Convey("Then empty windows are invalid", func() {
			validity := ObservationValidity(0)

			So(validity.State, ShouldEqual, ValidityInvalid)
			So(validity.Readiness, ShouldEqual, ReadinessObservation)
			So(validity.Reason, ShouldNotBeEmpty)
		})

		Convey("Then a single event stays provisional", func() {
			validity := ObservationValidity(1)

			So(validity.State, ShouldEqual, ValidityProvisional)
			So(validity.Readiness, ShouldEqual, ReadinessObservation)
			So(validity.Reason, ShouldNotBeEmpty)
		})

		Convey("Then corroborated windows are valid", func() {
			validity := ObservationValidity(2)

			So(validity.State, ShouldEqual, ValidityValid)
			So(validity.Readiness, ShouldEqual, ReadinessObservation)
			So(validity.Reason, ShouldBeEmpty)
		})
	})
}

func TestMeasurementValidateStruct(t *testing.T) {
	Convey("Given forward and backwards evidence intervals", t, func() {
		forward := Measurement{At: time.Unix(2, 0), ObservedFrom: time.Unix(1, 0)}
		backwards := Measurement{At: time.Unix(1, 0), ObservedFrom: time.Unix(2, 0)}

		Convey("Then forward observation provenance is accepted unchanged", func() {
			So(forward.ValidateStruct(), ShouldBeNil)
			So(forward.ObservedFrom, ShouldEqual, time.Unix(1, 0))
			So(forward.At, ShouldEqual, time.Unix(2, 0))
		})

		Convey("Then ObservedFrom after At is rejected without mutation", func() {
			So(backwards.ValidateStruct(), ShouldNotBeNil)
			So(backwards.ObservedFrom, ShouldEqual, time.Unix(2, 0))
			So(backwards.At, ShouldEqual, time.Unix(1, 0))
		})

		Convey("Then a backwards scale interval is rejected without mutation", func() {
			at := time.Unix(2, 0)
			mixed := Measurement{
				At: at,
				Scale: ScaleReference{
					Kind:    ScaleObservationWindow,
					From:    at.Add(3 * time.Second),
					Through: at.Add(time.Second),
				},
			}

			So(mixed.ValidateStruct(), ShouldNotBeNil)
			So(mixed.Scale.From, ShouldEqual, at.Add(3*time.Second))
			So(mixed.Scale.Through, ShouldEqual, at.Add(time.Second))
		})
	})
}

func TestMeasurementInterval(t *testing.T) {
	Convey("Given observation provenance separate from scale", t, func() {
		at := time.Unix(3, 0)
		fitFrom := time.Unix(1, 0)
		explicit := Measurement{
			At: at, ObservedFrom: time.Unix(2, 0),
			Scale: ScaleReference{From: fitFrom, Through: at},
		}
		implicit := Measurement{At: at}
		scaleOnly := Measurement{
			At: at,
			Scale: ScaleReference{
				Kind: ScaleObservationWindow, From: fitFrom, Through: at,
			},
		}

		Convey("Then Interval is ObservedFrom→At and ignores Scale", func() {
			from, through := explicit.Interval()
			implicitFrom, implicitThrough := implicit.Interval()
			scaleFrom, scaleThrough := scaleOnly.Interval()

			So(from, ShouldEqual, time.Unix(2, 0))
			So(through, ShouldEqual, at)
			So(implicitFrom, ShouldEqual, at)
			So(implicitThrough, ShouldEqual, at)
			So(scaleFrom, ShouldEqual, at)
			So(scaleThrough, ShouldEqual, at)
		})
	})
}

func TestFilterLatest(t *testing.T) {
	Convey("Given unsynchronized measurement epochs across symbols", t, func() {
		btcOlder := &Measurement{
			Symbol: "BTC/USD",
			At:     time.Unix(1, 0),
			Metrics: map[string]MetricSample{
				MetricKey(MetricBreadth, SideNone): {Raw: 0},
			},
		}
		btcLatest := &Measurement{
			Symbol: "BTC/USD",
			At:     time.Unix(2, 0),
			Metrics: map[string]MetricSample{
				MetricKey(MetricBreadth, SideNone):  {Raw: 1},
				MetricKey(MetricStrength, SideNone): {Raw: 2},
			},
		}
		ethLatest := &Measurement{
			Symbol: "ETH/USD",
			At:     time.Unix(3, 0),
			Metrics: map[string]MetricSample{
				MetricKey(MetricStrength, SideNone): {Raw: 3},
			},
		}

		Convey("Then every symbol keeps its newest complete epoch", func() {
			filtered := FilterLatest([]*Measurement{
				btcOlder,
				btcLatest,
				ethLatest,
			})

			So(filtered, ShouldResemble, []*Measurement{
				btcLatest,
				ethLatest,
			})
		})
	})
}

func BenchmarkFilterLatest(b *testing.B) {
	const (
		symbolCount = 256
		metricCount = 9
		epochCount  = 3
	)

	measurements := make([]*Measurement, 0, symbolCount*epochCount)

	for symbolIndex := 0; symbolIndex < symbolCount; symbolIndex++ {
		symbol := "PAIR-" + strconv.Itoa(symbolIndex)

		for epochIndex := 0; epochIndex < epochCount; epochIndex++ {
			row := &Measurement{
				Symbol: symbol,
				At:     time.Unix(int64(epochIndex), 0),
				Metrics: make(map[string]MetricSample, metricCount),
			}

			for metricIndex := 0; metricIndex < metricCount; metricIndex++ {
				row.Metrics[MetricKey(MetricStrength, SideNone)] = MetricSample{
					Raw: float64(metricIndex),
				}
			}

			measurements = append(measurements, row)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; b.Loop(); iteration++ {
		if len(FilterLatest(measurements)) != symbolCount {
			b.Fatal("latest measurement epoch lost a symbol")
		}
	}
}

func TestForPublish(t *testing.T) {
	Convey("Given merged rows for two symbols from one signal", t, func() {
		at := time.Unix(100, 0).UTC()
		rows := []*Measurement{
			{
				Source: SourceSentiment, Symbol: "BTC/USD", At: at,
				Metrics: map[string]MetricSample{
					MetricKey(MetricBreadth, SideNone):  {Raw: 1},
					MetricKey(MetricStrength, SideNone): {Raw: 0.4},
				},
				Validity: MeasurementValidity{State: ValidityValid, Readiness: ReadinessObservation},
			},
			{
				Source: SourceSentiment, Symbol: "ETH/USD", At: at,
				Metrics: map[string]MetricSample{
					MetricKey(MetricBreadth, SideNone):  {Raw: 0.5},
					MetricKey(MetricStrength, SideNone): {Raw: 0.2},
				},
				Validity: MeasurementValidity{State: ValidityValid, Readiness: ReadinessObservation},
			},
		}

		published := ForPublish(rows)

		Convey("It keeps one row per symbol with all metrics", func() {
			So(published, ShouldHaveLength, 2)
			So(ObservationCount(rows), ShouldEqual, 2)

			breadth, ok := published[0].Sample(MetricBreadth, SideNone)
			So(ok, ShouldBeTrue)
			So(breadth.Raw, ShouldEqual, 1.0)
			So(published[0].Symbol, ShouldEqual, "BTC/USD")
		})
	})

	Convey("Given directional Hawkes metrics that share a metric name", t, func() {
		at := time.Unix(100, 0).UTC()
		rows := []*Measurement{{
			Source: SourceHawkes, Symbol: "BTC/USD", At: at,
			Metrics: map[string]MetricSample{
				MetricKey(MetricArrivalRate, SideBuy):  {Raw: 1.5},
				MetricKey(MetricArrivalRate, SideSell): {Raw: 0.7},
			},
		}}

		published := ForPublish(rows)

		Convey("It keeps buy and sell under one source×symbol row", func() {
			So(published, ShouldHaveLength, 1)

			buy, ok := published[0].Sample(MetricArrivalRate, SideBuy)
			So(ok, ShouldBeTrue)
			So(buy.Raw, ShouldEqual, 1.5)

			sell, ok := published[0].Sample(MetricArrivalRate, SideSell)
			So(ok, ShouldBeTrue)
			So(sell.Raw, ShouldEqual, 0.7)
		})
	})

	Convey("Given an older epoch for one symbol", t, func() {
		rows := []*Measurement{
			{
				Source: SourcePumpDump, Symbol: "BTC/USD", At: time.Unix(1, 0),
				Metrics: map[string]MetricSample{
					MetricKey(MetricStrength, SideNone): {Raw: 0.1},
				},
			},
			{
				Source: SourcePumpDump, Symbol: "BTC/USD", At: time.Unix(2, 0),
				Metrics: map[string]MetricSample{
					MetricKey(MetricStrength, SideNone): {Raw: 0.9},
				},
			},
		}

		published := ForPublish(rows)

		Convey("It keeps only the newest complete epoch", func() {
			So(published, ShouldHaveLength, 1)

			strength, ok := published[0].Sample(MetricStrength, SideNone)
			So(ok, ShouldBeTrue)
			So(strength.Raw, ShouldEqual, 0.9)
		})
	})

	Convey("Given a Hawkes fit epoch older than the live intensity", t, func() {
		fitFrom := time.Unix(80, 0).UTC()
		fitAt := time.Unix(100, 0).UTC()
		evalAt := time.Unix(140, 0).UTC()
		fitScale := ScaleReference{
			Kind: ScaleObservationWindow, From: fitFrom, Through: fitAt,
		}
		evalScale := ScaleReference{
			Kind: ScaleObservationWindow, From: fitFrom, Through: evalAt,
		}
		rows := []*Measurement{
			{
				Source: SourceHawkes, Symbol: "BTC/USD", At: fitAt, Scale: fitScale,
				Metrics: map[string]MetricSample{
					MetricKey(MetricBaselineIntensity, SideBuy):  {Raw: 0.6},
					MetricKey(MetricBaselineIntensity, SideSell): {Raw: 0.4},
					MetricKey(MetricDecayRate, SideNone):         {Raw: 1.5},
					MetricKey(MetricSpectralRadius, SideNone):    {Raw: 0.72},
				},
			},
			{
				Source: SourceHawkes, Symbol: "BTC/USD", At: evalAt, Scale: evalScale,
				Metrics: map[string]MetricSample{
					MetricKey(MetricConditionalIntensity, SideBuy):  {Raw: 0.9},
					MetricKey(MetricConditionalIntensity, SideSell): {Raw: 0.6},
				},
			},
		}

		published := ForPublish(rows)

		Convey("It publishes the fit parameters beside the live intensity", func() {
			So(published, ShouldHaveLength, 2)

			var fitRow, liveRow *Measurement

			for _, row := range published {
				if row.At.Equal(fitAt) {
					fitRow = row
				}

				if row.At.Equal(evalAt) {
					liveRow = row
				}
			}

			So(fitRow, ShouldNotBeNil)
			So(liveRow, ShouldNotBeNil)

			decay, ok := fitRow.Sample(MetricDecayRate, SideNone)
			So(ok, ShouldBeTrue)
			So(decay.Raw, ShouldEqual, 1.5)

			intensity, ok := liveRow.Sample(MetricConditionalIntensity, SideBuy)
			So(ok, ShouldBeTrue)
			So(intensity.Raw, ShouldEqual, 0.9)
			So(fitRow.Scale.Through, ShouldEqual, fitAt)
			So(liveRow.Scale.Through, ShouldEqual, evalAt)
		})
	})
}

func BenchmarkObservationCount(b *testing.B) {
	const (
		symbolCount = 256
		metricCount = 9
	)

	rows := make([]*Measurement, 0, symbolCount)
	at := time.Unix(100, 0).UTC()

	for symbolIndex := range symbolCount {
		symbol := "PAIR-" + strconv.Itoa(symbolIndex)
		metrics := make(map[string]MetricSample, metricCount)

		for metricIndex := range metricCount {
			metrics[MetricKey(MetricStrength, SideNone)] = MetricSample{
				Raw: float64(metricIndex),
			}
		}

		rows = append(rows, &Measurement{
			Source:  SourceSentiment,
			Symbol:  symbol,
			At:      at,
			Metrics: metrics,
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		if ObservationCount(rows) != symbolCount {
			b.Fatal("observation count drifted")
		}
	}
}
