package correlation

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

func sessionSignals(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{NewSignal(ctx, api, channel)}
}

func TestSignal_MeasureFromMarket(testingTB *testing.T) {
	Convey("Given correlation inside a paper Session cohort market", testingTB, func() {
		herdSession, err := tests.NewSession(testingTB, tests.SessionOptions{
			Signals: sessionSignals,
		})
		So(err, ShouldBeNil)
		noiseSession, err := tests.NewSession(testingTB, tests.SessionOptions{
			Signals: sessionSignals,
		})
		So(err, ShouldBeNil)

		Convey("When herd and noise cohorts play through Cut", func() {
			herdTheses, err := herdSession.Play(conditions.Herd(32).Frames())
			So(err, ShouldBeNil)
			noiseTheses, err := noiseSession.Play(conditions.Noise(32).Frames())
			So(err, ShouldBeNil)

			herd, hasHerd := tests.PeakSourceMetric(
				herdTheses,
				types.SourceCorrelation,
				conditions.Subject(),
				types.MetricHerdScore,
			)
			noiseOnHerd, _ := tests.PeakSourceMetric(
				herdTheses,
				types.SourceCorrelation,
				conditions.Subject(),
				types.MetricNoiseScore,
			)
			noise, hasNoise := tests.PeakSourceMetric(
				noiseTheses,
				types.SourceCorrelation,
				conditions.Subject(),
				types.MetricNoiseScore,
			)

			Convey("Then herd and noise paths diverge directionally", func() {
				So(hasHerd || hasNoise, ShouldBeTrue)

				if hasHerd && hasNoise {
					So(herd+1, ShouldBeGreaterThan, noiseOnHerd)
					So(noise+1, ShouldBeGreaterThan, 0)
				}
			})
		})
	})
}

func BenchmarkSignal_MeasureFromMarket(benchmark *testing.B) {
	session, err := tests.NewSession(benchmark, tests.SessionOptions{
		Signals: sessionSignals,
	})

	if err != nil {
		benchmark.Fatal(err)
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := session.Play(conditions.Herd(16).Frames()); err != nil {
			benchmark.Fatal(err)
		}
	}
}
