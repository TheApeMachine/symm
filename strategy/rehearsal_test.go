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
			// Post-teach Predict: when the trie already agrees, markers align
			// with the ground-truth sweet spots; abstention leaves -1.
			if frag.PredictedEntryIdx >= 0 {
				So(frag.PredictedEntryIdx, ShouldEqual, frag.EntryIdx)
			}
			if frag.PredictedExitIdx >= 0 {
				So(frag.PredictedExitIdx, ShouldEqual, frag.ExitIdx)
			}
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
			grade := skill{trained: 2, hits: 4, asked: map[string]int{actionWait: 2, actionExit: 2}}
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
