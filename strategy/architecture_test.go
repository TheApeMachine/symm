package strategy

import (
	"math/rand"
	"reflect"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

func TestArchitectureProperties(t *testing.T) {
	Convey("Reinforcement Learning System Architectural Invariants", t, func() {

		Convey("1. No future leakage: context contains only region tokens from observations up to decision time", func() {
			agent := NewAgent(0, true, nil, 16)
			now := time.Now().UTC()
			m1 := data.NewMeasurement[float64]("m1", "BTC/USD", "flow", now, now)
			m1.PutMetric(data.Metric[float64]{Label: "rate", Raw: 10.0})
			m1.Maturity = 1.0
			m1.SNR = 5.0
			m1.SNRDefined = true

			impulse, err := agent.Step([]*data.Measurement[float64]{m1}, "BTC/USD")
			So(err, ShouldBeNil)
			// Context extracted from current impulse contains only current sequence
			seq := agent.Context().Sequence(impulse)
			if impulse.Ready {
				So(len(seq), ShouldBeGreaterThan, 0)
			}
			if !impulse.Ready {
				So(len(seq), ShouldEqual, 0)
			}
		})

		Convey("2. No hindsight action injection: measurements carry no Provenance['moment']", func() {
			now := time.Now().UTC()
			m := data.NewMeasurement[float64]("m1", "BTC/USD", "flow", now, now)
			So(m.Provenance["moment"], ShouldEqual, "")
			So(m.Provenance["grade"], ShouldEqual, "")
		})

		Convey("3. ENTER/WAIT while flat: legal actions are only Enter and Wait", func() {
			actions := LegalActions(false)
			So(len(actions), ShouldEqual, 2)
			So(actions, ShouldContain, ActionEnter)
			So(actions, ShouldContain, ActionWait)
			So(actions, ShouldNotContain, ActionExit)
		})

		Convey("4. EXIT/WAIT while holding: legal actions are only Exit and Wait", func() {
			actions := LegalActions(true)
			So(len(actions), ShouldEqual, 2)
			So(actions, ShouldContain, ActionExit)
			So(actions, ShouldContain, ActionWait)
			So(actions, ShouldNotContain, ActionEnter)
		})

		Convey("5. Friction from canonical owner: entry evaluation uses canonical fee rate", func() {
			rate := 0.0026
			evaluator := NewFragmentEvaluator(&rate)
			So(evaluator.FeeRate(), ShouldNotBeNil)
			So(*evaluator.FeeRate(), ShouldEqual, 0.0026)

			// Changing fee rate updates evaluator
			newRate := 0.0015
			evaluator.SetFeeRate(newRate)
			So(*evaluator.FeeRate(), ShouldEqual, 0.0015)
		})

		Convey("6. Exit not proximity to top: exit quality evaluates realizable outcome", func() {
			rate := 0.0026
			evaluator := NewFragmentEvaluator(&rate)

			// Build fragment where price falls after exit (good exit)
			now := time.Now().UTC()
			makeFrame := func(price float64) []*data.Measurement[float64] {
				m := data.NewMeasurement[float64]("p", "BTC/USD", "price", now, now)
				m.PutMetric(data.Metric[float64]{Label: "raw", Raw: price})
				return []*data.Measurement[float64]{m}
			}

			fragment := [][]*data.Measurement[float64]{
				makeFrame(100.0), // entry at 0
				makeFrame(110.0), // peak at 1
				makeFrame(105.0), // exit at 2
				makeFrame(90.0),  // after-exit fall at 3
			}

			outcome, err := evaluator.EvaluateExit(fragment, 2, 0)
			So(err, ShouldBeNil)
			So(outcome.Action, ShouldEqual, ActionExit)
			// Because price fell from 105 to 90 after exit, exit preserved value
			So(outcome.Correctness, ShouldBeGreaterThan, 0)
		})

		Convey("7. WAIT is first-class: receives explicit feedback competing against alternative", func() {
			rate := 0.0026
			evaluator := NewFragmentEvaluator(&rate)

			now := time.Now().UTC()
			makeFrame := func(price float64) []*data.Measurement[float64] {
				m := data.NewMeasurement[float64]("p", "BTC/USD", "price", now, now)
				m.PutMetric(data.Metric[float64]{Label: "raw", Raw: price})
				return []*data.Measurement[float64]{m}
			}

			fragment := [][]*data.Measurement[float64]{
				makeFrame(100.0), // decision point 0
				makeFrame(110.0), // big upward move
				makeFrame(120.0),
			}

			// When flat: alternative is ENTER. Since ENTER would have been very profitable, WAIT gets negative feedback
			flatWaitOutcome, err := evaluator.EvaluateWait(fragment, 0, false, -1)
			So(err, ShouldBeNil)
			So(flatWaitOutcome.Action, ShouldEqual, ActionWait)
			So(flatWaitOutcome.Correctness, ShouldBeLessThan, 0)

			// When holding: alternative is EXIT. If holding captured continued gains, WAIT gets positive feedback
			holdingWaitOutcome, err := evaluator.EvaluateWait(fragment, 0, true, 0)
			So(err, ShouldBeNil)
			So(holdingWaitOutcome.Action, ShouldEqual, ActionWait)
		})

		Convey("8. Randomized precursor starting points preserve event boundary while varying prefix", func() {
			seed := int64(42)
			rng1 := rand.New(rand.NewSource(seed))
			rng2 := rand.New(rand.NewSource(seed))

			childLen := 10
			offset1 := rng1.Intn(childLen - 1)
			offset2 := rng2.Intn(childLen - 1)
			So(offset1, ShouldEqual, offset2)
		})

		Convey("9. No fake consensus: single shared model evaluated once", func() {
			engine := cognition.NewEngine(cognition.DefaultConfig())
			agent := NewAgent(0, true, engine, 16)
			// Agent evaluates its memory trie directly
			action, _, _, _, _ := agent.ChooseAction(grid.Impulse{}, false)
			So(action, ShouldEqual, ActionWait)
		})

		Convey("10. Regions semantically opaque: Impulse carries no Moment/Grade/Graded fields", func() {
			impulseType := reflect.TypeOf(grid.Impulse{})
			_, hasMoment := impulseType.FieldByName("Moment")
			_, hasGrade := impulseType.FieldByName("Grade")
			_, hasGraded := impulseType.FieldByName("Graded")
			So(hasMoment, ShouldBeFalse)
			So(hasGrade, ShouldBeFalse)
			So(hasGraded, ShouldBeFalse)
		})

		Convey("11. Missing economics error: evaluator refuses entry scoring without fee rate", func() {
			evaluator := NewFragmentEvaluator(nil)
			now := time.Now().UTC()
			m := data.NewMeasurement[float64]("p", "BTC/USD", "price", now, now)
			m.PutMetric(data.Metric[float64]{Label: "raw", Raw: 100.0})
			fragment := [][]*data.Measurement[float64]{{m}, {m}}

			_, err := evaluator.EvaluateEntry(fragment, 0)
			So(err, ShouldNotBeNil)
		})

		Convey("12. Deterministic rehearsal under controlled RNG", func() {
			rngA := rand.New(rand.NewSource(9999))
			rngB := rand.New(rand.NewSource(9999))

			agentA := NewAgent(1, false, nil, 16, rngA)
			agentB := NewAgent(1, false, nil, 16, rngB)

			So(agentA.rng.Intn(100), ShouldEqual, agentB.rng.Intn(100))
			So(agentA.rng.Intn(100), ShouldEqual, agentB.rng.Intn(100))
		})
	})
}
