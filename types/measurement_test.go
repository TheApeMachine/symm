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
			Metric: MetricBreadth,
			At:     time.Unix(1, 0),
		}
		btcBreadth := &Measurement{
			Symbol: "BTC/USD",
			Metric: MetricBreadth,
			At:     time.Unix(2, 0),
		}
		btcStrength := &Measurement{
			Symbol: "BTC/USD",
			Metric: MetricStrength,
			At:     time.Unix(2, 0),
		}
		ethLatest := &Measurement{
			Symbol: "ETH/USD",
			Metric: MetricStrength,
			At:     time.Unix(3, 0),
		}

		Convey("Then every symbol keeps its newest complete epoch", func() {
			filtered := FilterLatest([]*Measurement{
				btcOlder,
				btcBreadth,
				btcStrength,
				ethLatest,
			})

			So(filtered, ShouldResemble, []*Measurement{
				btcBreadth,
				btcStrength,
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

	measurements := make(
		[]*Measurement,
		0,
		symbolCount*metricCount*epochCount,
	)

	for symbolIndex := 0; symbolIndex < symbolCount; symbolIndex++ {
		symbol := "PAIR-" + strconv.Itoa(symbolIndex)

		for epochIndex := 0; epochIndex < epochCount; epochIndex++ {
			for metricIndex := 0; metricIndex < metricCount; metricIndex++ {
				measurements = append(measurements, &Measurement{
					Symbol: symbol,
					Metric: MetricStrength,
					At:     time.Unix(int64(epochIndex), 0),
				})
			}
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; b.Loop(); iteration++ {
		if len(FilterLatest(measurements)) != symbolCount*metricCount {
			b.Fatal("latest measurement epoch lost a symbol")
		}
	}
}

func TestForPublish(t *testing.T) {
	Convey("Given flat typed rows for two symbols from one signal", t, func() {
		at := time.Unix(100, 0).UTC()
		rows := []*Measurement{
			{
				Source: SourceSentiment, Stream: Sentiment, Symbol: "BTC/USD",
				Metric: MetricBreadth, At: at, Raw: 1,
				Validity: MeasurementValidity{State: ValidityValid, Readiness: ReadinessObservation},
			},
			{
				Source: SourceSentiment, Stream: Sentiment, Symbol: "BTC/USD",
				Metric: MetricStrength, At: at, Raw: 0.4,
				Validity: MeasurementValidity{State: ValidityValid, Readiness: ReadinessObservation},
			},
			{
				Source: SourceSentiment, Stream: Sentiment, Symbol: "ETH/USD",
				Metric: MetricBreadth, At: at, Raw: 0.5,
				Validity: MeasurementValidity{State: ValidityValid, Readiness: ReadinessObservation},
			},
			{
				Source: SourceSentiment, Stream: Sentiment, Symbol: "ETH/USD",
				Metric: MetricStrength, At: at, Raw: 0.2,
				Validity: MeasurementValidity{State: ValidityValid, Readiness: ReadinessObservation},
			},
		}

		published := ForPublish(rows)

		Convey("It keeps typed rows for both symbols", func() {
			So(published, ShouldHaveLength, 4)
			So(ObservationCount(rows), ShouldEqual, 2)
			So(published[0].Metric, ShouldEqual, MetricBreadth)
			So(published[0].Raw, ShouldEqual, 1.0)
			So(published[0].Symbol, ShouldEqual, "BTC/USD")
		})
	})

	Convey("Given directional Hawkes metrics that share a metric name", t, func() {
		at := time.Unix(100, 0).UTC()
		rows := []*Measurement{
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricArrivalRate, Side: SideBuy, At: at, Raw: 1.5,
			},
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricArrivalRate, Side: SideSell, At: at, Raw: 0.7,
			},
		}

		published := ForPublish(rows)

		Convey("It keeps buy and sell as distinct typed rows", func() {
			So(published, ShouldHaveLength, 2)
			So(published[0].Side, ShouldEqual, SideBuy)
			So(published[0].Raw, ShouldEqual, 1.5)
			So(published[1].Side, ShouldEqual, SideSell)
			So(published[1].Raw, ShouldEqual, 0.7)
		})
	})

	Convey("Given an older epoch for one symbol", t, func() {
		rows := []*Measurement{
			{
				Source: SourcePumpDump, Symbol: "BTC/USD",
				Metric: MetricStrength, At: time.Unix(1, 0), Raw: 0.1,
			},
			{
				Source: SourcePumpDump, Symbol: "BTC/USD",
				Metric: MetricStrength, At: time.Unix(2, 0), Raw: 0.9,
			},
		}

		published := ForPublish(rows)

		Convey("It keeps only the newest complete epoch", func() {
			So(published, ShouldHaveLength, 1)
			So(published[0].Raw, ShouldEqual, 0.9)
		})
	})

	Convey("Given a Hawkes fit epoch older than the live intensity", t, func() {
		fitFrom := time.Unix(80, 0).UTC()
		fitAt := time.Unix(100, 0).UTC()
		evalAt := time.Unix(140, 0).UTC()
		rows := []*Measurement{
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricBaselineIntensity, Side: SideBuy, At: fitAt, Raw: 0.6,
				Scale: ScaleReference{
					Kind: ScaleObservationWindow, From: fitFrom, Through: fitAt,
				},
			},
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricBaselineIntensity, Side: SideSell, At: fitAt, Raw: 0.4,
				Scale: ScaleReference{
					Kind: ScaleObservationWindow, From: fitFrom, Through: fitAt,
				},
			},
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricDecayRate, At: fitAt, Raw: 1.5,
				Scale: ScaleReference{
					Kind: ScaleObservationWindow, From: fitFrom, Through: fitAt,
				},
			},
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricSpectralRadius, At: fitAt, Raw: 0.72,
				Scale: ScaleReference{
					Kind: ScaleObservationWindow, From: fitFrom, Through: fitAt,
				},
			},
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricConditionalIntensity, Side: SideBuy, At: evalAt, Raw: 0.9,
				Scale: ScaleReference{
					Kind: ScaleObservationWindow, From: fitFrom, Through: evalAt,
				},
			},
			{
				Source: SourceHawkes, Stream: Hawkes, Symbol: "BTC/USD",
				Metric: MetricConditionalIntensity, Side: SideSell, At: evalAt, Raw: 0.6,
				Scale: ScaleReference{
					Kind: ScaleObservationWindow, From: fitFrom, Through: evalAt,
				},
			},
		}

		published := ForPublish(rows)

		Convey("It publishes the fit parameters beside the live intensity", func() {
			So(published, ShouldHaveLength, 6)

			var decay, intensity *Measurement

			for _, row := range published {
				switch row.Metric {
				case MetricDecayRate:
					decay = row
				case MetricConditionalIntensity:
					if row.Side == SideBuy {
						intensity = row
					}
				}
			}

			So(decay, ShouldNotBeNil)
			So(intensity, ShouldNotBeNil)
			So(decay.Raw, ShouldEqual, 1.5)
			So(intensity.Raw, ShouldEqual, 0.9)
			So(decay.Scale.Through, ShouldEqual, fitAt)
			So(intensity.Scale.Through, ShouldEqual, evalAt)
		})
	})
}

func BenchmarkObservationCount(b *testing.B) {
	const (
		symbolCount = 256
		metricCount = 9
	)

	rows := make([]*Measurement, 0, symbolCount*metricCount)
	at := time.Unix(100, 0).UTC()

	for symbolIndex := range symbolCount {
		symbol := "PAIR-" + strconv.Itoa(symbolIndex)

		for metricIndex := range metricCount {
			rows = append(rows, &Measurement{
				Source: SourceSentiment,
				Stream: Sentiment,
				Symbol: symbol,
				Metric: MetricStrength,
				At:     at,
				Raw:    float64(metricIndex),
			})
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		if ObservationCount(rows) != symbolCount {
			b.Fatal("observation count drifted")
		}
	}
}
