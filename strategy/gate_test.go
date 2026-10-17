package strategy

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"math/rand/v2"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/logic/resonance"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
withGate sets the shipped config's gate for one test and returns its undo.
*/
func withGate(on bool, z float64) func() {
	saved := *system.Cfg.Resonance
	system.Cfg.Resonance.Gate, system.Cfg.Resonance.SurpriseZ = on, z

	return func() { *system.Cfg.Resonance = saved }
}

/*
featureTicks are stored-shape ticks whose symbols carry every pinned solver
Feature, one signal layer per source as the signals write them, moving
differently each tick so chords change and the solver is surprised.
*/
func featureTicks(count int, symbols ...string) []*data.Value {
	ticks := make([]*data.Value, 0, count)

	for tick := uint64(1); tick <= uint64(count); tick++ {
		metrics := make(map[string]map[string]float64, len(resonance.Features))

		for idx, feature := range resonance.Features {
			period := uint64(3 + idx)
			metrics[feature.Source] = map[string]float64{feature.Label: float64(1 + (tick*uint64(7+idx))%period)}
		}

		ticks = append(ticks, gridTick(tick, metrics, symbols...))
	}

	return ticks
}

func TestSettling(t *testing.T) {
	Convey("Given the settled warm-up at the shipped rates", t, func() {
		fast, slow := resonance.Rates(system.Cfg.Resonance.LearningRate)
		history := int(math.Ceil(1 / slow))
		noise := rand.New(rand.NewPCG(1, 2))

		// feed observes errors and returns the step (1-based) the symbol
		// warmed at, 0 when it never did, and the cold reasons seen.
		feed := func(errors []float64) (int, map[string]int) {
			settle := &settling{fastRate: fast, slowRate: slow}
			opened := 0
			reasons := make(map[string]int)

			for idx, err := range errors {
				warm, reason := settle.observe(err)

				if warm && opened == 0 {
					opened = idx + 1
				}

				if !warm {
					So(opened, ShouldEqual, 0) // once warm, warm for good
					So(reason, ShouldBeIn, []string{coldSilent, coldHistory, coldFalling})
					reasons[reason]++
				} else {
					So(reason, ShouldBeEmpty)
				}
			}

			return opened, reasons
		}

		Convey("An error that decays and then plateaus opens the gate on the plateau", func() {
			const plateau, length = 200, 2000
			errors := make([]float64, 0, length)

			for idx := range length {
				level := 1 + 4*math.Exp(-float64(min(idx, plateau))/40)
				errors = append(errors, level*(1+0.1*noise.NormFloat64()))
			}

			opened, reasons := feed(errors)
			So(opened, ShouldBeGreaterThan, plateau)
			So(opened, ShouldBeLessThan, length)
			So(reasons[coldHistory], ShouldEqual, history-1)
			So(reasons[coldFalling], ShouldEqual, opened-history)
		})

		Convey("An error that never stops falling keeps the gate closed", func() {
			// A noiseless 1/t decay, and a steady exponential decay whose fall
			// per slow time constant (~22%) dwarfs its 1% noise.
			decaying := make([]float64, 0, 3000)
			noisy := make([]float64, 0, 3000)

			for idx := range 3000 {
				decaying = append(decaying, 5/(1+float64(idx)/50))
				noisy = append(noisy, 5*math.Exp(-float64(idx)/400)*(1+0.01*noise.NormFloat64()))
			}

			for _, errors := range [][]float64{decaying, noisy} {
				opened, reasons := feed(errors)
				So(opened, ShouldEqual, 0)
				So(reasons[coldFalling], ShouldEqual, len(errors)-(history-1))
			}
		})

		Convey("A flat or rising error still waits one slow time constant", func() {
			errors := make([]float64, 0, 2*history)

			for idx := range 2 * history {
				errors = append(errors, 1+float64(idx)/10)
			}

			opened, reasons := feed(errors)
			So(opened, ShouldEqual, history)
			So(reasons[coldHistory], ShouldEqual, history-1)
		})

		Convey("Leading zero errors (no input yet) are not history", func() {
			errors := make([]float64, 0, 5+2*history)

			for range 5 {
				errors = append(errors, 0)
			}

			for idx := range 2 * history {
				errors = append(errors, 1+float64(idx)/10)
			}

			opened, reasons := feed(errors)
			So(reasons[coldSilent], ShouldEqual, 5)
			So(opened, ShouldEqual, 5+history)
		})
	})
}

func TestChordGate(t *testing.T) {
	Convey("Given the chord-level surprise gate", t, func() {
		ctx := context.Background()
		ticks := featureTicks(400, "BTC/USD")

		// run drives one tape the way both paths do (symbolTape.step).
		run := func(gate *chordGate) (*symbolTape, []tapeStep) {
			grid := nmstore.NewGrid(ctx)
			tape := newSymbolTape(gate, "BTC/USD")
			steps := make([]tapeStep, 0, len(ticks))

			for _, value := range ticks {
				symbolTicks, err := broker.ReadTick(value)
				So(err, ShouldBeNil)

				step, _, err := tape.step(grid, value, symbolTicks[0])
				So(err, ShouldBeNil)
				steps = append(steps, step)
			}

			return tape, steps
		}

		Convey("Its settings come from config, validated, without defaults", func() {
			cfg := *system.Cfg.Resonance
			cfg.LearningRate = 0.2 // faster than the coders' fastest rate
			_, err := newChordGate(ctx, &cfg)
			So(err, ShouldNotBeNil)

			cfg = *system.Cfg.Resonance
			cfg.LearningRate = 0
			_, err = newChordGate(ctx, &cfg)
			So(err, ShouldNotBeNil)

			cfg = *system.Cfg.Resonance
			cfg.SurpriseZ = math.NaN()
			_, err = newChordGate(ctx, &cfg)
			So(err, ShouldNotBeNil)
		})

		Convey("Nothing passes while cold; once warm every chord with a z reaching surprise_z does", func() {
			defer withGate(true, -1e9)()
			gate, err := newChordGate(ctx, system.Cfg.Resonance)
			So(err, ShouldBeNil)

			tape, steps := run(gate)
			appended, warmAt := 0, -1

			for idx, step := range steps {
				So(step.Gate, ShouldNotBeNil)

				if step.Gate.Warm && warmAt < 0 {
					warmAt = idx
				}

				So(step.Gate.Warm, ShouldEqual, warmAt >= 0)

				if !step.Gate.Warm {
					So(step.Gate.Cold, ShouldNotBeEmpty)
					So(step.Gate.Z, ShouldBeNil)
					So(step.Gate.Passed, ShouldBeFalse)
					So(step.Path, ShouldBeEmpty)
					continue
				}

				// z accumulates only once warm: none on the first two warm steps.
				So(step.Gate.Z == nil, ShouldEqual, idx < warmAt+2)
				So(step.Gate.Passed, ShouldEqual, idx >= warmAt+2)

				if step.Path != "" {
					appended++
				}
			}

			So(warmAt, ShouldBeGreaterThan, 0)
			So(appended, ShouldBeGreaterThan, 0)
			So(tape.grammar.StartRule().Terminals(), ShouldNotBeEmpty)
		})

		Convey("A surprise below surprise_z blocks the chord: nothing is appended", func() {
			defer withGate(true, 1e9)()
			gate, err := newChordGate(ctx, system.Cfg.Resonance)
			So(err, ShouldBeNil)

			tape, steps := run(gate)
			blocked := 0

			for _, step := range steps {
				So(step.Gate.Passed, ShouldBeFalse)
				So(step.Path, ShouldBeEmpty)

				if step.Blocked {
					blocked++
				}
			}

			So(blocked, ShouldBeGreaterThan, 0)
			So(tape.grammar.StartRule().Terminals(), ShouldBeEmpty)
		})

		Convey("The z-score is causal: taken against the warm surprises before it", func() {
			defer withGate(true, 0)()
			gate, err := newChordGate(ctx, system.Cfg.Resonance)
			So(err, ShouldBeNil)

			_, steps := run(gate)
			surprises := make([]float64, 0, len(steps))

			for _, step := range steps {
				if !step.Gate.Warm {
					continue
				}

				if len(surprises) >= 2 {
					var mean, m2 float64

					for _, prior := range surprises {
						mean += prior
					}

					mean /= float64(len(surprises))

					for _, prior := range surprises {
						m2 += (prior - mean) * (prior - mean)
					}

					deviation := math.Sqrt(m2 / float64(len(surprises)-1))

					So(deviation, ShouldBeGreaterThan, 0)
					So(step.Gate.Z, ShouldNotBeNil)
					So(*step.Gate.Z, ShouldAlmostEqual, (step.Gate.Surprise-mean)/deviation, 1e-9)
					So(step.Gate.Passed, ShouldEqual, *step.Gate.Z >= 0)
				} else {
					So(step.Gate.Z, ShouldBeNil)
				}

				surprises = append(surprises, step.Gate.Surprise)
			}
		})

		Convey("A blocked chord leaves the tape as it was, so the same chord can pass later", func() {
			regions := []*nmstore.Region{{Key: 1, Brightness: 2}, {Key: 4, Brightness: 3}}
			tape := newSymbolTape(nil, "BTC/USD")

			step, ok := tape.advance(regions, &GateReading{Passed: false})
			So(ok, ShouldBeTrue)
			So(step.Blocked, ShouldBeTrue)
			So(step.Fired, ShouldResemble, []string{"R04"})
			So(tape.lastSym, ShouldBeEmpty)
			So(tape.grammar.StartRule().Terminals(), ShouldBeEmpty)

			step, ok = tape.advance(regions, &GateReading{Passed: true})
			So(ok, ShouldBeTrue)
			So(step.Blocked, ShouldBeFalse)
			So(step.Path, ShouldEqual, "R04")

			step, _ = tape.advance(regions, nil)
			So(step.Repeat, ShouldBeTrue)
		})

		Convey("Train and run advance identical tapes from the same ticks", func() {
			defer withGate(true, 0)()
			types.SetFocus("BTC/USD")
			defer types.SetFocus("BTC/USD")
			ticks := featureTicks(400, "BTC/USD", "ETH/USD")

			trainGate, err := newChordGate(ctx, system.Cfg.Resonance)
			So(err, ShouldBeNil)
			replay := newTapeReplay(ctx, trainGate, nil)

			tree, err := newCognitionTree(&policy{actions: map[string]string{}}, 2)
			So(err, ShouldBeNil)
			sink := &frameSink{}
			sequitur, err := NewSequitur(ctx, sink, tree)
			So(err, ShouldBeNil)

			for _, value := range ticks {
				So(replay.observe(value), ShouldBeNil)
				_, err := io.Copy(sequitur, value)
				So(err, ShouldBeNil)
			}

			So(replay.order, ShouldResemble, []string{"BTC/USD", "ETH/USD"})

			for _, symbol := range replay.order {
				So(sequitur.tapes[symbol].grammar.StartRule().TerminalNames(sequitur.tapes[symbol].grammar),
					ShouldResemble, replay.tapes[symbol].grammar.StartRule().TerminalNames(replay.tapes[symbol].grammar))
				So(sequitur.tapes[symbol].surprise, ShouldResemble, replay.tapes[symbol].surprise)
			}

			// The focused symbol's appended chords, tick by tick.
			trained := make([]frameRecord, 0)

			for _, frame := range replay.tapes["BTC/USD"].frames {
				if frame.path != "" {
					trained = append(trained, frame)
				}
			}

			live := make([]frameRecord, 0)
			blocked := 0

			for _, value := range sink.values {
				var frame SequiturFrame
				So(json.Unmarshal(*data.Layer[[]byte](value, 0), &frame), ShouldBeNil)
				So(frame.Gate, ShouldNotBeNil)

				if frame.Blocked {
					blocked++
					So(frame.Gate.Passed, ShouldBeFalse)
					So(frame.Events, ShouldBeEmpty)
					So(frame.Policy.Stored, ShouldBeFalse)
					continue
				}

				So(frame.Gate.Passed, ShouldBeTrue)
				live = append(live, frameRecord{tick: frame.Tick, path: frame.Path})
			}

			So(blocked, ShouldBeGreaterThan, 0)
			So(live, ShouldNotBeEmpty)
			So(live, ShouldResemble, trained)
		})

		Convey("Run refuses a tree trained under another gate", func() {
			defer withGate(true, 0)()
			other := gateSettings(system.Cfg.Resonance)
			other.Enabled = false
			tree, err := newCognitionTree(&policy{epoch: 9, gate: other, actions: map[string]string{"R01/R04": "enter"}}, 2)
			So(err, ShouldBeNil)

			_, err = NewSequitur(ctx, &frameSink{}, tree)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "policy epoch 9 was trained under")
		})
	})
}
