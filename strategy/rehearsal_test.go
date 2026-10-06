package strategy

import (
	"bytes"
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestRehearsal_FragmentMarkers(t *testing.T) {
	Convey("Given a past excursion learned into a fragment", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Trades are sparser than signal frames (no trade at ticks 8, 10,
		// 11), so the sweet-spot markers must map onto real trade ticks.
		training := trainingFixture(
			t, ctx, uniformTape, detectionRowAt(excursionUp, 7, 10, 15, 60000, 63000),
			trades(map[int64]string{
				7: "60200", 9: "60100", 12: "61500", 14: "62500", 15: "63000",
			}),
		)

		Convey("It exposes A/B/C ticks and ENTER/EXIT sweet spots before B and C", func() {
			frags := training.Rehearsal.Chart.Fragments()
			So(len(frags), ShouldBeGreaterThan, 0)

			frag := frags[0]
			So(frag.MarkA, ShouldBeGreaterThan, 0)
			So(frag.MarkB, ShouldEqual, 10)
			So(frag.MarkC, ShouldEqual, 15)
			So(frag.MarkA, ShouldBeLessThan, frag.MarkB)

			So(frag.Points, ShouldHaveLength, 5)
			So(frag.EntryIdx, ShouldBeBetweenOrEqual, 0, len(frag.Points)-1)
			So(frag.ExitIdx, ShouldBeBetweenOrEqual, 0, len(frag.Points)-1)
			So(frag.ExitIdx, ShouldBeGreaterThan, frag.EntryIdx)

			enterSeq := frag.Points[frag.EntryIdx].Tick
			exitSeq := frag.Points[frag.ExitIdx].Tick

			// ENTER fills strictly before ignition B, EXIT strictly between
			// B and exhaustion C, on ticks that actually traded.
			So(enterSeq, ShouldBeGreaterThanOrEqualTo, frag.MarkA)
			So(enterSeq, ShouldBeLessThan, frag.MarkB)
			So(exitSeq, ShouldBeGreaterThan, frag.MarkB)
			So(exitSeq, ShouldBeLessThan, frag.MarkC)
			// The replayed window first moves on tick 8 (tick 7 is its first
			// observation), so ENTER fills on the first trade after it.
			So(enterSeq, ShouldEqual, 9)
			So(exitSeq, ShouldEqual, 14)

			So(frag.Direction, ShouldEqual, "up")
			So(frag.Magnitude, ShouldBeGreaterThan, 0)
			So(frag.EntryIdx, ShouldBeGreaterThanOrEqualTo, 0)
			So(frag.ExitIdx, ShouldBeGreaterThanOrEqualTo, 0)
			// Post-teach Predict: the uniform tape gives the precursor and
			// the holding run one shared token, so exit (gross feedback)
			// outweighs enter (net) on that context when both compete. Asked
			// from the stance each phase acts in, both are recalled and both
			// predicted markers sit on their ground-truth sweet spots.
			So(frag.PredictedEntryIdx, ShouldEqual, frag.EntryIdx)
			So(frag.PredictedExitIdx, ShouldEqual, frag.ExitIdx)
		})
	})
}

func TestDeduplicateTokens(t *testing.T) {
	Convey("Given a frame sequence with repeated region tokens", t, func() {
		frames := [][]byte{
			[]byte("R1"), []byte("R1"), []byte("R1"),
			[]byte("R2"),
			nil,
			[]byte("R2"), []byte(""), []byte("R2"),
			[]byte("R1"),
			[]byte("R3"), []byte("R3"),
		}

		Convey("Consecutive duplicates (also across empty frames) collapse to one transition", func() {
			So(deduplicateTokens(frames), ShouldResemble, [][]byte{
				[]byte("R1"), []byte("R2"), []byte("R1"), []byte("R3"),
			})
		})

		Convey("Holding a region longer does not change the learned context", func() {
			short := bytes.Join(deduplicateTokens([][]byte{
				[]byte("R1"), []byte("R2"), []byte("R3"),
			}), []byte("/"))
			long := bytes.Join(deduplicateTokens([][]byte{
				[]byte("R1"), []byte("R1"), []byte("R1"), []byte("R2"),
				[]byte("R2"), []byte("R3"), []byte("R3"), []byte("R3"),
			}), []byte("/"))

			So(string(long), ShouldEqual, string(short))
			So(string(short), ShouldEqual, "R1/R2/R3")
		})

		Convey("An empty or all-empty sequence yields no context", func() {
			So(deduplicateTokens(nil), ShouldBeEmpty)
			So(deduplicateTokens([][]byte{nil, []byte("")}), ShouldBeEmpty)
		})
	})
}

func TestRehearsal_LearnOffsetsA(t *testing.T) {
	Convey("Given the same excursion learned at two A offsets", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingSetup(t, ctx, detectionAt(excursionUp, 5, 10, 15, 60000, 63000), func(writer *tables.Writer, epoch int64) {
			sensor(writer, epoch, "cvd", 1, 0, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15)
		})

		detectionRow := firstDetection(ctx, training)
		So(detectionRow, ShouldNotBeNil)

		asked, _, _, err := training.Rehearsal.learn(ctx, detectionRow, 0)
		So(err, ShouldBeNil)
		So(asked, ShouldResemble, map[string]int{actionEnter: 1, actionExit: 1})

		asked, _, _, err = training.Rehearsal.learn(ctx, detectionRow, 2)
		So(err, ShouldBeNil)
		So(asked, ShouldResemble, map[string]int{actionEnter: 1, actionExit: 1})

		frags := training.Rehearsal.Chart.Fragments()
		So(len(frags), ShouldEqual, 2)
		So(frags[0].MarkA, ShouldNotEqual, frags[1].MarkA)
		So(frags[0].MarkA, ShouldBeLessThan, frags[0].MarkB)
		So(frags[1].MarkA, ShouldBeLessThan, frags[1].MarkB)
		So(census(training, "enter"), ShouldBeGreaterThan, 0)
		So(census(training, "exit"), ShouldBeGreaterThan, 0)
	})
}

func TestRehearsal_MissingTapeHalts(t *testing.T) {
	Convey("Given a stored detection whose run has no signal/logic tape", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		Convey("learn errors instead of reporting nothing to learn", func() {
			training := trainingSetup(t, ctx, detection(excursionUp, 60000, 63000))
			detectionRow := firstDetection(ctx, training)
			So(detectionRow, ShouldNotBeNil)

			asked, _, _, err := training.Rehearsal.learn(ctx, detectionRow, 0)
			So(err, ShouldNotBeNil)
			So(errnie.IsNotFound(err), ShouldBeTrue)
			So(err.Error(), ShouldContainSubstring, "no signal/logic tape")
			So(asked, ShouldBeNil)
			So(training.Rehearsal.Chart.Fragments(), ShouldBeEmpty)
		})

		Convey("The training pass fails Internal and closes the context cmd watches", func() {
			training := trainingFixture(t, ctx, detection(excursionUp, 60000, 63000))

			haltedInternal(training)
			So(training.Error().Error(), ShouldContainSubstring, "failed during rehearsal")
			So(training.Error().Error(), ShouldContainSubstring, "no signal/logic tape")
			So(census(training, "records"), ShouldEqual, 0)
		})
	})

	Convey("Given a stored detection whose signal/logic rows light no grid region", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Rows exist for every frame, but carry no numeric channel the grid
		// could place, so no frame produces a region token.
		blank := func(writer *tables.Writer, epoch int64) {
			for tick := int64(7); tick <= 15; tick++ {
				measurement := data.NewMeasurement(
					epoch, "BTC/USD", "cvd", tick*10, tick,
					&data.StringEntry{Key: "note", Value: "blank"},
				)
				measurement.At = time.Now().UTC()
				measurement.From = measurement.At
				// Write finalizes the row (WORM); it carries no metric.
				writer.Add("measurements", data.Publication{Measurement: measurement.Write()})
			}
		}

		training := trainingSetup(t, ctx, blank, detection(excursionUp, 60000, 63000))
		detectionRow := firstDetection(ctx, training)
		So(detectionRow, ShouldNotBeNil)

		_, _, _, err := training.Rehearsal.learn(ctx, detectionRow, 0)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "lights no grid region")
	})
}

func TestRehearsal_ShortPhasesTeachWeakened(t *testing.T) {
	cases := []struct {
		name string
		tape func(*tables.Writer, int64)
	}{
		// B=10, C=11: after the fill pullback the holding run keeps no frame;
		// learn still teaches enter (and exit on the ignition window) weakened.
		{"a holding run too short to leave a frame", detectionAt(excursionUp, 7, 10, 11, 60000, 63000)},
	}

	for _, tc := range cases {
		Convey("Given a stored detection with "+tc.name+" over an existing tape", t, func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			Convey("learn teaches enter (weakened) instead of soft-skipping", func() {
				training := trainingSetup(t, ctx, regimeTape, tc.tape)
				detectionRow := firstDetection(ctx, training)
				So(detectionRow, ShouldNotBeNil)

				asked, _, _, err := training.Rehearsal.learn(ctx, detectionRow, 0)
				So(err, ShouldBeNil)
				So(asked[actionEnter], ShouldBeGreaterThan, 0)
				So(census(training, actionEnter), ShouldBeGreaterThan, 0)
				So(len(training.Rehearsal.Chart.Fragments()), ShouldBeGreaterThan, 0)
			})

			Convey("The training pass records the short observation", func() {
				training := trainingFixture(t, ctx, regimeTape, tc.tape)

				So(training.Error(), ShouldBeNil)
				So(training.Context().Err(), ShouldBeNil)
				So(census(training, actionEnter), ShouldBeGreaterThan, 0)
			})
		})
	}
}

func TestRehearsal_CatalogReadFailureHalts(t *testing.T) {
	Convey("Given a stored excursion over its signal tape whose data files then fail to read", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		Convey("learn reports the read failure, not a missing tape", func() {
			training := trainingSetup(t, ctx, regimeTape, detection(excursionUp, 60000, 63000))
			detectionRow := firstDetection(ctx, training)
			So(detectionRow, ShouldNotBeNil)

			tablestest.DropDataFiles(t, training.catalog, tables.Measurements)

			asked, _, _, err := training.Rehearsal.learn(ctx, detectionRow, 0)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unable to read signal/logic tape")
			So(err.Error(), ShouldContainSubstring, "[iceberg]")
			So(err.Error(), ShouldNotContainSubstring, "no signal/logic tape")
			So(asked, ShouldBeNil)
			So(training.Rehearsal.Chart.Fragments(), ShouldBeEmpty)
		})

		Convey("A training pass fails instead of grading the trie on no excursions", func() {
			training := trainingSetup(t, ctx, regimeTape, detection(excursionUp, 60000, 63000))
			tablestest.DropDataFiles(t, training.catalog, tables.Measurements)

			graded, err := training.Rehearsal.pass(ctx)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unable to read stored detections")
			So(graded.trained, ShouldEqual, 0)
			So(graded.seen, ShouldEqual, 0)
		})

		Convey("Train halts Internal in the detector scan, before any pass, and records nothing", func() {
			training := trainingSetup(t, ctx, regimeTape, detection(excursionUp, 60000, 63000))
			tablestest.DropDataFiles(t, training.catalog, tables.Measurements)

			training.Train()

			// detectorDone closes after runDetectorScan's Error returns.
			// The context closes inside that call, before runtime.Close
			// finishes joining closer errors, so waiting on it would race.
			select {
			case <-training.detectorDone:
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for the detector scan to halt")
			}

			haltedInternal(training)
			So(training.Error().Error(), ShouldContainSubstring, "unable to read detections for detector scan")
			So(training.Error().Error(), ShouldContainSubstring, "[iceberg]")
			So(training.Passes(), ShouldEqual, 0)
			So(census(training, "records"), ShouldEqual, 0)
		})
	})
}

func TestSkill_Blocker(t *testing.T) {
	Convey("Given the grades a rehearsal can publish", t, func() {
		Convey("Past runs without a learnable excursion have nothing to rehearse, not a lack of skill", func() {
			So(skill{seen: 3}.blocker(), ShouldStartWith, "nothing to rehearse")
			So(skill{seen: 3}.open(), ShouldBeFalse)
		})

		Convey("A trie that was never graded on enter cannot open paper trading", func() {
			grade := skill{trained: 2, hits: 4, asked: map[string]int{actionExit: 2}}
			So(grade.blocker(), ShouldContainSubstring, "no enter phase")
			So(grade.open(), ShouldBeFalse)
		})

		Convey("Otherwise the gate compares the calls against the constant-policy baseline", func() {
			grade := skill{trained: 1, hits: 1, asked: map[string]int{actionEnter: 1, actionExit: 1}}
			So(grade.blocker(), ShouldContainSubstring, "skill gate: 1 of 2 calls correct")
			So(grade.blocker(), ShouldContainSubstring, "constant-policy baseline 1")
			So(grade.open(), ShouldBeFalse)

			grade.hits = 2
			So(grade.open(), ShouldBeTrue)

			grade.hits = 1
			grade.retained = 2
			So(grade.open(), ShouldBeTrue)
		})
	})
}

func TestPadWindow_ClampsNegativeLow(t *testing.T) {
	Convey("Given an ignition near the epoch start with a wide B→C", t, func() {
		Convey("padWindow never returns a negative lowTick", func() {
			// Crash repro: B small, move width large → b-left was -5923.
			lo, hi := padWindow(100, 12000)
			So(lo, ShouldBeGreaterThanOrEqualTo, 0)
			So(hi, ShouldBeGreaterThanOrEqualTo, lo)
			So(hi, ShouldBeGreaterThanOrEqualTo, int64(12000))
		})

		Convey("When B is at tick 0 the left pad clamps to 0", func() {
			lo, hi := padWindow(0, 50)
			So(lo, ShouldEqual, 0)
			So(hi, ShouldBeGreaterThanOrEqualTo, lo)
		})

		Convey("Short moves still get the minimum precursor pad when room allows", func() {
			lo, hi := padWindow(20, 22)
			So(lo, ShouldEqual, 20-minPrecursorPad)
			So(hi, ShouldBeGreaterThan, int64(22))
		})
	})
}

func TestClampTapeTicks(t *testing.T) {
	Convey("Given a tape read window", t, func() {
		Convey("A negative low is clamped to 0 when high stays above it", func() {
			lo, hi, err := clampTapeTicks(-5923, 19577)
			So(err, ShouldBeNil)
			So(lo, ShouldEqual, 0)
			So(hi, ShouldEqual, 19577)
		})

		Convey("A window that remains inverted after clamp is absurd geometry", func() {
			_, _, err := clampTapeTicks(-10, -5)
			So(err, ShouldNotBeNil)
			So(errnie.IsValidation(err), ShouldBeTrue)
			So(err.Error(), ShouldContainSubstring, "absurd tape ticks")
		})
	})
}

func TestExcursionWindow_AbsurdGeometry(t *testing.T) {
	Convey("Given an excursion with impossible B/C ticks", t, func() {
		Convey("window hard-errors instead of inventing a pad", func() {
			_, _, err := (excursion{b: 10, c: 10}).window()
			So(err, ShouldNotBeNil)
			So(errnie.IsValidation(err), ShouldBeTrue)
			So(err.Error(), ShouldContainSubstring, "absurd excursion geometry")

			_, _, err = (excursion{b: -1, c: 5}).window()
			So(err, ShouldNotBeNil)
			So(errnie.IsValidation(err), ShouldBeTrue)
		})

		Convey("A valid early-B move clamps without error", func() {
			lo, hi, err := (excursion{b: 2, c: 5000, end: 5000}).window()
			So(err, ShouldBeNil)
			So(lo, ShouldEqual, 0)
			So(hi, ShouldBeGreaterThanOrEqualTo, int64(5000))
		})
	})
}

func TestRehearsal_Augment(t *testing.T) {
	Convey("Given a fragment tape whose precursor crosses three regions", t, func() {
		region := func(name string) []byte { return []byte(name) }
		tokens := [][]byte{
			region("R0"), region("R0"), region("R1"), region("R2"), region("R2"), region("R3"),
		}
		// A may start on frames 0..3 and the stretch runs up to frame 5;
		// the graded draw started A on frame 3, so its context is "R2".
		stretch := drill{limit: 4, end: 5, feedback: 0.01}
		graded := contextOf(tokens[3:5])
		So(graded, ShouldEqual, "R2")

		recall := func(rehearsal *Rehearsal, context string) string {
			call, err := rehearsal.model.Recall(context, actionEnter)
			So(err, ShouldBeNil)
			return call.Winner
		}

		Convey("It teaches enter from every other distinct A, one per region run", func() {
			rehearsal := &Rehearsal{model: NewModel()}
			So(rehearsal.augment(tokens, stretch, graded, nil, false), ShouldBeNil)

			So(recall(rehearsal, "R0/R1/R2"), ShouldEqual, actionEnter)
			So(recall(rehearsal, "R1/R2"), ShouldEqual, actionEnter)

			records, err := rehearsal.model.Count("records")
			So(err, ShouldBeNil)

			// Frame 1 repeats frame 0's region and frame 3 is the graded
			// draw: neither adds a context, so a second run adds no record.
			So(rehearsal.augment(tokens, stretch, graded, nil, false), ShouldBeNil)
			again, err := rehearsal.model.Count("records")
			So(err, ShouldBeNil)
			So(again, ShouldEqual, records)
		})

		Convey("A drill whose graded draw taught nothing from A teaches nothing", func() {
			rehearsal := &Rehearsal{model: NewModel()}
			So(rehearsal.augment(tokens, drill{limit: 4, end: 5}, graded, nil, false), ShouldBeNil)

			records, err := rehearsal.model.Count("records")
			So(err, ShouldBeNil)
			So(records, ShouldEqual, 0)
		})

		Convey("A drill outside its tape is a validation error", func() {
			rehearsal := &Rehearsal{model: NewModel()}
			err := rehearsal.augment(tokens, drill{limit: 4, end: len(tokens) + 1, feedback: 0.01}, graded, nil, false)
			So(err, ShouldNotBeNil)
			So(errnie.IsValidation(err), ShouldBeTrue)
		})

		Convey("Dropout is off unless enabled, and adds missing-frame and noisy contexts when it is", func() {
			So(system.NewLearning().RehearsalDropout, ShouldBeFalse)

			census := func(dropout bool) float64 {
				rehearsal := &Rehearsal{model: NewModel()}

				// Each run draws a fresh missing frame and swap; enough runs
				// that the perturbations reach contexts no clean span holds.
				for range 16 {
					So(rehearsal.augment(tokens, stretch, graded, nil, dropout), ShouldBeNil)
				}

				records, err := rehearsal.model.Count("records")
				So(err, ShouldBeNil)
				return records
			}

			So(census(true), ShouldBeGreaterThan, census(false))
		})
	})
}
