package strategy

import (
	"context"
	"testing"

	nmstore "github.com/theapemachine/nomagique/store"
	. "github.com/smartystreets/goconvey/convey"
)

func TestStageStepsGridToken(t *testing.T) {
	Convey("Given a grid reading", t, func() {
		seq := NewStage(context.Background(), nil).Sequencer("BTC/USD")

		reading := nmstore.ObserveReading{
			Oracle: &nmstore.Region{Key: 9, Brightness: 0.7},
			Action: &nmstore.Region{Key: 2, Brightness: 0.9},
		}

		cut := seq.Step(reading.TokenSymbol(), reading.TokenBrightness())

		So(cut.String(), ShouldEqual, "R09.R02")
	})
}
