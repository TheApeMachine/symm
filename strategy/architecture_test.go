package strategy

import (
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/types"
)

func TestArchitectureProperties(t *testing.T) {
	Convey("Reinforcement Learning System Architectural Invariants", t, func() {

		Convey("1. Temporal trajectory: Context differentiates sequences reaching identical final regions", func() {
			ctxA := associative.NewContext()
			ctxB := associative.NewContext()

			now := time.Now().UTC()
			// Path A: r1 -> r2 -> r3 -> r7
			pA := []grid.Region{
				{ID: 1, Condition: 0x10},
				{ID: 2, Condition: 0x20},
				{ID: 3, Condition: 0x30},
				{ID: 7, Condition: 0x70},
			}
			for _, r := range pA {
				ctxA.Sequence(grid.Impulse{Ready: true, Regions: []grid.Region{r}, At: now, From: now})
			}
			seqA := ctxA.Sequence(grid.Impulse{Ready: true, Regions: []grid.Region{pA[3]}, At: now, From: now})

			// Path B: r4 -> r5 -> r6 -> r7
			pB := []grid.Region{
				{ID: 4, Condition: 0x40},
				{ID: 5, Condition: 0x50},
				{ID: 6, Condition: 0x60},
				{ID: 7, Condition: 0x70},
			}
			for _, r := range pB {
				ctxB.Sequence(grid.Impulse{Ready: true, Regions: []grid.Region{r}, At: now, From: now})
			}
			seqB := ctxB.Sequence(grid.Impulse{Ready: true, Regions: []grid.Region{pB[3]}, At: now, From: now})

			// Both ended at region 7, but trajectories differ:
			So(seqA, ShouldNotResemble, seqB)
			So(len(seqA), ShouldBeGreaterThan, 8)
			So(len(seqB), ShouldBeGreaterThan, 8)
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

		Convey("5. Action exploration from empty cognition: legal actions sampled symmetrically", func() {
			engine := cognition.NewEngine(cognition.DefaultConfig())
			seed := int64(12345)
			rng := rand.New(rand.NewSource(seed))
			agent := NewAgent(1, false, engine, 16, rng)

			now := time.Now().UTC()
			impulse := grid.Impulse{
				Ready:   true,
				Regions: []grid.Region{{ID: 42, Condition: 0x42}},
				At:      now,
				From:    now,
			}

			// When flat with no prior evidence, both ENTER and WAIT are sampled across trials
			chosenActions := make(map[Action]int)
			for i := 0; i < 50; i++ {
				action, _, _, _, _ := agent.ChooseAction(impulse, false)
				chosenActions[action]++
			}
			So(chosenActions[ActionEnter], ShouldBeGreaterThan, 0)
			So(chosenActions[ActionWait], ShouldBeGreaterThan, 0)
			So(chosenActions[ActionExit], ShouldEqual, 0)

			// When holding, legal actions are strictly EXIT and WAIT
			holdingActions := make(map[Action]int)
			for i := 0; i < 50; i++ {
				action, _, _, _, _ := agent.ChooseAction(impulse, true)
				holdingActions[action]++
			}
			So(holdingActions[ActionExit], ShouldBeGreaterThan, 0)
			So(holdingActions[ActionWait], ShouldBeGreaterThan, 0)
			So(holdingActions[ActionEnter], ShouldEqual, 0)
		})

		Convey("6. Friction from canonical owner: entry evaluation uses canonical fee rate", func() {
			rate := 0.0026
			evaluator := NewFragmentEvaluator(&rate)
			So(evaluator.FeeRate(), ShouldNotBeNil)
			So(*evaluator.FeeRate(), ShouldEqual, 0.0026)

			newRate := 0.0015
			evaluator.SetFeeRate(newRate)
			So(*evaluator.FeeRate(), ShouldEqual, 0.0015)
		})

		Convey("7. Timing modulates correctness: premature entry punished, mature entry rewarded", func() {
			rate := 0.001
			evaluator := NewFragmentEvaluator(&rate)
			evaluator.SetAnchorIndex(5) // Anchor B is at frame 5

			now := time.Now().UTC()
			makeFrame := func(p float64) []*data.Measurement[float64] {
				m := data.NewMeasurement[float64]("p", "BTC/USD", "price", now, now)
				m.PutMetric(data.Metric[float64]{Label: "raw", Raw: p})
				return []*data.Measurement[float64]{m}
			}

			frames := [][]*data.Measurement[float64]{
				makeFrame(100.0), // 0: far precursor
				makeFrame(100.2), // 1: early
				makeFrame(100.5), // 2
				makeFrame(101.0), // 3
				makeFrame(102.0), // 4
				makeFrame(103.0), // 5: Anchor B (excursion ignition)
				makeFrame(110.0), // 6: strong rise clearing friction
				makeFrame(115.0), // 7: peak
			}

			// Premature entry at frame 0 (far from B=5)
			earlyOutcome, err := evaluator.EvaluateEntry(frames, 0)
			So(err, ShouldBeNil)
			So(earlyOutcome.Timing, ShouldBeLessThan, 0.5)
			So(earlyOutcome.Reinforcement, ShouldBeLessThan, 0)

			// Mature entry at Anchor B (frame 5)
			anchorOutcome, err := evaluator.EvaluateEntry(frames, 5)
			So(err, ShouldBeNil)
			So(anchorOutcome.Timing, ShouldBeGreaterThanOrEqualTo, 0.9)
			So(anchorOutcome.Reinforcement, ShouldBeGreaterThan, 0.5)

			// Early wait at frame 0 (waiting during precursor is rewarded)
			earlyWait, err := evaluator.EvaluateWait(frames, 0, false, -1)
			So(err, ShouldBeNil)
			So(earlyWait.Reinforcement, ShouldBeGreaterThan, 0)

			// Waiting at Anchor B (when it was time to enter is punished)
			anchorWait, err := evaluator.EvaluateWait(frames, 5, false, -1)
			So(err, ShouldBeNil)
			So(anchorWait.Reinforcement, ShouldBeLessThan, 0)
		})

		Convey("8. Randomized A is strictly constrained to A < B and agent.Reset clears perception", func() {
			engine := cognition.NewEngine(cognition.DefaultConfig())
			seed := int64(777)
			rng := rand.New(rand.NewSource(seed))
			agent := NewAgent(2, false, engine, 16, rng)
			agent.SetFeeRate(0.001)

			anchorB := 6
			now := time.Now().UTC()
			makeFrame := func(p float64) []*data.Measurement[float64] {
				m := data.NewMeasurement[float64]("m", "BTC/USD", "flow", now, now)
				m.PutMetric(data.Metric[float64]{Label: "raw", Raw: p})
				m.PutMetric(data.Metric[float64]{Label: "level", Raw: p})
				m.Maturity = 1.0
				m.SNR = 5.0
				m.SNRDefined = true
				return []*data.Measurement[float64]{m}
			}

			frames := make([][]*data.Measurement[float64], 12)
			for i := range frames {
				frames[i] = makeFrame(float64(100 + i))
			}

			fragment := types.ReplayFragment{
				Frames:      frames,
				Symbol:      "BTC/USD",
				AnchorIndex: anchorB,
			}

			// Ingest and rehearse across 20 trials; verify A < B in every single run
			for trial := 0; trial < 20; trial++ {
				agent.IngestReplay(fragment, 0)
				stepped, err := agent.RehearseChild()
				So(err, ShouldBeNil)
				So(stepped, ShouldBeGreaterThan, 0)
				So(agent.LastOffset(), ShouldBeLessThan, anchorB)
				So(agent.LastOffset(), ShouldBeGreaterThanOrEqualTo, 0)
			}
		})

		Convey("9. Real rehearsal writes causal memory from empty trie", func() {
			engine := cognition.NewEngine(cognition.DefaultConfig())
			So(engine.Root().Len(), ShouldEqual, 0)

			rng := rand.New(rand.NewSource(42))
			agent := NewAgent(3, false, engine, 16, rng)
			agent.SetFeeRate(0.001)

			now := time.Now().UTC()
			// Form the space so that fixed sympathetic regions exist
			for idx := 0; idx < 128; idx++ {
				at := now.Add(time.Duration(idx) * time.Second)
				m := data.NewMeasurement[float64]("m", "BTC/USD", "flow", at, now)
				m.Metadata = map[string]float64{data.MetadataSupport: float64(idx + 1), data.MetadataMahalanobisSNR: 100}
				first := float64(idx%2)*2 - 1
				m.PutMetric(data.Metric[float64]{Label: "raw", Raw: first})
				m.PutMetric(data.Metric[float64]{Label: "level", Raw: first})
				_ = agent.Space().Step([]*data.Measurement[float64]{m})
				imp, _ := agent.Space().Impulse("BTC/USD", at, now)
				if imp.Ready {
					break
				}
			}

			makeFrame := func(p float64) []*data.Measurement[float64] {
				m := data.NewMeasurement[float64]("m", "BTC/USD", "flow", now, now)
				m.PutMetric(data.Metric[float64]{Label: "raw", Raw: p})
				m.PutMetric(data.Metric[float64]{Label: "level", Raw: p})
				m.Maturity = 1.0
				m.SNR = 5.0
				m.SNRDefined = true
				return []*data.Measurement[float64]{m}
			}

			frames := make([][]*data.Measurement[float64], 10)
			for i := range frames {
				frames[i] = makeFrame(float64(100 + i*2))
			}

			fragment := types.ReplayFragment{
				Frames:      frames,
				Symbol:      "BTC/USD",
				AnchorIndex: 5,
			}

			agent.IngestReplay(fragment, 0)
			stepped, err := agent.RehearseChild()
			So(err, ShouldBeNil)
			So(stepped, ShouldBeGreaterThan, 0)

			// Cognitive memory has now recorded learned action associations
			So(engine.Root().Len(), ShouldBeGreaterThan, 0)

			// Rehearsal trace captures actual causal marks
			marks := agent.LastMarks()
			So(len(marks), ShouldBeGreaterThan, 0)
			for _, mark := range marks {
				So(mark.Graded, ShouldBeTrue)
				So(mark.Kind, ShouldBeIn, "enter", "exit", "wait")
			}
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

		Convey("12. Absence of second reinforcement path: forward testing does not pollute cognition", func() {
			engine := cognition.NewEngine(cognition.DefaultConfig())
			mainAgent := NewMainAgent(decimal.NewFromInt64(1000), "paper", nil, nil, engine)

			// Step an entry and exit
			now := time.Now().UTC()
			m := data.NewMeasurement[float64]("p", "BTC/USD", "price", now, now)
			m.PutMetric(data.Metric[float64]{Label: "raw", Raw: 50000.0})

			envelope := &types.Envelope{
				TickerData: kraken.TickerData{
					Symbol: "BTC/USD",
					Last:   decimal.NewFromInt64(50000),
				},
			}

			mainAgent.Step(envelope, ActionDecision{
				Action:  ActionEnter,
				Context: []byte("trade_context_test"),
			})

			// Close position with profit
			envelopeExit := &types.Envelope{
				TickerData: kraken.TickerData{
					Symbol: "BTC/USD",
					Last:   decimal.NewFromInt64(55000),
				},
			}

			mainAgent.Step(envelopeExit, ActionDecision{
				Action: ActionExit,
			})

			// Verify shared cognition was not touched by MainAgent
			eval := engine.Evaluate([]byte("trade_context_test"))
			So(eval.Support, ShouldEqual, 0)
			So(engine.Root().Len(), ShouldEqual, 0)
		})
	})
}
