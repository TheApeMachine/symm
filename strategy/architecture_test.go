package strategy

import (
	"bytes"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/types"
)

func formAgentSpace(targetAgent *Agent, now time.Time) {
	calibrationStart := now.Add(-time.Hour)
	for idx := 0; idx < 128; idx++ {
		at := calibrationStart.Add(time.Duration(idx) * time.Second)
		measurement := data.NewMeasurement[float64]("flow", nil)
		measurement.Label, measurement.At, measurement.From = "BTC/USD", at, at
		measurement.Metadata = map[string]float64{data.MetadataSupport: float64(idx + 1), data.MetadataMahalanobisSNR: 100}
		first := float64(idx%2)*2 - 1
		measurement.Metrics["raw"] = data.Metric[float64]{Label: "raw", Raw: first}
		measurement.Metrics["level"] = data.Metric[float64]{Label: "level", Raw: first}
		impulse, _ := targetAgent.Step([]*data.Measurement[float64]{measurement}, "BTC/USD")
		if impulse.Ready {
			break
		}
	}
}

func TestArchitectureProperties(t *testing.T) {
	Convey("Reinforcement Learning System Architectural Invariants", t, func() {

		Convey("1. Suffix Invariance: Prefix decisions are identical regardless of divergent suffixes", func() {
			seed := int64(999)
			now := time.Now().UTC()

			makeFrame := func(priceValue float64, source string) []*data.Measurement[float64] {
				mPrice := data.NewMeasurement[float64]("price", nil)
				mPrice.Label, mPrice.At, mPrice.From = "BTC/USD", now, now
				mPrice.Metrics["price"] = data.Metric[float64]{Label: "price", Raw: priceValue}
				mFlow := data.NewMeasurement[float64](source, nil)
				mFlow.Label, mFlow.At, mFlow.From = "BTC/USD", now, now
				mFlow.Metrics["raw"] = data.Metric[float64]{Label: "raw", Raw: priceValue}
				mFlow.Metrics["level"] = data.Metric[float64]{Label: "level", Raw: priceValue}
				return []*data.Measurement[float64]{mPrice, mFlow}
			}

			// Prefix: 6 identical frames (0..5)
			prefix := make([][]*data.Measurement[float64], 6)
			for frameIdx := 0; frameIdx < 6; frameIdx++ {
				prefix[frameIdx] = makeFrame(100.0+float64(frameIdx)*0.5, "flow")
			}

			// Fragment 1 suffix: strong rally to 150
			framesRally := make([][]*data.Measurement[float64], 10)
			copy(framesRally, prefix)
			for frameIdx := 6; frameIdx < 10; frameIdx++ {
				framesRally[frameIdx] = makeFrame(110.0+float64(frameIdx-5)*10.0, "flow")
			}

			// Fragment 2 suffix: sharp crash to 50
			framesCrash := make([][]*data.Measurement[float64], 10)
			copy(framesCrash, prefix)
			for frameIdx := 6; frameIdx < 10; frameIdx++ {
				framesCrash[frameIdx] = makeFrame(95.0-float64(frameIdx-5)*10.0, "flow")
			}

			fragRally := types.ReplayFragment{
				Frames:        framesRally,
				Symbol:        "BTC/USD",
				AnchorIndex:   5,
				ExtremumIndex: 9,
			}
			fragCrash := types.ReplayFragment{
				Frames:        framesCrash,
				Symbol:        "BTC/USD",
				AnchorIndex:   5,
				ExtremumIndex: 9,
			}

			formSpace := func(targetAgent *Agent) {
				for idx := 0; idx < 128; idx++ {
					at := now.Add(time.Duration(idx) * time.Second)
					m := data.NewMeasurement[float64]("flow", nil)
					m.Label, m.At, m.From = "BTC/USD", at, now
					m.Metadata = map[string]float64{data.MetadataSupport: float64(idx + 1), data.MetadataMahalanobisSNR: 100}
					first := float64(idx%2)*2 - 1
					m.Metrics["raw"] = data.Metric[float64]{Label: "raw", Raw: first}
					m.Metrics["level"] = data.Metric[float64]{Label: "level", Raw: first}
					imp, _ := targetAgent.Step([]*data.Measurement[float64]{m}, "BTC/USD")
					if imp.Ready {
						break
					}
				}
			}

			engine1 := cognition.NewEngine(cognition.Config{})
			agent1 := NewAgent(1, false, engine1, 16, rand.New(rand.NewSource(seed)))
			formSpace(agent1)

			engine2 := cognition.NewEngine(cognition.Config{})
			agent2 := NewAgent(2, false, engine2, 16, rand.New(rand.NewSource(seed)))
			formSpace(agent2)

			agent1.IngestReplay(fragRally, 0)
			_, err1 := agent1.RehearseChild()
			So(err1, ShouldBeNil)

			agent2.IngestReplay(fragCrash, 0)
			_, err2 := agent2.RehearseChild()
			So(err2, ShouldBeNil)

			marks1 := agent1.LastMarks()
			marks2 := agent2.LastMarks()
			So(len(marks1), ShouldBeGreaterThan, 0)
			So(len(marks2), ShouldBeGreaterThan, 0)
			So(len(marks1), ShouldEqual, len(marks2))

			prefixDecisions := 0
			for markIdx := range marks1 {
				if marks1[markIdx].Index <= 5 {
					So(marks1[markIdx].Kind, ShouldEqual, marks2[markIdx].Kind)
					prefixDecisions++
				}
			}
			So(prefixDecisions, ShouldBeGreaterThan, 0)

			// The post-replay reinforcements must differ because suffixes diverged radically
			reinforcementsDiffer := false
			for markIdx := range marks1 {
				if marks1[markIdx].Value != marks2[markIdx].Value {
					reinforcementsDiffer = true
					break
				}
			}
			So(reinforcementsDiffer, ShouldBeTrue)
		})

		Convey("2. Region-set boundary preservation: [r1, r2] -> [r3] differs from [r1] -> [r2, r3]", func() {
			ctx1 := associative.NewContext()
			ctx2 := associative.NewContext()
			now := time.Now().UTC()

			r1 := grid.Region{ID: 1, Condition: 0x1111}
			r2 := grid.Region{ID: 2, Condition: 0x2222}
			r3 := grid.Region{ID: 3, Condition: 0x3333}

			// Sequence 1: [r1, r2] at step 1, [r3] at step 2
			sequenceOf(t, ctx1, grid.Impulse{Ready: true, Regions: []grid.Region{r1, r2}, Version: 1, At: now, From: now})
			seq1 := sequenceOf(t, ctx1, grid.Impulse{Ready: true, Regions: []grid.Region{r3}, Version: 2, At: now, From: now})

			// Sequence 2: [r1] at step 1, [r2, r3] at step 2
			sequenceOf(t, ctx2, grid.Impulse{Ready: true, Regions: []grid.Region{r1}, Version: 1, At: now, From: now})
			seq2 := sequenceOf(t, ctx2, grid.Impulse{Ready: true, Regions: []grid.Region{r2, r3}, Version: 2, At: now, From: now})

			So(seq1, ShouldNotResemble, seq2)

			engine := cognition.NewEngine(cognition.Config{})
			mustObserve(t, engine, seq1, []byte("ENTER"), 1.0)

			eval1 := mustEvaluate(t, engine, seq1)
			eval2 := mustEvaluate(t, engine, seq2)

			So(eval1.WinnerClass, ShouldEqual, "ENTER")
			So(eval2.WinnerClass, ShouldNotEqual, "ENTER")
		})

		Convey("3. Excursion boundary enforcement: evaluator strictly rejects missing boundaries", func() {
			evaluator := NewFragmentEvaluator()
			now := time.Now().UTC()

			m1 := data.NewMeasurement[float64]("flow", nil)
			m1.Label, m1.At, m1.From = "BTC/USD", now, now
			m1.Metrics["raw"] = data.Metric[float64]{Label: "raw", Raw: 100.0}
			m2 := data.NewMeasurement[float64]("flow", nil)
			m2.Label, m2.At, m2.From = "BTC/USD", now, now
			m2.Metrics["raw"] = data.Metric[float64]{Label: "raw", Raw: 120.0}

			fragment := [][]*data.Measurement[float64]{{m1}, {m2}}

			// Missing anchor and extremum boundaries
			_, err := evaluator.EvaluateEntry(fragment, 0)
			So(err, ShouldNotBeNil)
		})

		Convey("4. Tape excursion ground truth: entry at or before anchor B captures the leg while entry after extremum C buys the dump", func() {
			evaluator := NewFragmentEvaluator()
			evaluator.SetAnchorIndex(1)
			evaluator.SetExtremumIndex(2)

			now := time.Now().UTC()
			makeFrame := func(symbol string) []*data.Measurement[float64] {
				m := data.NewMeasurement[float64]("signal", nil)
				m.Label, m.At, m.From = symbol, now, now
				m.Metrics["val"] = data.Metric[float64]{Label: "val", Raw: 1.0}
				return []*data.Measurement[float64]{m}
			}

			// 4 frames: precursor at 0, anchor at 1, peak at 2, retracement at 3
			frag := [][]*data.Measurement[float64]{
				makeFrame("BTC/USD"),
				makeFrame("BTC/USD"),
				makeFrame("BTC/USD"),
				makeFrame("BTC/USD"),
			}

			// Entry at anchor B captures the excursion
			outcomeA, errA := evaluator.EvaluateEntry(frag, 1)
			So(errA, ShouldBeNil)
			So(outcomeA.Correctness, ShouldBeGreaterThan, 0)

			// Entry after peak C buys into the retracement dump
			outcomeB, errB := evaluator.EvaluateEntry(frag, 3)
			So(errB, ShouldBeNil)
			So(outcomeB.Correctness, ShouldEqual, -1.0)
		})

		Convey("5. Excursion ground truth exit protection: exit at peak captures full gain while premature exit cuts winner short", func() {
			evaluator := NewFragmentEvaluator()
			evaluator.SetAnchorIndex(0)
			evaluator.SetExtremumIndex(1)

			now := time.Now().UTC()
			makeFrame := func() []*data.Measurement[float64] {
				m := data.NewMeasurement[float64]("signal", nil)
				m.Label, m.At, m.From = "BTC/USD", now, now
				m.Metrics["val"] = data.Metric[float64]{Label: "val", Raw: 1.0}
				return []*data.Measurement[float64]{m}
			}

			frames := [][]*data.Measurement[float64]{
				makeFrame(),
				makeFrame(),
			}

			// Case A: Premature exit at index 0 before peak at index 1
			outcomeA, errA := evaluator.EvaluateExit(frames, 0, 0)
			So(errA, ShouldBeNil)
			So(outcomeA.Correctness, ShouldBeLessThan, 0) // Premature exit

			// Case B: Peak exit at index 1 capturing excursion
			outcomeB, errB := evaluator.EvaluateExit(frames, 1, 0)
			So(errB, ShouldBeNil)
			So(outcomeB.Correctness, ShouldBeGreaterThan, 0) // Peak exit
		})

		Convey("6. Behavioral action learning: held-out contexts separate ENTER vs WAIT after rehearsal", func() {
			engine := cognition.NewEngine(cognition.Config{})
			seed := int64(555)
			rng := rand.New(rand.NewSource(seed))

			// Build two distinguishable temporal contexts with length-framed timesteps
			ctxBull := associative.NewContext()
			now := time.Now().UTC()
			var seqBull []byte
			for idx := 0; idx < 3; idx++ {
				at := now.Add(time.Duration(idx) * time.Second)
				imp := grid.Impulse{
					At:      at,
					From:    at,
					Ready:   true,
					Version: uint64(idx + 1),
					Regions: []grid.Region{
						{ID: uint64(100 + idx), Condition: uint64(0x1000 + idx)},
					},
				}
				seqBull = sequenceOf(t, ctxBull, imp)
			}
			So(len(seqBull), ShouldBeGreaterThan, 0)

			ctxBear := associative.NewContext()
			var seqBear []byte
			for idx := 0; idx < 3; idx++ {
				at := now.Add(time.Duration(idx) * time.Second)
				imp := grid.Impulse{
					At:      at,
					From:    at,
					Ready:   true,
					Version: uint64(idx + 1),
					Regions: []grid.Region{
						{ID: uint64(200 + idx), Condition: uint64(0x2000 + idx)},
					},
				}
				seqBear = sequenceOf(t, ctxBear, imp)
			}
			So(len(seqBear), ShouldBeGreaterThan, 0)
			So(bytes.Equal(seqBull, seqBear), ShouldBeFalse)

			// Train repeatedly:
			// seqBull represents a precursor leading to profitable rally -> reinforces ENTER, inhibits WAIT
			// seqBear represents a precursor leading to crash -> inhibits ENTER, reinforces WAIT
			for r := 0; r < 5; r++ {
				mustObserve(t, engine, seqBull, []byte(ActionEnter), 0.85)
				mustObserve(t, engine, seqBull, []byte(ActionWait), -0.85)

				mustObserve(t, engine, seqBear, []byte(ActionEnter), -0.85)
				mustObserve(t, engine, seqBear, []byte(ActionWait), 0.85)
			}

			// Evaluate held-out contexts on cognition engine
			evalBull := mustEvaluate(t, engine, seqBull)
			So(evalBull.Support, ShouldBeGreaterThan, 0)
			So(evalBull.WinnerClass, ShouldEqual, string(ActionEnter))

			evalBear := mustEvaluate(t, engine, seqBear)
			So(evalBear.Support, ShouldBeGreaterThan, 0)
			So(evalBear.WinnerClass, ShouldEqual, string(ActionWait))

			// Prove agent policy chooses ENTER on bullish context and WAIT on bearish context
			actionBull, shareBull, contrastBull, suppBull := selectLegalAction(
				[]Action{ActionEnter, ActionWait},
				evalBull.Candidates,
				true,
				rng,
			)
			So(actionBull, ShouldEqual, ActionEnter)
			So(shareBull, ShouldBeGreaterThan, 0.5)
			So(contrastBull, ShouldBeGreaterThan, 0)
			So(suppBull, ShouldBeGreaterThan, 0)

			actionBear, shareBear, contrastBear, suppBear := selectLegalAction(
				[]Action{ActionEnter, ActionWait},
				evalBear.Candidates,
				true,
				rng,
			)
			So(actionBear, ShouldEqual, ActionWait)
			So(shareBear, ShouldBeGreaterThan, 0.5)
			So(contrastBear, ShouldBeGreaterThan, 0)
			So(suppBear, ShouldBeGreaterThan, 0)
		})

		Convey("7. Missing economics prevents action: MainAgent takes zero economic action when fees are absent", func() {
			engine := cognition.NewEngine(cognition.Config{})
			instrument := broker.NewInstrumentWithQuote("USD")
			price := broker.NewPrice(nil, instrument)
			mainAgent := NewMainAgent(decimal.NewFromInt64(1000), "paper", instrument, price, engine)

			envelope := &types.Envelope{
				TickerData: kraken.TickerData{
					Symbol: "NOFEE/USD",
					Last:   decimal.NewFromInt64(100),
				},
			}

			mainAgent.Step(envelope, ActionDecision{
				Action:  ActionEnter,
				Context: []byte("nofee_context"),
			})

			So(len(mainAgent.positions), ShouldEqual, 0)
			So(mainAgent.fills, ShouldEqual, 0)
			So(mainAgent.cash.Cmp(decimal.NewFromInt64(1000)), ShouldEqual, 0)
		})

		Convey("8. Excursion timing: evaluation derives timing relative to anchor B and extremum C", func() {
			evaluator := NewFragmentEvaluator()
			evaluator.SetAnchorIndex(1)
			evaluator.SetExtremumIndex(3)

			now := time.Now().UTC()
			makeFrame := func(val float64) []*data.Measurement[float64] {
				m := data.NewMeasurement[float64]("flow", nil)
				m.Label, m.At, m.From = "BTC/USD", now, now
				m.Metrics["level"] = data.Metric[float64]{Label: "level", Raw: val}
				return []*data.Measurement[float64]{m}
			}

			frames := [][]*data.Measurement[float64]{
				makeFrame(1.0), // 0: precursor
				makeFrame(2.0), // 1: anchor B
				makeFrame(3.0), // 2: climb
				makeFrame(4.0), // 3: extremum C
			}

			// Entry at frame 1 (anchor B): optimal entry captures entire excursion
			optimalOutcome, err1 := evaluator.EvaluateEntry(frames, 1)
			So(err1, ShouldBeNil)
			So(optimalOutcome.Timing, ShouldEqual, 1.0)
			So(optimalOutcome.Reinforcement, ShouldBeGreaterThan, 0)

			// Entry at frame 2 (between B and C): captures less of the excursion
			lateOutcome, err2 := evaluator.EvaluateEntry(frames, 2)
			So(err2, ShouldBeNil)
			So(lateOutcome.Timing, ShouldBeLessThan, optimalOutcome.Timing)
		})

		Convey("9. Randomized A is strictly constrained to A < B and agent.Reset clears perception", func() {
			engine := cognition.NewEngine(cognition.Config{})
			seed := int64(777)
			rng := rand.New(rand.NewSource(seed))
			agent := NewAgent(2, false, engine, 16, rng)

			anchorB := 6
			now := time.Now().UTC()
			makeFrame := func(val float64) []*data.Measurement[float64] {
				mFlow := data.NewMeasurement[float64]("flow", nil)
				mFlow.Label, mFlow.At, mFlow.From = "BTC/USD", now, now
				mFlow.Metrics["raw"] = data.Metric[float64]{Label: "raw", Raw: val}
				mFlow.Metrics["level"] = data.Metric[float64]{Label: "level", Raw: val}
				return []*data.Measurement[float64]{mFlow}
			}

			frames := make([][]*data.Measurement[float64], 12)
			for frameIdx := range frames {
				frames[frameIdx] = makeFrame(float64(100 + frameIdx))
			}

			fragment := types.ReplayFragment{
				Frames:        frames,
				Symbol:        "BTC/USD",
				AnchorIndex:   anchorB,
				ExtremumIndex: 11,
			}

			for trial := 0; trial < 20; trial++ {
				agent.IngestReplay(fragment, 0)
				stepped, err := agent.RehearseChild()
				So(err, ShouldBeNil)
				So(stepped, ShouldBeGreaterThan, 0)
				So(agent.LastOffset(), ShouldBeLessThan, anchorB)
				So(agent.LastOffset(), ShouldBeGreaterThanOrEqualTo, 0)
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

		Convey("11. Missing anchor index returns explicit validation error", func() {
			engine := cognition.NewEngine(cognition.Config{})
			agent := NewAgent(1, false, engine, 16)
			frag := types.ReplayFragment{
				Frames:      [][]*data.Measurement[float64]{{{Label: "BTC/USD"}}},
				Symbol:      "BTC/USD",
				AnchorIndex: -1,
			}
			agent.IngestReplay(frag, 0)
			_, err := agent.RehearseChild()
			So(err, ShouldNotBeNil)
		})

		Convey("12. Absence of second reinforcement path: forward testing does not pollute cognition", func() {
			engine := cognition.NewEngine(cognition.Config{})
			instrument := broker.NewInstrumentWithQuote("USD")
			price := broker.NewPrice(nil, instrument)
			price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
			mainAgent := NewMainAgent(decimal.NewFromInt64(1000), "paper", instrument, price, engine)

			now := time.Now().UTC()
			m := data.NewMeasurement[float64]("price", nil)
			m.Label, m.At, m.From = "BTC/USD", now, now
			m.Metrics["price"] = data.Metric[float64]{Label: "price", Raw: 50000.0}

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

			envelopeExit := &types.Envelope{
				TickerData: kraken.TickerData{
					Symbol: "BTC/USD",
					Last:   decimal.NewFromInt64(55000),
				},
			}

			mainAgent.Step(envelopeExit, ActionDecision{
				Action: ActionExit,
			})

			eval := mustEvaluate(t, engine, []byte("trade_context_test"))
			So(eval.Support, ShouldEqual, 0)
			So(mustTree(t, engine).Len(), ShouldEqual, 0)
		})

		Convey("13. Production economic replay wiring: Hindsight derives execution surfaces and price measurements that train agent cognition", func() {
			series := make([]hindsight.Observation, 0, 400)
			now := time.Now().UTC()
			seq := uint64(0)
			addRamp := func(startPrice, endPrice float64, steps int) {
				for idx := 0; idx < steps; idx++ {
					fraction := float64(idx) / float64(steps)
					priceVal := startPrice + (endPrice-startPrice)*fraction
					at := now.Add(time.Duration(seq) * time.Second)
					series = append(series, hindsight.Observation{
						Capture:    hindsight.CaptureIdentity{Run: "run-test", Sequence: types.CaptureSequence(seq)},
						Ordinal:    1,
						ReceivedAt: at,
						VenueAt:    at,
						Symbol:     "BTC/USD",
						HasBid:     true,
						Bid:        priceVal - 0.05,
						HasAsk:     true,
						Ask:        priceVal + 0.05,
						HasLast:    true,
						Last:       priceVal,
					})
					seq++
				}
			}
			addRamp(100, 100, 100)
			addRamp(100, 120, 100)
			addRamp(120, 100, 100)
			addRamp(100, 105, 100)

			catalog := tablestest.New(t)
			writer := tables.NewWriter(catalog)

			for _, obs := range series {
				identity := tables.EnvelopeRefRow{
					Run:      "run-test",
					Sequence: int64(obs.Capture.Sequence),
					Ordinal:  1,
				}
				env := &types.Envelope{Key: "BTC/USD"}
				measurement := data.NewMeasurement[float64]("cvd", nil)
				measurement.Label, measurement.At, measurement.From = "BTC/USD", obs.At(), obs.At()
				measurement.Metrics["level"] = data.Metric[float64]{Label: "level", Raw: obs.Bid}
				env.CVD = measurement
				writer.AddWitness(tables.WitnessRow{
					Run: identity.Run, Envelope: identity, ArtifactKind: "precursor",
					Boundary: "after-logic", Payload: env.EncodePrecursor(),
				})
			}
			So(writer.Commit(t.Context()), ShouldBeNil)

			tape := hindsight.Query(hindsight.Excursions, catalog, "run-test", hindsight.DefaultDiscoveryPolicy())
			fragments := tape.ReplayFragmentsFrom(series)
			So(len(fragments), ShouldBeGreaterThan, 0)

			// Real fragment carries objective prices from observation with ZERO manual injection
			// Surfaces are NOT fabricated when depth was not recorded
			frag := fragments[0]

			engine := cognition.NewEngine(cognition.Config{})
			agent := NewAgent(10, false, engine, 16, rand.New(rand.NewSource(12345)))
			formAgentSpace(agent, now)

			agent.IngestReplay(frag, 0)
			stepped, err := agent.RehearseChild()
			So(err, ShouldBeNil)
			So(stepped, ShouldBeGreaterThan, 0)

			marks := agent.LastMarks()
			So(len(marks), ShouldBeGreaterThan, 0)
			So(mustTree(t, engine).Len(), ShouldBeGreaterThan, 0)
		})

		Convey("14. Pump-exit timing criterion: WAIT at 100 > EXIT at 100, EXIT at 115/120 > WAIT there, WAIT into 80 strongly punished", func() {
			feeRate := 0.001
			evaluator := NewFragmentEvaluator(&feeRate)

			now := time.Now().UTC()
			makeFrame := func(priceValue float64) []*data.Measurement[float64] {
				measurement := data.NewMeasurement[float64]("price", nil)
				measurement.Label, measurement.At, measurement.From = "BTC/USD", now, now
				measurement.Metrics["price"] = data.Metric[float64]{Label: "price", Raw: priceValue}
				return []*data.Measurement[float64]{measurement}
			}

			pumpFrames := [][]*data.Measurement[float64]{
				makeFrame(100.0), // 0: price 100
				makeFrame(120.0), // 1: pump peak at 120
				makeFrame(115.0), // 2: beginning dump at 115
				makeFrame(80.0),  // 3: crash to 80
			}

			entryIdx := 0 // Position entered at 100

			// 1. At 100 (frame 0): WAIT > EXIT (premature to exit before pump to 120)
			wait0, err := evaluator.EvaluateWait(pumpFrames, 0, true, entryIdx)
			So(err, ShouldBeNil)
			exit0, err := evaluator.EvaluateExit(pumpFrames, 0, entryIdx)
			So(err, ShouldBeNil)
			So(wait0.Correctness, ShouldBeGreaterThan, exit0.Correctness)
			So(exit0.Correctness, ShouldBeLessThan, 0)

			// 2. At 120 (frame 1): EXIT > WAIT (peak reached, protects against dump to 80)
			wait1, err := evaluator.EvaluateWait(pumpFrames, 1, true, entryIdx)
			So(err, ShouldBeNil)
			exit1, err := evaluator.EvaluateExit(pumpFrames, 1, entryIdx)
			So(err, ShouldBeNil)
			So(exit1.Correctness, ShouldBeGreaterThan, wait1.Correctness)
			So(exit1.Correctness, ShouldBeGreaterThan, 0)

			// 3. At 115 (frame 2): EXIT > WAIT (protects against further dump to 80)
			wait2, err := evaluator.EvaluateWait(pumpFrames, 2, true, entryIdx)
			So(err, ShouldBeNil)
			exit2, err := evaluator.EvaluateExit(pumpFrames, 2, entryIdx)
			So(err, ShouldBeNil)
			So(exit2.Correctness, ShouldBeGreaterThan, wait2.Correctness)
			So(exit2.Correctness, ShouldBeGreaterThan, 0)

			// 4. Into 80 (frame 3): WAIT is strongly punished (<= -0.5) for terminal loss
			wait3, err := evaluator.EvaluateWait(pumpFrames, 3, true, entryIdx)
			So(err, ShouldBeNil)
			So(wait3.Correctness, ShouldBeLessThanOrEqualTo, -0.5)
		})

		Convey("15. Entry timing counterfactual: in [100, 90, 120], WAIT at 100 > ENTER at 100, ENTER at 90 > WAIT at 90", func() {
			feeRate := 0.001
			evaluator := NewFragmentEvaluator(&feeRate)

			now := time.Now().UTC()
			makeFrame := func(priceValue float64) []*data.Measurement[float64] {
				measurement := data.NewMeasurement[float64]("price", nil)
				measurement.Label, measurement.At, measurement.From = "BTC/USD", now, now
				measurement.Metrics["price"] = data.Metric[float64]{Label: "price", Raw: priceValue}
				return []*data.Measurement[float64]{measurement}
			}

			dipFrames := [][]*data.Measurement[float64]{
				makeFrame(100.0), // 0: initial price 100
				makeFrame(90.0),  // 1: dip to 90
				makeFrame(120.0), // 2: rally to 120
			}

			// When flat:
			// At 100 (frame 0): WAIT > ENTER because a substantially better entry (90) is available
			enter0, err := evaluator.EvaluateEntry(dipFrames, 0)
			So(err, ShouldBeNil)
			wait0, err := evaluator.EvaluateWait(dipFrames, 0, false, 0)
			So(err, ShouldBeNil)
			So(wait0.Correctness, ShouldBeGreaterThan, enter0.Correctness)
			So(enter0.Correctness, ShouldBeLessThan, 0)

			// At 90 (frame 1): ENTER > WAIT because 90 is optimal entry before the rally to 120
			enter1, err := evaluator.EvaluateEntry(dipFrames, 1)
			So(err, ShouldBeNil)
			wait1, err := evaluator.EvaluateWait(dipFrames, 1, false, 0)
			So(err, ShouldBeNil)
			So(enter1.Correctness, ShouldBeGreaterThan, wait1.Correctness)
			So(enter1.Correctness, ShouldBeGreaterThan, 0)
		})

		Convey("16. Tape excursion boundary evaluation: entry at anchor B evaluates positive while entry past peak C evaluates loss", func() {
			feeRate := 0.001
			evaluator := NewFragmentEvaluator(&feeRate)

			now := time.Now().UTC()
			makeFrame := func(priceValue float64) []*data.Measurement[float64] {
				measurement := data.NewMeasurement[float64]("price", nil)
				measurement.Label, measurement.At, measurement.From = "BTC/USD", now, now
				measurement.Metrics["price"] = data.Metric[float64]{Label: "price", Raw: priceValue}
				return []*data.Measurement[float64]{measurement}
			}

			chartFrames := [][]*data.Measurement[float64]{
				makeFrame(100.0),
				makeFrame(100.0),
				makeFrame(120.0),
				makeFrame(90.0),
			}
			evaluator.SetAnchorIndex(1)
			evaluator.SetExtremumIndex(2)

			outcomeAnchor, err := evaluator.EvaluateEntry(chartFrames, 1)
			So(err, ShouldBeNil)
			So(outcomeAnchor.Correctness, ShouldBeGreaterThan, 0)

			outcomeLate, err := evaluator.EvaluateEntry(chartFrames, 3)
			So(err, ShouldBeNil)
			So(outcomeLate.Correctness, ShouldEqual, -1.0)
		})

		Convey("17. Behavioral learning via rehearsal: ReplayFragments train cognition via RehearseChild with zero manual Observe", func() {
			engine := cognition.NewEngine(cognition.Config{})
			seed := int64(1337)
			agent := NewAgent(5, false, engine, 16, rand.New(rand.NewSource(seed)))
			formAgentSpace(agent, time.Now().UTC())

			now := time.Now().UTC()
			makeFlowFrame := func(sigValue float64, step int) []*data.Measurement[float64] {
				at := now.Add(time.Duration(step) * time.Second)
				mFlow := data.NewMeasurement[float64]("flow", nil)
				mFlow.Label, mFlow.At, mFlow.From = "BTC/USD", at, at
				mFlow.Metrics["raw"] = data.Metric[float64]{Label: "raw", Raw: sigValue}
				mFlow.Metrics["level"] = data.Metric[float64]{Label: "level", Raw: sigValue}
				return []*data.Measurement[float64]{mFlow}
			}

			// Bullish fragment: precursor at 100 with dynamic signals, then huge rally to 150
			bullFrames := make([][]*data.Measurement[float64], 8)
			surfaces := make([]*types.ExecutionSurface, 8)
			for idx := 0; idx < 4; idx++ {
				bullFrames[idx] = makeFlowFrame(float64(idx+1), idx)
				surfaces[idx] = &types.ExecutionSurface{
					BestAsk: decimal.NewFromFloat64(100.0),
					BestBid: decimal.NewFromFloat64(99.95),
				}
			}
			for idx := 4; idx < 8; idx++ {
				bullFrames[idx] = makeFlowFrame(5.0, idx)
				rallyPrice := 100.0 + float64(idx-3)*15.0
				surfaces[idx] = &types.ExecutionSurface{
					BestAsk: decimal.NewFromFloat64(rallyPrice),
					BestBid: decimal.NewFromFloat64(rallyPrice - 0.05),
				}
			}
			bullFrag := types.ReplayFragment{
				Frames:      bullFrames,
				Symbol:      "BTC/USD",
				AnchorIndex: 4,
			}

			for iter := 0; iter < 10; iter++ {
				agent.IngestReplay(bullFrag, iter)
				stepped, err := agent.RehearseChild()
				So(err, ShouldBeNil)
				So(stepped, ShouldBeGreaterThan, 0)
			}

			So(mustTree(t, engine).Len(), ShouldBeGreaterThan, 0)
			answers := agent.Answers()
			So(len(answers), ShouldBeGreaterThan, 0)
			So(answers[0].Asked, ShouldNotEqual, "action")
			So(answers[0].Asked, ShouldBeIn, []string{"enter", "wait", "exit"})
			So(answers[0].Answered, ShouldBeIn, []string{"enter", "wait", "exit"})

			// Live evaluation of the learned precursor context on held-out agent with deterministic policy (rng == nil):
			evalAgent := NewAgent(99, false, engine, 16)
			formAgentSpace(evalAgent, now)

			var chosenActions []Action
			var supports []uint64
			holding := false
			for idx := 0; idx < 4; idx++ {
				meas := bullFrames[idx]
				imp, err := evalAgent.Step(meas, "BTC/USD")
				So(err, ShouldBeNil)
				decision, err := evalAgent.ChooseAction(imp, holding)
				So(err, ShouldBeNil)
				chosenActions = append(chosenActions, decision.Action)
				supports = append(supports, decision.Support)
				if decision.Action == ActionEnter {
					holding = true
				}
			}

			So(supports[0], ShouldBeGreaterThan, 0)
			So(chosenActions[0], ShouldEqual, ActionEnter)
			So(chosenActions[1], ShouldEqual, ActionWait)
			So(chosenActions[2], ShouldEqual, ActionWait)
			So(chosenActions[3], ShouldEqual, ActionWait)
		})

		Convey("18. Live context adaptive suffix matching: 100-step live history recalls learned precursor sequence", func() {
			engine := cognition.NewEngine(cognition.Config{})
			seed := int64(42)
			agent := NewAgent(8, false, engine, 16, rand.New(rand.NewSource(seed)))
			formAgentSpace(agent, time.Now().UTC())

			now := time.Now().UTC()
			ctxLearned := associative.NewContext()
			var learnedSeq []byte

			for idx := 0; idx < 12; idx++ {
				at := now.Add(time.Duration(idx) * time.Second)
				meas := data.NewMeasurement[float64]("flow", nil)
				meas.Label, meas.At, meas.From = "BTC/USD", at, now
				val := float64((idx % 3) + 1)
				meas.Metrics["raw"] = data.Metric[float64]{Label: "raw", Raw: val}
				meas.Metrics["level"] = data.Metric[float64]{Label: "level", Raw: val}
				imp, _ := agent.Step([]*data.Measurement[float64]{meas}, "BTC/USD")
				learnedSeq = sequenceOf(t, ctxLearned, imp)
			}
			So(len(learnedSeq), ShouldBeGreaterThan, 0)

			mustObserve(t, engine, learnedSeq, []byte("ENTER"), 1.0)

			ctxLive := associative.NewContext()
			var liveSeq []byte
			for idx := 0; idx < 100; idx++ {
				at := now.Add(time.Duration(100+idx) * time.Second)
				meas := data.NewMeasurement[float64]("flow", nil)
				meas.Label, meas.At, meas.From = "BTC/USD", at, now
				val := float64(-1.0)
				if idx >= 88 {
					val = float64(((idx - 88) % 3) + 1)
				}
				meas.Metrics["raw"] = data.Metric[float64]{Label: "raw", Raw: val}
				meas.Metrics["level"] = data.Metric[float64]{Label: "level", Raw: val}
				imp, _ := agent.Step([]*data.Measurement[float64]{meas}, "BTC/USD")
				liveSeq = sequenceOf(t, ctxLive, imp)
			}

			eval := mustEvaluate(t, engine, liveSeq)
			So(eval.Support, ShouldBeGreaterThan, 0)
			So(eval.WinnerClass, ShouldEqual, "ENTER")
		})

		Convey("19. Excursion peak evaluation: exiting at peak C reinforces EXIT and holding through retracement punishes WAIT", func() {
			feeRate := 0.001
			evaluator := NewFragmentEvaluator(&feeRate)

			now := time.Now().UTC()
			makeFlowFrame := func(step int) []*data.Measurement[float64] {
				at := now.Add(time.Duration(step) * time.Second)
				mFlow := data.NewMeasurement[float64]("flow", nil)
				mFlow.Label, mFlow.At, mFlow.From = "BTC/USD", at, at
				mFlow.Metrics["level"] = data.Metric[float64]{Label: "level", Raw: 100.0}
				return []*data.Measurement[float64]{mFlow}
			}

			fragment := [][]*data.Measurement[float64]{
				makeFlowFrame(0),
				makeFlowFrame(1),
				makeFlowFrame(2),
			}

			evaluator.SetAnchorIndex(0)
			evaluator.SetExtremumIndex(1)

			// Holding entered at index 0
			// Exiting at frame 1 (peak) captures the move
			exitOutcome, err := evaluator.EvaluateExit(fragment, 1, 0)
			So(err, ShouldBeNil)
			So(exitOutcome.Correctness, ShouldBeGreaterThan, 0)
			So(exitOutcome.Reinforcement, ShouldBeGreaterThan, 0)

			// Waiting at frame 1 into frame 2 suffers dump and is punished
			waitOutcome, err := evaluator.EvaluateWait(fragment, 1, true, 0)
			So(err, ShouldBeNil)
			So(waitOutcome.Correctness, ShouldBeLessThan, 0)
			So(waitOutcome.Reinforcement, ShouldBeLessThan, 0)
		})

		Convey("20. Rehearsal frame schema purity: frames contain zero synthetic price measurements and mirror live envelopes", func() {
			series := make([]hindsight.Observation, 0, 400)
			now := time.Now().UTC()
			seq := uint64(0)
			addRamp := func(startPrice, endPrice float64, steps int) {
				for idx := 0; idx < steps; idx++ {
					fraction := float64(idx) / float64(steps)
					priceVal := startPrice + (endPrice-startPrice)*fraction
					at := now.Add(time.Duration(seq) * time.Second)
					series = append(series, hindsight.Observation{
						Capture:    hindsight.CaptureIdentity{Run: "schema-test", Sequence: types.CaptureSequence(seq)},
						Ordinal:    1,
						ReceivedAt: at,
						VenueAt:    at,
						Symbol:     "BTC/USD",
						HasBid:     true,
						Bid:        priceVal - 0.05,
						HasAsk:     true,
						Ask:        priceVal + 0.05,
						HasLast:    true,
						Last:       priceVal,
					})
					seq++
				}
			}
			addRamp(100, 100, 100)
			addRamp(100, 120, 100)
			addRamp(120, 100, 100)
			addRamp(100, 105, 100)

			catalog := tablestest.New(t)
			writer := tables.NewWriter(catalog)

			for _, obs := range series {
				identity := tables.EnvelopeRefRow{
					Run:      "schema-test",
					Sequence: int64(obs.Capture.Sequence),
					Ordinal:  1,
				}
				env := &types.Envelope{Key: "BTC/USD"}
				measurement := data.NewMeasurement[float64]("cvd", nil)
				measurement.Label, measurement.At, measurement.From = "BTC/USD", obs.At(), obs.At()
				measurement.Metrics["level"] = data.Metric[float64]{Label: "level", Raw: obs.Bid}
				env.CVD = measurement
				writer.AddWitness(tables.WitnessRow{
					Run: identity.Run, Envelope: identity, ArtifactKind: "precursor",
					Boundary: "after-logic", Payload: env.EncodePrecursor(),
				})
			}
			So(writer.Commit(t.Context()), ShouldBeNil)

			tape := hindsight.Query(hindsight.Excursions, catalog, "schema-test", hindsight.DefaultDiscoveryPolicy())
			fragments := tape.ReplayFragmentsFrom(series)
			So(len(fragments), ShouldBeGreaterThan, 0)

			for _, frag := range fragments {
				// Invariant: ZERO synthetic "price" measurements injected into rehearsal Frames
				for _, frame := range frag.Frames {
					for _, meas := range frame {
						So(meas.Source, ShouldNotEqual, "price")
						So(meas.Label, ShouldNotEqual, "price")
						So(meas.Source, ShouldEqual, "cvd")
					}
				}
			}
		})

		Convey("21. Live vs rehearsal action selection: unseen context deterministically produces WAIT for live agent, but explores for rehearsal", func() {
			engine := cognition.NewEngine(cognition.Config{})
			now := time.Now().UTC()

			// 1. Live agent (isLive = true)
			liveAgent := NewAgent(1, true, engine, 16)
			formAgentSpace(liveAgent, now)

			meas := data.NewMeasurement[float64]("flow", nil)
			meas.Label, meas.At, meas.From = "BTC/USD", now, now
			meas.Metrics["level"] = data.Metric[float64]{Label: "level", Raw: 42.0}
			impulse, err := liveAgent.Step([]*data.Measurement[float64]{meas}, "BTC/USD")
			So(err, ShouldBeNil)

			// In holding state with empty/unseen cognition:
			// Live agent MUST deterministically produce ActionWait 100% of the time (never random liquidation)
			for idx := 0; idx < 50; idx++ {
				decision, chooseErr := liveAgent.ChooseAction(impulse, true)
				So(chooseErr, ShouldBeNil)
				So(decision.Action, ShouldEqual, ActionWait)
				So(decision.Support, ShouldEqual, 0)
			}

			// 2. Rehearsal worker (isLive = false with RNG)
			rng := rand.New(rand.NewSource(99999))
			rehearsalAgent := NewAgent(2, false, engine, 16, rng)
			formAgentSpace(rehearsalAgent, now)

			rehearsalImpulse, err := rehearsalAgent.Step([]*data.Measurement[float64]{meas}, "BTC/USD")
			So(err, ShouldBeNil)

			exitCount := 0
			waitCount := 0
			for idx := 0; idx < 100; idx++ {
				decision, chooseErr := rehearsalAgent.ChooseAction(rehearsalImpulse, true)
				So(chooseErr, ShouldBeNil)
				So(decision.Support, ShouldEqual, 0)

				if decision.Action == ActionExit {
					exitCount++
				}

				if decision.Action == ActionWait {
					waitCount++
				}
			}

			// Rehearsal explores both legal actions when holding on unseen context
			So(exitCount, ShouldBeGreaterThan, 0)
			So(waitCount, ShouldBeGreaterThan, 0)
			So(exitCount+waitCount, ShouldEqual, 100)
		})

		Convey("22. MainAgent exitLong executes liquidation and updates paper economics", func() {
			initialCash := decimal.NewFromInt64(1000)
			engine := cognition.NewEngine(cognition.Config{})
			priceSvc, _ := testExecutablePrice()
			mainAgent := NewMainAgent(initialCash, "paper", nil, priceSvc, engine)

			// Setup an open position: 10 BTC
			posQty := decimal.NewFromInt64(10)
			entryPrice := decimal.NewFromInt64(50000)
			mainAgent.positions["BTC/USD"] = &types.Holding{
				Symbol:     "BTC/USD",
				Qty:        posQty,
				EntryPrice: entryPrice,
				PnL:        decimal.NewFromInt64(0),
			}
			mainAgent.posQuantities["BTC/USD"] = posQty
			mainAgent.posCosts["BTC/USD"] = decimal.NewFromInt64(500000)

			now := time.Now().UTC()
			env := &types.Envelope{
				Key: "BTC/USD",
				TickerData: kraken.TickerData{
					Last: entryPrice,
					Bid:  decimal.NewFromInt64(55000),
					Ask:  decimal.NewFromInt64(55010),
				},
			}

			// Execute exit
			mainAgent.exitLong(env, "BTC/USD", decimal.NewFromInt64(55000), now)

			// Position is successfully liquidated
			So(mainAgent.fills, ShouldEqual, 1)
			So(mainAgent.positions["BTC/USD"], ShouldBeNil)
			So(mainAgent.posQuantities["BTC/USD"], ShouldBeNil)
			So(mainAgent.cash.Cmp(initialCash), ShouldBeGreaterThan, 0)
		})
	})
}
