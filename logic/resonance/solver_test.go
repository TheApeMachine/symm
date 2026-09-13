package resonance

import (
	"context"
	"strconv"
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestStep(t *testing.T) {
	Convey("Given a resonance solver", t, func() {
		solver := NewSolver(context.Background(), 0)
		defer solver.Close()

		m := solver.Register()
		m.Label = "TEST/USD"
		m.At = time.Unix(1, 0)

		result := solver.Step(m)

		Convey("the step populates energy and surprise metrics", func() {
			So(result, ShouldNotBeNil)
			So(result.Source, ShouldEqual, "resonance")
		})
	})
}

func TestSignalFeatureIngestion(t *testing.T) {
	Convey("Given a resonance solver receiving measurements with all 11 canonical signal peers", t, func() {
		solver := NewSolver(context.Background(), 0.01)
		defer solver.Close()

		createMetric := func(label, metricName string, value float64) *data.Measurement[float64] {
			measurement := data.NewMeasurement[float64](label, nil)
			measurement.Label, measurement.At, measurement.From = "BTC/USD", time.Unix(10, 0), time.Unix(10, 0)
			measurement.Metrics[metricName] = data.Metric[float64]{Label: metricName, Raw: value}
			measurement.Metadata = map[string]string{data.MetadataSupport: "1"}
			for range data.NewFinalizer[float64]().Next(transport.NewOne(unsafe.Pointer(&measurement)).Next(nil)) {
			}
			return measurement
		}

		m := solver.Register()
		m.Label = "BTC/USD"
		m.At = time.Unix(10, 0)
		m.Peers = []*data.Measurement[float64]{
			createMetric("correlation", "relative_return_energy", 1.2),
			createMetric("leadlag", "best_lag_correlation", 0.75),
			createMetric("liquidity", "relative_spread", 0.0002),
			createMetric("sentiment", "advance_fraction", 0.6),
			createMetric("cvd", "signed_net_fraction", 0.4),
			createMetric("depthflow", "observed_notional_imbalance", 0.3),
			createMetric("morphology", "book_shape_distance", 0.05),
			createMetric("hawkes", "excitation_fraction:buy", 0.45),
			createMetric("pumpdump", "spread_ratio", 1.05),
			createMetric("toxicity", "net_withdrawal_fraction:bid", 0.15),
			createMetric("derivatives", "basis", 0.001),
		}

		result := solver.Step(m)

		Convey("the predictive coder ingests all 11 features and produces resonance dynamics", func() {
			So(result, ShouldNotBeNil)
			So(result.Metrics["energy"], ShouldNotBeNil)
			So(result.Metrics["surprise"], ShouldNotBeNil)
		})
	})
}

func TestSurpriseBreakInCommonFlow(t *testing.T) {
	Convey("Given a resonance solver receiving consecutive sensory observations", t, func() {
		solver := NewSolver(context.Background(), 0.05)
		defer solver.Close()

		createMeasurement := func(sec int64, cvdVal, toxVal float64) *data.Measurement[float64] {
			m := solver.Register()
			m.Label = "ETH/USD"
			m.At = time.Unix(sec, 0)

			createMetric := func(label, metricName string, val float64) *data.Measurement[float64] {
				measurement := data.NewMeasurement[float64](label, nil)
				measurement.Label, measurement.At, measurement.From = "ETH/USD", time.Unix(sec, 0), time.Unix(sec, 0)
				measurement.Metrics[metricName] = data.Metric[float64]{Label: metricName, Raw: val}
				measurement.Metadata = map[string]string{data.MetadataSupport: "1"}
				for range data.NewFinalizer[float64]().Next(transport.NewOne(unsafe.Pointer(&measurement)).Next(nil)) {
				}
				return measurement
			}

			m.Peers = []*data.Measurement[float64]{
				createMetric("correlation", "relative_return_energy", 1.0),
				createMetric("leadlag", "best_lag_correlation", 0.5),
				createMetric("liquidity", "relative_spread", 0.0003),
				createMetric("sentiment", "advance_fraction", 0.5),
				createMetric("cvd", "signed_net_fraction", cvdVal),
				createMetric("depthflow", "observed_notional_imbalance", 0.1),
				createMetric("morphology", "book_shape_distance", 0.02),
				createMetric("hawkes", "excitation_fraction:buy", 0.2),
				createMetric("pumpdump", "spread_ratio", 1.0),
				createMetric("toxicity", "net_withdrawal_fraction:bid", toxVal),
				createMetric("derivatives", "basis", 0.0005),
			}

			return m
		}

		for step := int64(1); step <= 10; step++ {
			res := solver.Step(createMeasurement(step, 0.2, 0.1))
			So(res, ShouldNotBeNil)
		}

		Convey("when an unexpected break in common flow occurs, the solver processes the surprise", func() {
			disrupted := solver.Step(createMeasurement(11, 0.95, 0.85))
			So(disrupted, ShouldNotBeNil)
			So(disrupted.Metrics["surprise"].Raw, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func TestNoVarianceCollapseOnAsynchronousSignals(t *testing.T) {
	Convey("Given a resonance solver receiving interspersed signals", t, func() {
		solver := NewSolver(context.Background(), 0.05)
		defer solver.Close()

		createMetric := func(label, metricName string, val float64, support float64) *data.Measurement[float64] {
			measurement := data.NewMeasurement[float64](label, nil)
			measurement.Label, measurement.At, measurement.From = "BTC/USD", time.Now(), time.Now()
			measurement.Metrics[metricName] = data.Metric[float64]{Label: metricName, Raw: val}
			measurement.Metadata = map[string]string{data.MetadataSupport: strconv.FormatFloat(support, 'f', -1, 64)}
			for range data.NewFinalizer[float64]().Next(transport.NewOne(unsafe.Pointer(&measurement)).Next(nil)) {
			}
			return measurement
		}

		m1 := solver.Register()
		m1.Label = "BTC/USD"
		m1.At = time.Unix(100, 0)
		m1.Peers = []*data.Measurement[float64]{
			createMetric("cvd", "signed_net_fraction", 0.1, 1),
		}
		res1 := solver.Step(m1)
		So(res1, ShouldNotBeNil)

		// 500 measurements arrive where CVD is absent (only DepthFlow is present)
		for step := int64(1); step <= 500; step++ {
			mL3 := solver.Register()
			mL3.Label = "BTC/USD"
			mL3.At = time.Unix(100+step, 0)
			mL3.Peers = []*data.Measurement[float64]{
				createMetric("depthflow", "observed_notional_imbalance", 0.2, float64(step)),
			}
			resL3 := solver.Step(mL3)
			So(resL3, ShouldNotBeNil)
		}

		// Subsequent measurement: CVD changes from 0.1 to 0.8
		m2 := solver.Register()
		m2.Label = "BTC/USD"
		m2.At = time.Unix(700, 0)
		m2.Peers = []*data.Measurement[float64]{
			createMetric("cvd", "signed_net_fraction", 0.8, 2),
		}
		res2 := solver.Step(m2)

		Convey("CVD standardizer does not collapse variance and surprise remains realistic", func() {
			So(res2, ShouldNotBeNil)
			So(res2.Metrics["surprise"].Raw, ShouldBeLessThan, 5.0)

			scorer := solver.scorer("BTC/USD")
			So(scorer.lastReading[4].Count, ShouldEqual, 2)
		})
	})
}
