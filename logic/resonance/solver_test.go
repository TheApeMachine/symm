package resonance

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestStep(t *testing.T) {
	Convey("Given a resonance solver with a synchronous observer", t, func() {
		solver := NewSolver(context.Background(), 0)
		defer solver.Close()
		observed := false
		solver.SetObserver(func(envelope *types.Envelope) {
			observed = envelope.Resonance != nil && envelope.Resonance.Manifold != nil
		})
		envelope := types.NewEnvelope(types.EnvelopeTicker)
		envelope.TickerData = kraken.TickerData{
			Symbol:    "TEST/USD",
			Bid:       decimal.NewFromFloat64(99),
			BidQty:    2,
			Ask:       decimal.NewFromFloat64(101),
			AskQty:    2,
			Last:      decimal.NewFromFloat64(100),
			Volume:    10,
			Vwap:      100,
			Low:       decimal.NewFromFloat64(98),
			High:      decimal.NewFromFloat64(102),
			Change:    decimal.NewFromFloat64(1),
			ChangePct: 1,
			Timestamp: time.Unix(1, 0),
		}

		result := solver.Step(envelope)

		Convey("the observer sees the owned model before it leaves the ring", func() {
			So(observed, ShouldBeTrue)
			So(result.Resonance, ShouldNotBeNil)
			So(result.Resonance.Manifold, ShouldBeNil)
		})
	})
}

func TestSignalFeatureIngestion(t *testing.T) {
	Convey("Given a resonance solver receiving envelopes with all 11 canonical signal measurements", t, func() {
		solver := NewSolver(context.Background(), 0.01)
		defer solver.Close()

		createMetric := func(label, metricName string, value float64) *data.Measurement[float64] {
			measurement := data.NewMeasurement[float64](label, "BTC/USD", label, time.Unix(10, 0), time.Unix(10, 0))
			measurement.PutMetric(data.Metric[float64]{
				Label: metricName,
				Raw:   value,
			})
			measurement.Metadata = map[string]float64{data.MetadataSupport: 1}
			measurement.Finalize()
			return measurement
		}

		envelope := types.NewEnvelope(types.EnvelopeTrade)
		envelope.TradeData = kraken.TradeData{
			Symbol:    "BTC/USD",
			Price:     *decimal.NewFromFloat64(50000),
			Qty:       1.5,
			Side:      "buy",
			Timestamp: time.Unix(10, 0),
		}

		envelope.Correlation = createMetric("correlation", "relative_return_energy", 1.2)
		envelope.LeadLag = createMetric("leadlag", "best_lag_correlation", 0.75)
		envelope.Liquidity = createMetric("liquidity", "relative_spread", 0.0002)
		envelope.Sentiment = createMetric("sentiment", "advance_fraction", 0.6)
		envelope.CVD = createMetric("cvd", "signed_net_fraction", 0.4)
		envelope.DepthFlow = createMetric("depthflow", "observed_notional_imbalance", 0.3)
		envelope.Morphology = createMetric("morphology", "book_shape_distance", 0.05)
		envelope.Hawkes = createMetric("hawkes", "excitation_fraction:buy", 0.45)
		envelope.PumpDump = createMetric("pumpdump", "spread_ratio", 1.05)
		envelope.Toxicity = createMetric("toxicity", "net_withdrawal_fraction:bid", 0.15)
		envelope.Derivatives = createMetric("derivatives", "basis", 0.001)

		result := solver.Step(envelope)

		Convey("the predictive coder ingests all 11 features and produces resonance dynamics", func() {
			So(result, ShouldNotBeNil)
			So(result.Resonance, ShouldNotBeNil)
			So(result.Resonance.Symbol, ShouldEqual, "BTC/USD")
			So(result.Resonance.Dynamics, ShouldNotBeNil)
			So(result.Resonance.Dynamics.Ready, ShouldEqual, 1)
			So(result.Resonance.Forecast, ShouldNotBeNil)
		})
	})
}

func TestSurpriseBreakInCommonFlow(t *testing.T) {
	Convey("Given a resonance solver receiving consecutive sensory observations", t, func() {
		solver := NewSolver(context.Background(), 0.05)
		defer solver.Close()

		createEnvelope := func(sec int64, cvdVal, toxVal float64) *types.Envelope {
			envelope := types.NewEnvelope(types.EnvelopeTrade)
			envelope.TradeData = kraken.TradeData{
				Symbol:    "ETH/USD",
				Price:     *decimal.NewFromFloat64(3000),
				Qty:       1.0,
				Side:      "buy",
				Timestamp: time.Unix(sec, 0),
			}

			createMetric := func(label, metricName string, val float64) *data.Measurement[float64] {
				measurement := data.NewMeasurement[float64](label, "ETH/USD", label, time.Unix(sec, 0), time.Unix(sec, 0))
				measurement.PutMetric(data.Metric[float64]{
					Label: metricName,
					Raw:   val,
				})
				measurement.Metadata = map[string]float64{data.MetadataSupport: 1}
				measurement.Finalize()
				return measurement
			}

			envelope.Correlation = createMetric("correlation", "relative_return_energy", 1.0)
			envelope.LeadLag = createMetric("leadlag", "best_lag_correlation", 0.5)
			envelope.Liquidity = createMetric("liquidity", "relative_spread", 0.0003)
			envelope.Sentiment = createMetric("sentiment", "advance_fraction", 0.5)
			envelope.CVD = createMetric("cvd", "signed_net_fraction", cvdVal)
			envelope.DepthFlow = createMetric("depthflow", "observed_notional_imbalance", 0.1)
			envelope.Morphology = createMetric("morphology", "book_shape_distance", 0.02)
			envelope.Hawkes = createMetric("hawkes", "excitation_fraction:buy", 0.2)
			envelope.PumpDump = createMetric("pumpdump", "spread_ratio", 1.0)
			envelope.Toxicity = createMetric("toxicity", "net_withdrawal_fraction:bid", toxVal)
			envelope.Derivatives = createMetric("derivatives", "basis", 0.0005)

			return envelope
		}

		for step := int64(1); step <= 10; step++ {
			res := solver.Step(createEnvelope(step, 0.2, 0.1))
			So(res, ShouldNotBeNil)
			So(res.Resonance, ShouldNotBeNil)
		}

		Convey("when an unexpected break in common flow occurs, the solver processes the surprise", func() {
			disrupted := solver.Step(createEnvelope(11, 0.95, 0.85))
			So(disrupted, ShouldNotBeNil)
			So(disrupted.Resonance, ShouldNotBeNil)
			So(disrupted.Resonance.Dynamics, ShouldNotBeNil)
		})
	})
}
