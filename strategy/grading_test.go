package strategy

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	markettest "github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/types"
)

func TestGrade(t *testing.T) {
	Convey("Completed multi-leg tape evaluates actions without manufactured rewards", t, func() {
		tape := markettest.NewOpportunityTape("TEST/USD", time.Unix(1000, 0), 4)
		for index := 0; index < len(tape.Steps)-tape.HorizonSteps; index++ {
			start, end := tape.Steps[index], tape.Steps[index+tape.HorizonSteps]
			sequence := hindsight.CaptureSequence(index + 1)
			episode := hindsight.Episode{ID: "leg", Symbol: tape.Symbol, Confirmed: true, FromSequence: sequence, ToSequence: sequence + hindsight.CaptureSequence(tape.HorizonSteps), References: []hindsight.ReferencePoint{{HasValue: true, Value: end.ExecutableBid, ReceivedAt: end.EventTime, Capture: hindsight.CaptureIdentity{Sequence: sequence + hindsight.CaptureSequence(tape.HorizonSteps)}}}}
			decision := TraderDecision{Symbol: tape.Symbol, Sequence: sequence, At: start.EventTime, Price: start.ExecutableBid, State: "flat", Action: LearningAction{Kind: types.ActionEnter}}
			verdict, ready, err := Grade(decision, []hindsight.Episode{episode}, -0.5)
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			So(verdict.Tape, ShouldAlmostEqual, math.Log(end.ExecutableBid/start.ExecutableBid))
			So(verdict.Wallet, ShouldEqual, -0.5)
			decision.Action = LearningAction{Kind: types.ActionHold}
			verdict, ready, err = Grade(decision, []hindsight.Episode{episode}, 0)
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			So(verdict.Tape, ShouldEqual, 0)
			decision.State = "holding"
			verdict, _, err = Grade(decision, []hindsight.Episode{episode}, 0)
			So(err, ShouldBeNil)
			So(verdict.Tape, ShouldAlmostEqual, math.Log(end.ExecutableBid/start.ExecutableBid))
			decision.Action = LearningAction{Kind: types.ActionExit, Reduce: true}
			verdict, _, err = Grade(decision, []hindsight.Episode{episode}, 0)
			So(err, ShouldBeNil)
			So(verdict.Tape, ShouldAlmostEqual, -math.Log(end.ExecutableBid/start.ExecutableBid))
			episode.Confirmed = false
			_, ready, err = Grade(decision, []hindsight.Episode{episode}, 0)
			So(err, ShouldBeNil)
			So(ready, ShouldBeFalse)
			episode.Confirmed = true
			decision.Sequence = episode.ToSequence + 1
			_, ready, err = Grade(decision, []hindsight.Episode{episode}, 0)
			So(err, ShouldBeNil)
			So(ready, ShouldBeFalse)
		}
	})
}

func BenchmarkGrade(b *testing.B) {
	at := time.Unix(1, 0)
	decision := TraderDecision{Symbol: "TEST/USD", Sequence: 1, At: at, Price: 100, State: "holding", Action: LearningAction{Kind: types.ActionHold}}
	episodes := []hindsight.Episode{{ID: "leg", Symbol: decision.Symbol, Confirmed: true, FromSequence: 1, ToSequence: 2, References: []hindsight.ReferencePoint{{HasValue: true, Value: 98, ReceivedAt: at.Add(time.Second), Capture: hindsight.CaptureIdentity{Sequence: 2}}}}}
	b.ReportAllocs()
	for b.Loop() {
		_, ready, err := Grade(decision, episodes, 0)
		if err != nil || !ready {
			b.Fatal("confirmed leg did not grade", err)
		}
	}
}
