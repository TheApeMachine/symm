package strategy_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/strategy"
)

func TestDecisionNext(t *testing.T) {
	Convey("unseen context yields no action", t, func() {
		holding := false
		decision := strategy.NewDecision(func() bool { return holding })

		unseenEval := cognition.Evaluation{
			Context: []byte("unseen_context"),
			Support: 0,
		}

		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&unseenEval))
		}

		var actions []strategy.Action
		for out := range decision.Next(in) {
			actions = append(actions, *(*strategy.Action)(out))
		}

		So(decision.Error(), ShouldBeNil)
		So(len(actions), ShouldEqual, 0)
	})

	Convey("tie yields abstention", t, func() {
		holding := false
		decision := strategy.NewDecision(func() bool { return holding })

		tieEval := cognition.Evaluation{
			Context:     []byte("ctx"),
			Support:     2,
			WinnerClass: string(strategy.ActionEnter),
			RunnerUp:    string(strategy.ActionExit),
			Confidence:  0.5,
			Contrast:    0.0,
			IsTie:       true,
			Candidates: []cognition.ClassCandidate{
				{Name: string(strategy.ActionEnter), Probability: 0.5, Support: 1},
				{Name: string(strategy.ActionExit), Probability: 0.5, Support: 1},
			},
		}

		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&tieEval))
		}

		var actions []strategy.Action
		for out := range decision.Next(in) {
			actions = append(actions, *(*strategy.Action)(out))
		}

		So(decision.Error(), ShouldBeNil)
		So(len(actions), ShouldEqual, 0)
	})

	Convey("illegal action is rejected downstream", t, func() {
		// Case 1: Winner is Exit while flat (not holding) -> rejected
		holding := false
		decision := strategy.NewDecision(func() bool { return holding })

		exitEval := cognition.Evaluation{
			Context:     []byte("ctx"),
			Support:     5,
			WinnerClass: string(strategy.ActionExit),
			Confidence:  0.9,
			Contrast:    2.0,
			Candidates: []cognition.ClassCandidate{
				{Name: string(strategy.ActionExit), Probability: 0.9, Support: 5},
			},
		}

		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&exitEval))
		}

		var actions []strategy.Action
		for out := range decision.Next(in) {
			actions = append(actions, *(*strategy.Action)(out))
		}

		So(len(actions), ShouldEqual, 0)

		// Case 2: Winner is Enter while holding -> rejected
		holding = true
		enterEval := cognition.Evaluation{
			Context:     []byte("ctx"),
			Support:     5,
			WinnerClass: string(strategy.ActionEnter),
			Confidence:  0.9,
			Contrast:    2.0,
			Candidates: []cognition.ClassCandidate{
				{Name: string(strategy.ActionEnter), Probability: 0.9, Support: 5},
			},
		}

		inEnter := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&enterEval))
		}

		actions = nil
		for out := range decision.Next(inEnter) {
			actions = append(actions, *(*strategy.Action)(out))
		}

		So(len(actions), ShouldEqual, 0)
	})

	Convey("decisive legal action is emitted", t, func() {
		// When flat, Enter is legal and emitted
		holding := false
		decision := strategy.NewDecision(func() bool { return holding })

		enterEval := cognition.Evaluation{
			Context:     []byte("ctx"),
			Support:     5,
			WinnerClass: string(strategy.ActionEnter),
			Confidence:  0.85,
			Contrast:    1.5,
			Candidates: []cognition.ClassCandidate{
				{Name: string(strategy.ActionEnter), Probability: 0.85, Support: 5},
			},
		}

		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&enterEval))
		}

		var actions []strategy.Action
		for out := range decision.Next(in) {
			actions = append(actions, *(*strategy.Action)(out))
		}

		So(decision.Error(), ShouldBeNil)
		So(len(actions), ShouldEqual, 1)
		So(actions[0], ShouldEqual, strategy.ActionEnter)

		// When holding, Exit is legal and emitted
		holding = true
		exitEval := cognition.Evaluation{
			Context:     []byte("ctx"),
			Support:     5,
			WinnerClass: string(strategy.ActionExit),
			Confidence:  0.85,
			Contrast:    1.5,
			Candidates: []cognition.ClassCandidate{
				{Name: string(strategy.ActionExit), Probability: 0.85, Support: 5},
			},
		}

		inExit := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&exitEval))
		}

		actions = nil
		for out := range decision.Next(inExit) {
			actions = append(actions, *(*strategy.Action)(out))
		}

		So(len(actions), ShouldEqual, 1)
		So(actions[0], ShouldEqual, strategy.ActionExit)
	})
}
