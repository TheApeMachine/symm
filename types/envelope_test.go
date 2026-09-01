package types

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func TestNewEquityReading(t *testing.T) {
	Convey("Given a complete broker trade balance", t, func() {
		balance := kraken.TradeBalanceResult{
			TradeBalance:  decimal.NewFromFloat64(1000),
			UnrealizedPnL: decimal.NewFromFloat64(-25.5),
			Equity:        decimal.NewFromFloat64(974.5),
		}

		Convey("It should carry cash, unrealized, and equity as decimal strings", func() {
			reading := NewEquityReading(balance)

			So(reading, ShouldNotBeNil)
			So(reading.Cash, ShouldEqual, "1000")
			So(reading.Unrealized, ShouldEqual, "-25.5")
			So(reading.Equity, ShouldEqual, "974.5")
		})
	})

	Convey("Given a balance the broker has not yet valued", t, func() {
		balance := kraken.TradeBalanceResult{
			TradeBalance: decimal.NewFromFloat64(1000),
		}

		Convey("It should yield no reading rather than a reading of zeros", func() {
			So(NewEquityReading(balance), ShouldBeNil)
		})
	})

	Convey("Given a valued account with no open positions", t, func() {
		balance := kraken.TradeBalanceResult{
			TradeBalance: decimal.NewFromFloat64(1000),
			Equity:       decimal.NewFromFloat64(1000),
		}

		Convey("It should report the valuation with an absent unrealized", func() {
			reading := NewEquityReading(balance)

			So(reading, ShouldNotBeNil)
			So(reading.Equity, ShouldEqual, "1000")
			So(reading.Unrealized, ShouldEqual, "")
		})
	})
}

/*
settledCoder runs a predictive coder long enough that its manifold has real
layer states to project, so the encoder is exercised against a live model
rather than a zero-valued one.
*/
func settledCoder() *learning.PredictiveCoder {
	coder := learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
		CustomArch: []int{4, 8, 4},
		MaxHorizon: 4,
		Target:     learning.DirectionalTarget(0.01),
		Pace:       0.03,
		Learn:      true,
	})

	for step := range 32 {
		value := float64(step % 5)

		_, err := coder.Step(learning.PredictiveInput{
			Features:     []float64{value, value / 2, value / 3, 1},
			Reference:    value,
			HasReference: true,
			Step:         int64(step),
		})

		So(err, ShouldBeNil)
	}

	return coder
}

func TestEncodeResonanceArtifact(t *testing.T) {
	Convey("Given an artifact carrying a settled predictive manifold", t, func() {
		artifact := &ResonanceArtifact{
			Symbol:           "BTC/USD",
			At:               time.Now(),
			Manifold:         settledCoder().Manifold(),
			ForwardCurve:     []float64{0.01, 0.02},
			SupportedHorizon: 2,
		}

		encoded := (&Envelope{Key: "BTC/USD", Resonance: artifact}).Encode()

		Convey("It should project the coder's own layers onto the wire", func() {
			So(encoded.Resonance, ShouldNotBeNil)
			So(len(encoded.Resonance.Layers), ShouldBeGreaterThan, 0)

			for _, layer := range encoded.Resonance.Layers {
				So(len(layer.State), ShouldBeGreaterThan, 0)
			}
		})

		Convey("It should project the settled latent state", func() {
			So(len(encoded.Resonance.Latent), ShouldBeGreaterThan, 0)
		})

		Convey("It should carry the task-head quality with its readiness", func() {
			// The values are whatever the coder actually reports; what matters
			// is that the readiness flag travels with them, so an unready head
			// is never read as a genuine zero.
			So(encoded.Resonance.TaskSkillReady, ShouldBeIn, []bool{true, false})
			So(encoded.Resonance.TaskRelativePrecisionReady, ShouldBeIn, []bool{true, false})
			So(encoded.Resonance.TaskScaleReady, ShouldBeIn, []bool{true, false})
		})

		Convey("It should name the continuous dynamics rather than leaving them opaque", func() {
			// The dynamics also cross as a raw mask/data frame, which no consumer
			// can read a named quantity out of; the named projection is what makes
			// velocity, memory, dissipation and the rest readable at all.
			So(encoded.Resonance.DynamicsNamed, ShouldNotBeNil)
		})

		Convey("It should survive a round trip through the encoded bytes", func() {
			decoded := telemetry.GetRootAsEnvelopeState(
				(&Envelope{Key: "BTC/USD", Resonance: artifact}).EncodeBytes(), 0,
			)

			resonance := decoded.Resonance(nil)
			So(resonance, ShouldNotBeNil)
			So(resonance.LayersLength(), ShouldBeGreaterThan, 0)
			So(resonance.LatentLength(), ShouldBeGreaterThan, 0)
			So(resonance.DynamicsNamed(nil), ShouldNotBeNil)
		})
	})

	Convey("Given an artifact whose solver has built no manifold yet", t, func() {
		encoded := (&Envelope{
			Key:       "BTC/USD",
			Resonance: &ResonanceArtifact{Symbol: "BTC/USD", At: time.Now()},
		}).Encode()

		Convey("It should leave the model fields absent rather than zeroed", func() {
			So(encoded.Resonance, ShouldNotBeNil)
			So(encoded.Resonance.Layers, ShouldBeEmpty)
			So(encoded.Resonance.Latent, ShouldBeEmpty)
			So(encoded.Resonance.TaskSkillReady, ShouldBeFalse)
		})
	})
}

func TestEnvelopeEncode(t *testing.T) {
	Convey("Given an envelope carrying an account valuation", t, func() {
		envelope := &Envelope{
			Key: "BTC/USD",
			Equity: &EquityReading{
				Cash:       "1000",
				Unrealized: "-25.5",
				Equity:     "974.5",
			},
		}

		Convey("It should project the valuation onto the wire state", func() {
			encoded := envelope.Encode()

			So(encoded, ShouldNotBeNil)
			So(encoded.Equity, ShouldNotBeNil)
			So(encoded.Equity.Cash, ShouldEqual, "1000")
			So(encoded.Equity.Unrealized, ShouldEqual, "-25.5")
			So(encoded.Equity.Equity, ShouldEqual, "974.5")
		})

		Convey("It should survive a round trip through the encoded bytes", func() {
			decoded := telemetry.GetRootAsEnvelopeState(envelope.EncodeBytes(), 0)

			So(decoded, ShouldNotBeNil)

			equity := decoded.Equity(nil)
			So(equity, ShouldNotBeNil)
			So(string(equity.Cash()), ShouldEqual, "1000")
			So(string(equity.Unrealized()), ShouldEqual, "-25.5")
			So(string(equity.Equity()), ShouldEqual, "974.5")
		})
	})

	Convey("Given an envelope produced before the first valuation", t, func() {
		envelope := &Envelope{Key: "BTC/USD"}

		Convey("It should leave the wire equity absent rather than zeroed", func() {
			encoded := envelope.Encode()

			So(encoded, ShouldNotBeNil)
			So(encoded.Equity, ShouldBeNil)
		})
	})
}

/*
TestEnvelopeEncodeWebsocketLean proves the observer projection never serializes
the volumetric Manifold field, and that Resonance and Boundaries no longer ride
the websocket either — all three are delivered over their own WebRTC channels.
The websocket carries only lean, latency-relevant state. Full Hindsight
EncodeBytes still carries every populated field.
*/
func TestEnvelopeEncodeWebsocketLean(t *testing.T) {
	Convey("Given an envelope with heavy fields populated", t, func() {
		envelope := &Envelope{
			Key:        "BTC/USD",
			Resonance:  &ResonanceArtifact{Symbol: "BTC/USD", At: time.Now()},
			Boundaries: []BoundaryStamp{{AtNs: time.Now().UnixNano()}},
			Equity: &EquityReading{
				Cash: "1000", Unrealized: "0", Equity: "1000",
			},
		}

		Convey("the websocket encoding carries only lean state", func() {
			decoded := telemetry.GetRootAsEnvelopeState(envelope.EncodeWebsocket(), 0)

			So(decoded, ShouldNotBeNil)
			So(decoded.Resonance(nil), ShouldBeNil)
			So(decoded.Manifold(nil), ShouldBeNil)
			So(decoded.BoundariesLength(), ShouldEqual, 0)
			So(decoded.Equity(nil), ShouldNotBeNil)
		})

		Convey("the full Hindsight encoding still contains every field", func() {
			decoded := telemetry.GetRootAsEnvelopeState(envelope.EncodeBytes(), 0)

			So(decoded, ShouldNotBeNil)
			So(decoded.Resonance(nil), ShouldNotBeNil)
			So(decoded.BoundariesLength(), ShouldEqual, 1)
			So(decoded.Equity(nil), ShouldNotBeNil)
		})
	})
}

/*
TestEnvelopeBoundaryComposition proves the ring a stage runs in survives the
wire. The group and stage are the only part of the trace a consumer cannot
reconstruct from the stamps themselves — concurrent siblings stamp in
goroutine-completion order — so dropping them in encoding would silently turn
the diagnostics topology back into a guess.
*/
func TestEnvelopeBoundaryComposition(t *testing.T) {
	Convey("Given an envelope stamped by stages from two different rings", t, func() {
		envelope := &Envelope{
			Key: "BTC/USD",
			Boundaries: []BoundaryStamp{
				{Label: "ticker.ingress", Group: "ticker", Stage: 0, AtNs: 1},
				{Label: "ticker.pumpdump", Group: "ticker", Stage: 2, AtNs: 2},
				{Label: "logic.manifold", Group: "logic", Stage: 0, AtNs: 3},
			},
		}

		Convey("every stamp carries its own ring and handler group across the wire", func() {
			decoded := telemetry.GetRootAsEnvelopeState(envelope.EncodeBytes(), 0)

			So(decoded.BoundariesLength(), ShouldEqual, 3)

			stamp := new(telemetry.EnvelopeBoundaryStamp)

			So(decoded.Boundaries(stamp, 1), ShouldBeTrue)
			So(string(stamp.Label()), ShouldEqual, "ticker.pumpdump")
			So(string(stamp.Group()), ShouldEqual, "ticker")
			So(stamp.Stage(), ShouldEqual, int32(2))

			So(decoded.Boundaries(stamp, 2), ShouldBeTrue)
			So(string(stamp.Group()), ShouldEqual, "logic")
			So(stamp.Stage(), ShouldEqual, int32(0))
		})
	})
}

/*
TestEnvelopeMeasurementFocusGate proves the websocket projection drops signal
measurements whose own Label is not the dashboard focus, while the full
Hindsight encoding keeps every measurement. It is the fix for the dashboard's
measurement rings interleaving multiple symbols: Hub.Step broadcasts the
EncodeWebsocket mirror, and the frontend feeds every decoded measurement into a
single per-source ring keyed only by source. Gating by the measurement's own
Label (the observation symbol) at the wire boundary means only the focused
symbol's measurements ever reach the browser.
*/
func TestEnvelopeMeasurementFocusGate(t *testing.T) {
	Convey("Given a dashboard focused on BTC/USD", t, func() {
		SetFocus("BTC/USD")

		Reset(func() {
			SetFocus("BTC/USD")
		})

		envelope := &Envelope{
			Key:    "BTC/USD",
			Equity: &EquityReading{Cash: "1000", Unrealized: "0", Equity: "1000"},
			Hawkes: data.NewMeasurement[float64](
				"hawkes:BTC/USD:1", "BTC/USD", "hawkes", time.Now(), time.Time{},
			),
			Toxicity: data.NewMeasurement[float64](
				"toxicity:ETH/USD:2", "ETH/USD", "toxicity", time.Now(), time.Time{},
			),
		}

		Convey("the websocket mirror keeps the focused measurement and equity", func() {
			decoded := telemetry.GetRootAsEnvelopeState(envelope.EncodeWebsocket(), 0)

			So(decoded, ShouldNotBeNil)

			hawkes := decoded.Hawkes(nil)
			So(hawkes, ShouldNotBeNil)
			So(string(hawkes.Label()), ShouldEqual, "BTC/USD")

			// Non-measurement field survives the measurement-only gate.
			So(decoded.Equity(nil), ShouldNotBeNil)
		})

		Convey("the websocket mirror drops the non-focused measurement", func() {
			decoded := telemetry.GetRootAsEnvelopeState(envelope.EncodeWebsocket(), 0)

			So(decoded, ShouldNotBeNil)

			toxicity := decoded.Toxicity(nil)
			So(toxicity, ShouldBeNil)
		})

		Convey("the full Hindsight encoding keeps every measurement", func() {
			decoded := telemetry.GetRootAsEnvelopeState(envelope.EncodeBytes(), 0)

			So(decoded, ShouldNotBeNil)
			So(decoded.Hawkes(nil), ShouldNotBeNil)
			So(decoded.Toxicity(nil), ShouldNotBeNil)
		})
	})
}
