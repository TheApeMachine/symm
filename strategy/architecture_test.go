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
		measurement := data.NewMeasurement[float64]("m", "BTC/USD", "flow", at, at)
		measurement.Metadata = map[string]float64{data.MetadataSupport: float64(idx + 1), data.MetadataMahalanobisSNR: 100}
		first := float64(idx%2)*2 - 1
		measurement.PutMetric(data.Metric[float64]{Label: "raw", Raw: first})
		measurement.PutMetric(data.Metric[float64]{Label: "level", Raw: first})
		_ = targetAgent.Space().Step([]*data.Measurement[float64]{measurement})
		impulse, _ := targetAgent.Space().Impulse("BTC/USD", at, at)
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
				mPrice := data.NewMeasurement[float64]("p", "BTC/USD", "price", now, now)
				mPrice.PutMetric(data.Metric[float64]{Label: "price", Raw: priceValue})
				mFlow := data.NewMeasurement[float64]("m", "BTC/USD", source, now, now)
				mFlow.PutMetric(data.Metric[float64]{Label: "raw", Raw: priceValue})
				mFlow.PutMetric(data.Metric[float64]{Label: "level", Raw: priceValue})
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
				Frames:      framesRally,
				Symbol:      "BTC/USD",
				AnchorIndex: 5,
			}
			fragCrash := types.ReplayFragment{
				Frames:      framesCrash,
				Symbol:      "BTC/USD",
				AnchorIndex: 5,
			}

			formSpace := func(targetAgent *Agent) {
				for idx := 0; idx < 128; idx++ {
					at := now.Add(time.Duration(idx) * time.Second)
					m := data.NewMeasurement[float64]("m", "BTC/USD", "flow", at, now)
					m.Metadata = map[string]float64{data.MetadataSupport: float64(idx + 1), data.MetadataMahalanobisSNR: 100}
					first := float64(idx%2)*2 - 1
					m.PutMetric(data.Metric[float64]{Label: "raw", Raw: first})
					m.PutMetric(data.Metric[float64]{Label: "level", Raw: first})
					_ = targetAgent.Space().Step([]*data.Measurement[float64]{m})
					imp, _ := targetAgent.Space().Impulse("BTC/USD", at, now)
					if imp.Ready {
						break
					}
				}
			}

			engine1 := cognition.NewEngine(cognition.DefaultConfig())
			agent1 := NewAgent(1, false, engine1, 16, rand.New(rand.NewSource(seed)))
			agent1.SetFeeRate(0.001)
			formSpace(agent1)

			engine2 := cognition.NewEngine(cognition.DefaultConfig())
			agent2 := NewAgent(2, false, engine2, 16, rand.New(rand.NewSource(seed)))
			agent2.SetFeeRate(0.001)
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
			ctx1.Sequence(grid.Impulse{Ready: true, Regions: []grid.Region{r1, r2}, Version: 1, At: now, From: now})
			seq1 := ctx1.Sequence(grid.Impulse{Ready: true, Regions: []grid.Region{r3}, Version: 2, At: now, From: now})

			// Sequence 2: [r1] at step 1, [r2, r3] at step 2
			ctx2.Sequence(grid.Impulse{Ready: true, Regions: []grid.Region{r1}, Version: 1, At: now, From: now})
			seq2 := ctx2.Sequence(grid.Impulse{Ready: true, Regions: []grid.Region{r2, r3}, Version: 2, At: now, From: now})

			So(seq1, ShouldNotResemble, seq2)

			engine := cognition.NewEngine(cognition.DefaultConfig())
			engine.Observe(seq1, []byte("ENTER"), 1.0)

			eval1 := engine.Evaluate(seq1)
			eval2 := engine.Evaluate(seq2)

			So(eval1.WinnerClass, ShouldEqual, "ENTER")
			So(eval2.WinnerClass, ShouldNotEqual, "ENTER")
		})

		Convey("3. Genuine price enforcement: evaluator strictly rejects flow/CVD metrics without price", func() {
			rate := 0.001
			evaluator := NewFragmentEvaluator(&rate)
			now := time.Now().UTC()

			m1 := data.NewMeasurement[float64]("m1", "BTC/USD", "flow", now, now)
			m1.PutMetric(data.Metric[float64]{Label: "raw", Raw: 100.0})
			m1.PutMetric(data.Metric[float64]{Label: "level", Raw: 100.0})

			m2 := data.NewMeasurement[float64]("m2", "BTC/USD", "flow", now, now)
			m2.PutMetric(data.Metric[float64]{Label: "raw", Raw: 120.0})
			m2.PutMetric(data.Metric[float64]{Label: "level", Raw: 120.0})

			fragment := [][]*data.Measurement[float64]{{m1}, {m2}}

			_, err := evaluator.EvaluateEntry(fragment, 0)
			So(err, ShouldNotBeNil)
		})

		Convey("4. Canonical friction from broker.Price: identical raw move clears on low-fee symbol and fails on high-fee symbol", func() {
			instrument := broker.NewInstrumentWithQuote("USD")
			price := broker.NewPrice(nil, instrument)

			// BTC/USD: 0.1% fee (round-trip 0.2%)
			price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.1)})
			// HIGH/USD: 2.0% fee (round-trip 4.0%)
			price.SetFee("HIGH/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(2.0)})

			evaluator := NewFragmentEvaluator(nil, price)
			now := time.Now().UTC()

			makePriceFrame := func(symbol string, priceValue float64) []*data.Measurement[float64] {
				m := data.NewMeasurement[float64]("p", symbol, "price", now, now)
				m.PutMetric(data.Metric[float64]{Label: "price", Raw: priceValue})
				return []*data.Measurement[float64]{m}
			}

			// A 1.0% raw gain ($100 -> $101)
			fragA := [][]*data.Measurement[float64]{makePriceFrame("BTC/USD", 100.0), makePriceFrame("BTC/USD", 101.0)}
			fragB := [][]*data.Measurement[float64]{makePriceFrame("HIGH/USD", 100.0), makePriceFrame("HIGH/USD", 101.0)}

			outcomeA, errA := evaluator.EvaluateEntry(fragA, 0)
			So(errA, ShouldBeNil)
			So(outcomeA.Correctness, ShouldBeGreaterThan, 0)

			outcomeB, errB := evaluator.EvaluateEntry(fragB, 0)
			So(errB, ShouldBeNil)
			So(outcomeB.Correctness, ShouldEqual, -1.0)
		})

		Convey("5. Pump-exit protection: identical chart price with evaporating bids triggers exit while deep bids triggers wait", func() {
			evaluator := NewFragmentEvaluator(nil)
			rate := 0.001
			evaluator.SetFeeRate(rate)

			now := time.Now().UTC()
			makePriceFrame := func(priceValue float64) []*data.Measurement[float64] {
				m := data.NewMeasurement[float64]("p", "BTC/USD", "price", now, now)
				m.PutMetric(data.Metric[float64]{Label: "price", Raw: priceValue})
				return []*data.Measurement[float64]{m}
			}

			frames := [][]*data.Measurement[float64]{
				makePriceFrame(100.0),
				makePriceFrame(105.0),
			}

			// Case A: Deep bids persist ($ExecutableValue rises from 100 to 105)
			surfA := []*types.ExecutionSurface{
				{ExecutableValue: decimal.NewFromInt64(100)},
				{ExecutableValue: decimal.NewFromInt64(105)},
			}
			evaluator.SetSurfaces(surfA)
			outcomeA, errA := evaluator.EvaluateExit(frames, 0, 0)
			So(errA, ShouldBeNil)
			So(outcomeA.Correctness, ShouldBeLessThan, 0) // Premature exit

			// Case B: Price prints 105, but bids evaporate ($ExecutableValue crashes to 85)
			surfB := []*types.ExecutionSurface{
				{ExecutableValue: decimal.NewFromInt64(100)},
				{ExecutableValue: decimal.NewFromInt64(85)},
			}
			evaluator.SetSurfaces(surfB)
			outcomeB, errB := evaluator.EvaluateExit(frames, 0, 0)
			So(errB, ShouldBeNil)
			So(outcomeB.Correctness, ShouldBeGreaterThan, 0) // Correct exit protecting liquidation proceeds
		})

		Convey("6. Behavioral action learning: held-out contexts separate ENTER vs WAIT after rehearsal", func() {
			engine := cognition.NewEngine(cognition.DefaultConfig())
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
				seqBull = ctxBull.Sequence(imp)
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
				seqBear = ctxBear.Sequence(imp)
			}
			So(len(seqBear), ShouldBeGreaterThan, 0)
			So(bytes.Equal(seqBull, seqBear), ShouldBeFalse)

			// Train repeatedly:
			// seqBull represents a precursor leading to profitable rally -> reinforces ENTER, inhibits WAIT
			// seqBear represents a precursor leading to crash -> inhibits ENTER, reinforces WAIT
			for r := 0; r < 5; r++ {
				engine.Observe(seqBull, []byte(ActionEnter), 0.85)
				engine.Observe(seqBull, []byte(ActionWait), -0.85)

				engine.Observe(seqBear, []byte(ActionEnter), -0.85)
				engine.Observe(seqBear, []byte(ActionWait), 0.85)
			}

			// Evaluate held-out contexts on cognition engine
			evalBull := engine.Evaluate(seqBull)
			So(evalBull.Support, ShouldBeGreaterThan, 0)
			So(evalBull.WinnerClass, ShouldEqual, string(ActionEnter))

			evalBear := engine.Evaluate(seqBear)
			So(evalBear.Support, ShouldBeGreaterThan, 0)
			So(evalBear.WinnerClass, ShouldEqual, string(ActionWait))

			// Prove agent policy chooses ENTER on bullish context and WAIT on bearish context
			actionBull, shareBull, contrastBull, suppBull := selectLegalAction(
				[]Action{ActionEnter, ActionWait},
				evalBull.Candidates,
				rng,
			)
			So(actionBull, ShouldEqual, ActionEnter)
			So(shareBull, ShouldBeGreaterThan, 0.5)
			So(contrastBull, ShouldBeGreaterThan, 0)
			So(suppBull, ShouldBeGreaterThan, 0)

			actionBear, shareBear, contrastBear, suppBear := selectLegalAction(
				[]Action{ActionEnter, ActionWait},
				evalBear.Candidates,
				rng,
			)
			So(actionBear, ShouldEqual, ActionWait)
			So(shareBear, ShouldBeGreaterThan, 0.5)
			So(contrastBear, ShouldBeGreaterThan, 0)
			So(suppBear, ShouldBeGreaterThan, 0)
		})

		Convey("7. Missing economics prevents action: MainAgent takes zero economic action when fees are absent", func() {
			engine := cognition.NewEngine(cognition.DefaultConfig())
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

		Convey("8. Economic timing: fraction of feasible excursion move captured relative to best feasible entry", func() {
			rate := 0.001
			evaluator := NewFragmentEvaluator(&rate)

			now := time.Now().UTC()
			makeFrame := func(priceValue float64) []*data.Measurement[float64] {
				m := data.NewMeasurement[float64]("p", "BTC/USD", "price", now, now)
				m.PutMetric(data.Metric[float64]{Label: "price", Raw: priceValue})
				return []*data.Measurement[float64]{m}
			}

			frames := [][]*data.Measurement[float64]{
				makeFrame(100.0), // 0: price 100
				makeFrame(98.0),  // 1: optimal entry at 98
				makeFrame(105.0), // 2: late entry at 105
				makeFrame(120.0), // 3: peak
			}

			// Entry at frame 1 (price 98): captures the entire feasible move
			optimalOutcome, err1 := evaluator.EvaluateEntry(frames, 1)
			So(err1, ShouldBeNil)
			So(optimalOutcome.Timing, ShouldEqual, 1.0)
			So(optimalOutcome.Reinforcement, ShouldBeGreaterThan, 0)

			// Entry at frame 2 (price 105): captures much less of the feasible move
			lateOutcome, err2 := evaluator.EvaluateEntry(frames, 2)
			So(err2, ShouldBeNil)
			So(lateOutcome.Timing, ShouldBeLessThan, optimalOutcome.Timing)
		})

		Convey("9. Randomized A is strictly constrained to A < B and agent.Reset clears perception", func() {
			engine := cognition.NewEngine(cognition.DefaultConfig())
			seed := int64(777)
			rng := rand.New(rand.NewSource(seed))
			agent := NewAgent(2, false, engine, 16, rng)
			agent.SetFeeRate(0.001)

			anchorB := 6
			now := time.Now().UTC()
			makeFrame := func(priceValue float64) []*data.Measurement[float64] {
				mPrice := data.NewMeasurement[float64]("p", "BTC/USD", "price", now, now)
				mPrice.PutMetric(data.Metric[float64]{Label: "price", Raw: priceValue})
				mFlow := data.NewMeasurement[float64]("m", "BTC/USD", "flow", now, now)
				mFlow.PutMetric(data.Metric[float64]{Label: "raw", Raw: priceValue})
				mFlow.PutMetric(data.Metric[float64]{Label: "level", Raw: priceValue})
				return []*data.Measurement[float64]{mPrice, mFlow}
			}

			frames := make([][]*data.Measurement[float64], 12)
			for frameIdx := range frames {
				frames[frameIdx] = makeFrame(float64(100 + frameIdx))
			}

			fragment := types.ReplayFragment{
				Frames:      frames,
				Symbol:      "BTC/USD",
				AnchorIndex: anchorB,
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
			engine := cognition.NewEngine(cognition.DefaultConfig())
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
			engine := cognition.NewEngine(cognition.DefaultConfig())
			instrument := broker.NewInstrumentWithQuote("USD")
			price := broker.NewPrice(nil, instrument)
			price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
			mainAgent := NewMainAgent(decimal.NewFromInt64(1000), "paper", instrument, price, engine)

			now := time.Now().UTC()
			m := data.NewMeasurement[float64]("p", "BTC/USD", "price", now, now)
			m.PutMetric(data.Metric[float64]{Label: "price", Raw: 50000.0})

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

			eval := engine.Evaluate([]byte("trade_context_test"))
			So(eval.Support, ShouldEqual, 0)
			So(engine.Root().Len(), ShouldEqual, 0)
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
				measurement := data.NewMeasurement[float64]("flow", "BTC/USD", "cvd", obs.At(), obs.At())
				measurement.PutMetric(data.Metric[float64]{Label: "level", Raw: obs.Bid})
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

			// Real fragment carries execution surfaces and price measurements with ZERO manual injection
			frag := fragments[0]
			So(len(frag.Surfaces), ShouldBeGreaterThan, 0)
			So(frag.Surfaces[0].BestBid, ShouldNotBeNil)
			So(frag.Surfaces[0].BestAsk, ShouldNotBeNil)
			So(frag.Surfaces[0].ExecutableValue, ShouldNotBeNil)

			engine := cognition.NewEngine(cognition.DefaultConfig())
			agent := NewAgent(10, false, engine, 16, rand.New(rand.NewSource(12345)))
			agent.SetFeeRate(0.001)
			formAgentSpace(agent, now)

			agent.IngestReplay(frag, 0)
			stepped, err := agent.RehearseChild()
			So(err, ShouldBeNil)
			So(stepped, ShouldBeGreaterThan, 0)

			marks := agent.LastMarks()
			So(len(marks), ShouldBeGreaterThan, 0)
			So(engine.Root().Len(), ShouldBeGreaterThan, 0)
		})

		Convey("14. Pump-exit timing criterion: WAIT at 100 > EXIT at 100, EXIT at 115/120 > WAIT there, WAIT into 80 strongly punished", func() {
			feeRate := 0.001
			evaluator := NewFragmentEvaluator(&feeRate)

			now := time.Now().UTC()
			makeFrame := func(priceValue float64) []*data.Measurement[float64] {
				measurement := data.NewMeasurement[float64]("p", "BTC/USD", "price", now, now)
				measurement.PutMetric(data.Metric[float64]{Label: "price", Raw: priceValue})
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
				measurement := data.NewMeasurement[float64]("p", "BTC/USD", "price", now, now)
				measurement.PutMetric(data.Metric[float64]{Label: "price", Raw: priceValue})
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

		Convey("16. Canonical entry book economics: identical chart move evaluates profitable on deep book and loss on thin/wide book", func() {
			feeRate := 0.001
			evalDeep := NewFragmentEvaluator(&feeRate)
			evalThin := NewFragmentEvaluator(&feeRate)

			now := time.Now().UTC()
			makeFrame := func(priceValue float64) []*data.Measurement[float64] {
				measurement := data.NewMeasurement[float64]("p", "BTC/USD", "price", now, now)
				measurement.PutMetric(data.Metric[float64]{Label: "price", Raw: priceValue})
				return []*data.Measurement[float64]{measurement}
			}

			chartFrames := [][]*data.Measurement[float64]{
				makeFrame(100.0),
				makeFrame(120.0),
			}

			// Deep book: tight spread (ask 100.10 at entry, bid 119.90 at peak)
			evalDeep.SetSurfaces([]*types.ExecutionSurface{
				{BestAsk: decimal.NewFromFloat64(100.10), BestBid: decimal.NewFromFloat64(100.00)},
				{BestAsk: decimal.NewFromFloat64(120.00), BestBid: decimal.NewFromFloat64(119.90)},
			})

			// Thin/wide book: wide spread (ask 115.00 at entry, bid 105.00 at peak)
			evalThin.SetSurfaces([]*types.ExecutionSurface{
				{BestAsk: decimal.NewFromFloat64(115.00), BestBid: decimal.NewFromFloat64(95.00)},
				{BestAsk: decimal.NewFromFloat64(125.00), BestBid: decimal.NewFromFloat64(105.00)},
			})

			outcomeDeep, err := evalDeep.EvaluateEntry(chartFrames, 0)
			So(err, ShouldBeNil)
			So(outcomeDeep.Correctness, ShouldBeGreaterThan, 0)

			outcomeThin, err := evalThin.EvaluateEntry(chartFrames, 0)
			So(err, ShouldBeNil)
			So(outcomeThin.Correctness, ShouldEqual, -1.0)
		})

		Convey("17. Behavioral learning via rehearsal: ReplayFragments train cognition via RehearseChild with zero manual Observe", func() {
			engine := cognition.NewEngine(cognition.DefaultConfig())
			seed := int64(1337)
			agent := NewAgent(5, false, engine, 16, rand.New(rand.NewSource(seed)))
			agent.SetFeeRate(0.001)
			formAgentSpace(agent, time.Now().UTC())

			now := time.Now().UTC()
			makeFlowFrame := func(sigValue float64, step int) []*data.Measurement[float64] {
				at := now.Add(time.Duration(step) * time.Second)
				mFlow := data.NewMeasurement[float64]("m", "BTC/USD", "flow", at, at)
				mFlow.PutMetric(data.Metric[float64]{Label: "raw", Raw: sigValue})
				mFlow.PutMetric(data.Metric[float64]{Label: "level", Raw: sigValue})
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
				Surfaces:    surfaces,
				Symbol:      "BTC/USD",
				AnchorIndex: 4,
			}

			for iter := 0; iter < 10; iter++ {
				agent.IngestReplay(bullFrag, iter)
				stepped, err := agent.RehearseChild()
				So(err, ShouldBeNil)
				So(stepped, ShouldBeGreaterThan, 0)
			}

			So(engine.Root().Len(), ShouldBeGreaterThan, 0)

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
				act, _, _, _, supp := evalAgent.ChooseAction(imp, holding)
				chosenActions = append(chosenActions, act)
				supports = append(supports, supp)
				if act == ActionEnter {
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
			engine := cognition.NewEngine(cognition.DefaultConfig())
			seed := int64(42)
			agent := NewAgent(8, false, engine, 16, rand.New(rand.NewSource(seed)))
			formAgentSpace(agent, time.Now().UTC())

			now := time.Now().UTC()
			ctxLearned := associative.NewContext()
			var learnedSeq []byte

			for idx := 0; idx < 12; idx++ {
				at := now.Add(time.Duration(idx) * time.Second)
				meas := data.NewMeasurement[float64]("m", "BTC/USD", "flow", at, now)
				val := float64((idx % 3) + 1)
				meas.PutMetric(data.Metric[float64]{Label: "raw", Raw: val})
				meas.PutMetric(data.Metric[float64]{Label: "level", Raw: val})
				_ = agent.Space().Step([]*data.Measurement[float64]{meas})
				imp, _ := agent.Space().Impulse("BTC/USD", at, now)
				learnedSeq = ctxLearned.Sequence(imp)
			}
			So(len(learnedSeq), ShouldBeGreaterThan, 0)

			engine.Observe(learnedSeq, []byte("ENTER"), 1.0)

			ctxLive := associative.NewContext()
			var liveSeq []byte
			for idx := 0; idx < 100; idx++ {
				at := now.Add(time.Duration(100+idx) * time.Second)
				meas := data.NewMeasurement[float64]("m", "BTC/USD", "flow", at, now)
				val := float64(-1.0)
				if idx >= 88 {
					val = float64(((idx - 88) % 3) + 1)
				}
				meas.PutMetric(data.Metric[float64]{Label: "raw", Raw: val})
				meas.PutMetric(data.Metric[float64]{Label: "level", Raw: val})
				_ = agent.Space().Step([]*data.Measurement[float64]{meas})
				imp, _ := agent.Space().Impulse("BTC/USD", at, now)
				liveSeq = ctxLive.Sequence(imp)
			}

			eval := engine.Evaluate(liveSeq)
			So(eval.Support, ShouldBeGreaterThan, 0)
			So(eval.WinnerClass, ShouldEqual, "ENTER")
		})
	})
}
