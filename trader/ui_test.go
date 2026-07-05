package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	dashboard "github.com/theapemachine/symm/ui"
)

type messageRecorder struct {
	messages []dashboard.Message
}

func (recorder *messageRecorder) Publish(message dashboard.Message) error {
	recorder.messages = append(recorder.messages, message)
	return nil
}

func TestUIPublish(t *testing.T) {
	Convey("Given a trader UI publisher", t, func() {
		recorder := &messageRecorder{}
		publisher := NewUI(recorder)
		measurement := &logic.Measurement{
			Source: logic.SourceFluid,
			Symbol: "BTC/USD",
			At:     time.Now().UTC(),
			Distribution: map[logic.CategoryType]float64{
				logic.CategoryLaminar: 0.7,
			},
			Confidence:    0.8,
			Strength:      0.6,
			EntryBaseline: 0.25,
			ExitBaseline:  0.25,
			Status:        "measured",
		}

		Convey("When it publishes a market tick", func() {
			err := publisher.Publish(
				channelTicker,
				measurement.At,
				market.RegimeReading{Confidence: 1, Strength: 1, At: measurement.At},
				[]*logic.Measurement{measurement},
				nil,
				nil,
				nil,
			)

			Convey("Then tick, regime, and measurement messages reach the publisher", func() {
				So(err, ShouldBeNil)
				So(recorder.messages, ShouldHaveLength, 3)
				So(recorder.messages[0].Regime, ShouldNotBeNil)
				So(recorder.messages[1].Measurement, ShouldNotBeNil)
				So(recorder.messages[2].Tick, ShouldNotBeNil)
			})
		})
	})
}

func TestUIPublishSnapshots(t *testing.T) {
	Convey("Given a trader UI publisher and signal snapshots", t, func() {
		recorder := &messageRecorder{}
		publisher := NewUI(recorder)
		at := time.Now().UTC()
		snapshots := []SignalSnapshot{
			{Source: logic.SourceManifold, Payload: map[string]any{"type": "manifold"}},
			{Source: logic.SourceResonance, Payload: map[string]any{"type": "resonance"}},
		}

		Convey("When it publishes the tick", func() {
			err := publisher.Publish(
				channelBook,
				at,
				market.RegimeReading{},
				nil,
				nil,
				nil,
				snapshots,
			)

			Convey("Then the dashboard receives the real signal snapshot payloads", func() {
				So(err, ShouldBeNil)
				So(recorder.messages, ShouldHaveLength, 3)
				So(recorder.messages[0].Manifold["type"], ShouldEqual, "manifold")
				So(recorder.messages[1].Resonance["type"], ShouldEqual, "resonance")
				So(recorder.messages[2].Tick, ShouldNotBeNil)
			})
		})
	})
}

func TestUIDecisions(t *testing.T) {
	Convey("Given a trader UI publisher and an admitted action", t, func() {
		recorder := &messageRecorder{}
		publisher := NewUI(recorder)
		action := &logic.Action{
			Type:            logic.ActionMarket,
			Side:            logic.SideBuy,
			Symbol:          "BTC/USD",
			EntryScore:      0.6,
			EntryConfidence: 0.9,
			Fraction:        0.05,
			Verdict:         "allow",
			Reason:          "selected candidate",
		}

		Convey("When it publishes decisions", func() {
			err := publisher.Decisions([]*logic.Action{action})

			Convey("Then the decision message carries typed trader data", func() {
				So(err, ShouldBeNil)
				So(recorder.messages, ShouldHaveLength, 1)
				So(recorder.messages[0].Decision, ShouldNotBeNil)
				So(recorder.messages[0].Decision.ID, ShouldNotBeBlank)
				So(recorder.messages[0].Decision.Symbol, ShouldEqual, "BTC/USD")
				So(recorder.messages[0].Decision.Score, ShouldEqual, 0.6)
				So(recorder.messages[0].Decision.EntryScore, ShouldEqual, 0.6)
				So(recorder.messages[0].Decision.Reason, ShouldEqual, "selected candidate")
			})
		})
	})
}

func TestUIPositions(t *testing.T) {
	Convey("Given a trader UI publisher and position readings", t, func() {
		recorder := &messageRecorder{}
		publisher := NewUI(recorder)
		at := time.Now().UTC()
		readings := []map[string]any{{
			"symbol":        "BTC/USD",
			"quantity":      0.01,
			"mark":          100.0,
			"entry":         90.0,
			"unrealizedPnl": 0.1,
		}}

		Convey("When it publishes positions", func() {
			err := publisher.Positions(readings, "USD", at)

			Convey("Then the position message carries the backend ledger state", func() {
				So(err, ShouldBeNil)
				So(recorder.messages, ShouldHaveLength, 1)
				So(recorder.messages[0].Positions, ShouldNotBeNil)
				So(recorder.messages[0].Positions.Count, ShouldEqual, 1)
				So(recorder.messages[0].Positions.Quote, ShouldEqual, "USD")
				So(recorder.messages[0].Positions.Net, ShouldEqual, 0.1)
			})
		})
	})
}
