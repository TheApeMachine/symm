package audit

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDeskDecisionFrameDedupeKey(t *testing.T) {
	Convey("Given a rejected desk decision", t, func() {
		frame := DeskDecisionFrame{
			RecordedAt: time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC),
			Symbol:     "BTC/USD",
			ActionType: "market",
			Verdict:    "rejected",
			Reason:     "spread too wide",
		}

		Convey("It should expose a stable dedupe key", func() {
			So(frame.DedupeKey(), ShouldEqual, "desk_reject:BTC/USD:market:spread too wide")
			So(frame.Payload()["event"], ShouldEqual, "desk_decision")
		})
	})
}

func TestDeadLetterFramePayload(t *testing.T) {
	Convey("Given a dead letter frame", t, func() {
		frame := DeadLetterFrame{
			RecordedAt: time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC),
			Kind:       "order",
			Reason:     "invalid action payload",
			Detail: map[string]any{
				"message_type": "order",
			},
		}

		Convey("It should marshal audit fields", func() {
			payload := frame.Payload()

			So(payload["event"], ShouldEqual, "dead_letter")
			So(payload["kind"], ShouldEqual, "order")
			So(payload["message_type"], ShouldEqual, "order")
		})
	})
}
