package resonance

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func metricRaw(measurement *data.Measurement, label string) (float64, bool) {
	if measurement == nil {
		return 0, false
	}

	for entry := range measurement.Read(label) {
		if entry.Err != nil {
			continue
		}
		return entry.Metric.Raw, true
	}

	return 0, false
}

func signalPeer(source, symbol, metricName string, value float64, at time.Time) *data.Measurement {
	measurement := data.NewMeasurement(1, symbol, source, at.Unix(), at.Unix())
	measurement.At = at
	measurement.From = at
	return measurement.Write(data.NewMetric(metricName, value, data.UnitDimensionless, data.TimescaleInstantaneous))
}

func TestStep(t *testing.T) {
	Convey("Given a resonance solver", t, func() {
		solver := NewSolver(context.Background(), 0)
		defer solver.Close()

		at := time.Unix(1, 0).UTC()
		prior := data.NewMeasurement(1, "TEST/USD", "ingress", 1, 1)
		prior.At = at
		prior.From = at
		prior = prior.Write(data.NewMetric("midpoint", 100, data.UnitPrice, data.TimescaleInstantaneous))

		result := solver.Step(prior)

		Convey("the step produces a finalized resonance measurement", func() {
			So(result, ShouldNotBeNil)
			_, held := metricRaw(result, "energy")
			So(held, ShouldBeTrue)
		})
	})
}

func TestSignalFeatureIngestion(t *testing.T) {
	Convey("Given a resonance solver receiving measurements with all 11 canonical signal peers", t, func() {
		solver := NewSolver(context.Background(), 0.01)
		defer solver.Close()

		at := time.Unix(10, 0).UTC()
		peers := []*data.Measurement{
			signalPeer("correlation", "BTC/USD", "relative_return_energy", 1.2, at),
			signalPeer("leadlag", "BTC/USD", "covariance_score_gain_median", 0.75, at),
			signalPeer("liquidity", "BTC/USD", "relative_spread", 0.0002, at),
			signalPeer("sentiment", "BTC/USD", "advance_fraction", 0.6, at),
			signalPeer("cvd", "BTC/USD", "signed_net_fraction", 0.4, at),
			signalPeer("depthflow", "BTC/USD", "book_imbalance", 0.3, at),
			signalPeer("morphology", "BTC/USD", "book_shape_distance", 0.05, at),
			signalPeer("hawkes", "BTC/USD", "excitation_fraction:buy", 0.45, at),
			signalPeer("pumpdump", "BTC/USD", "spread_ratio", 1.05, at),
			signalPeer("toxicity", "BTC/USD", "net_withdrawal_fraction:bid", 0.15, at),
			signalPeer("derivatives", "BTC/USD", "basis", 0.001, at),
		}

		m := data.NewMeasurement(1, "BTC/USD", "runtime:join", 10, 10)
		m.Peers(peers...)
		m.At = at
		m.From = at
		m = m.Write(data.NewMetric("midpoint", 50000, data.UnitPrice, data.TimescaleInstantaneous))

		result := solver.Step(m)

		Convey("the predictive coder ingests all 11 features and produces resonance dynamics", func() {
			So(result, ShouldNotBeNil)
			_, energyHeld := metricRaw(result, "energy")
			_, surpriseHeld := metricRaw(result, "surprise")
			So(energyHeld, ShouldBeTrue)
			So(surpriseHeld, ShouldBeTrue)
		})

		Convey("every layer of the predictive-coding stack is published", func() {
			arch := coderArch(11)
			So(len(arch), ShouldEqual, 4)

			for layer, rows := range arch {
				_, errHeld := metricRaw(result, fmt.Sprintf("layer_%d_error", layer))
				So(errHeld, ShouldBeTrue)

				_, firstHeld := metricRaw(result, fmt.Sprintf("layer_%d_state_0", layer))
				So(firstHeld, ShouldBeTrue)

				_, lastHeld := metricRaw(result, fmt.Sprintf("layer_%d_state_%d", layer, rows-1))
				So(lastHeld, ShouldBeTrue)

				_, predHeld := metricRaw(result, fmt.Sprintf("layer_%d_prediction_%d", layer, rows-1))
				So(predHeld, ShouldBeTrue)

				_, pastHeld := metricRaw(result, fmt.Sprintf("layer_%d_state_%d", layer, rows))
				So(pastHeld, ShouldBeFalse)
			}

			_, extraHeld := metricRaw(result, fmt.Sprintf("layer_%d_state_0", len(arch)))
			So(extraHeld, ShouldBeFalse)
		})
	})
}

func TestSurpriseBreakInCommonFlow(t *testing.T) {
	Convey("Given a resonance solver receiving consecutive sensory observations", t, func() {
		solver := NewSolver(context.Background(), 0.05)
		defer solver.Close()

		createMeasurement := func(sec int64, cvdVal, toxVal float64) *data.Measurement {
			at := time.Unix(sec, 0).UTC()
			peers := []*data.Measurement{
				signalPeer("correlation", "ETH/USD", "relative_return_energy", 1.0, at),
				signalPeer("leadlag", "ETH/USD", "covariance_score_gain_median", 0.5, at),
				signalPeer("liquidity", "ETH/USD", "relative_spread", 0.0003, at),
				signalPeer("sentiment", "ETH/USD", "advance_fraction", 0.5, at),
				signalPeer("cvd", "ETH/USD", "signed_net_fraction", cvdVal, at),
				signalPeer("depthflow", "ETH/USD", "book_imbalance", 0.1, at),
				signalPeer("morphology", "ETH/USD", "book_shape_distance", 0.02, at),
				signalPeer("hawkes", "ETH/USD", "excitation_fraction:buy", 0.2, at),
				signalPeer("pumpdump", "ETH/USD", "spread_ratio", 1.0, at),
				signalPeer("toxicity", "ETH/USD", "net_withdrawal_fraction:bid", toxVal, at),
				signalPeer("derivatives", "ETH/USD", "basis", 0.0005, at),
			}

			m := data.NewMeasurement(1, "ETH/USD", "runtime:join", sec, sec)
			m.Peers(peers...)
			m.At = at
			m.From = at
			return m.Write(data.NewMetric("midpoint", 3000, data.UnitPrice, data.TimescaleInstantaneous))
		}

		for step := int64(1); step <= 10; step++ {
			res := solver.Step(createMeasurement(step, 0.2, 0.1))
			So(res, ShouldNotBeNil)
		}

		Convey("when an unexpected break in common flow occurs, the solver processes the surprise", func() {
			disrupted := solver.Step(createMeasurement(11, 0.95, 0.85))
			So(disrupted, ShouldNotBeNil)
			surprise, held := metricRaw(disrupted, "surprise")
			So(held, ShouldBeTrue)
			So(surprise, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func TestNoVarianceCollapseOnAsynchronousSignals(t *testing.T) {
	Convey("Given a resonance solver receiving interspersed signals", t, func() {
		solver := NewSolver(context.Background(), 0.05)
		defer solver.Close()
		solver.Transition(runtime.READY)

		at := time.Unix(100, 0).UTC()
		m1 := data.NewMeasurement(1, "BTC/USD", "runtime:join", 100, 100)
		m1.Peers(signalPeer("cvd", "BTC/USD", "signed_net_fraction", 0.1, at))
		m1.At = at
		m1.From = at
		m1 = m1.Write(data.NewMetric("midpoint", 50000, data.UnitPrice, data.TimescaleInstantaneous))
		res1 := solver.Step(m1)
		So(res1, ShouldNotBeNil)

		at2 := time.Unix(101, 0).UTC()
		m2 := data.NewMeasurement(1, "BTC/USD", "runtime:join", 101, 101)
		m2.Peers(signalPeer("toxicity", "BTC/USD", "net_withdrawal_fraction:bid", 0.2, at2))
		m2.At = at2
		m2.From = at2
		m2 = m2.Write(data.NewMetric("midpoint", 50010, data.UnitPrice, data.TimescaleInstantaneous))
		res2 := solver.Step(m2)
		So(res2, ShouldNotBeNil)

		Convey("asynchronous partial sensory envelopes still produce resonance output", func() {
			_, held := metricRaw(res2, "energy")
			So(held, ShouldBeTrue)
		})
	})
}

func TestPeerSuffixedHeadlineMetrics(t *testing.T) {
	Convey("Given correlation peer-suffixed metrics", t, func() {
		at := time.Unix(1, 0).UTC()
		peer := data.NewMeasurement(1, "ETH/USD", "correlation", 1, 1)
		peer.At = at
		peer.From = at
		peer = peer.Write(data.NewMetric(
			"covariance_score@BTC/USD", 0.82, data.UnitCovarianceScore, data.TimescaleRollingWindow,
		))

		value, ok := extractHeadlineMetric(0, peer)
		So(ok, ShouldBeTrue)
		So(value, ShouldEqual, 0.82)
	})
}

func TestPublishReturnsRejectsMisshapedLayers(t *testing.T) {
	Convey("Given a layer reading that disagrees with the architecture", t, func() {
		var out [12][]float64
		out[7] = make([]float64, 2+2*3+1)

		_, _, err := publishReturns(out, []int{3})
		So(err, ShouldNotBeNil)
	})
}
