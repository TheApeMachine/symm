package geometry

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestPhasePathNext(t *testing.T) {
	Convey("Given a phase path primitive", t, func() {
		op := NewPhasePath()

		Convey("One sample yields the origin angle", func() {
			samples := 1
			angles := tests.CollectSeq[PhasePathReading](op.Next(tests.SliceToSeq([]int{samples})))

			So(len(angles), ShouldEqual, 1)
			So(angles[0].Angles, ShouldResemble, []float64{0})
		})

		Convey("Four samples exclude the repeated endpoint", func() {
			samples := 4
			angles := tests.CollectSeq[PhasePathReading](op.Next(tests.SliceToSeq([]int{samples})))

			So(len(angles), ShouldEqual, 1)
			So(angles[0].Angles, ShouldHaveLength, 4)
			So(angles[0].Angles[0], ShouldEqual, 0)
			So(angles[0].Angles[1], ShouldEqual, math.Pi/2)
			So(angles[0].Angles[2], ShouldEqual, math.Pi)
			So(angles[0].Angles[3], ShouldEqual, 3*math.Pi/2)
		})

		Convey("A non-positive sample count is a domain failure", func() {
			op := NewPhasePath()
			angles := tests.CollectSeq[PhasePathReading](op.Next(tests.SliceToSeq([]int{0})))

			So(len(angles), ShouldEqual, 0)
			So(op.Error(), ShouldNotBeNil)
		})
	})
}

func TestNormalizeNext(t *testing.T) {
	Convey("Given a normalize primitive", t, func() {
		op := NewNormalize()

		Convey("A scaled dial is normalized to unit energy in place", func() {
			dial := PhaseDial{complex(3, 0), complex(0, 4)}
			results := tests.CollectSeq[PhaseDial](op.Next(tests.SliceToSeq([]PhaseDial{dial})))

			So(len(results), ShouldEqual, 1)
			So(dialNorm(results[0]), ShouldAlmostEqual, 1.0, 0.000000001)
			So(real(results[0][0]), ShouldAlmostEqual, 0.6, 0.000000001)
			So(real(results[0][1]), ShouldEqual, 0)
			So(imag(results[0][1]), ShouldAlmostEqual, 0.8, 0.000000001)
			So(real(dial[0]), ShouldEqual, real(results[0][0]))
		})

		Convey("A zero-energy dial passes through unchanged", func() {
			dial := PhaseDial{0, 0}
			results := tests.CollectSeq[PhaseDial](op.Next(tests.SliceToSeq([]PhaseDial{dial})))

			So(len(results), ShouldEqual, 1)
			So(results[0], ShouldResemble, PhaseDial{0, 0})
			So(op.Error(), ShouldBeNil)
		})
	})
}

func TestOverlapNext(t *testing.T) {
	Convey("Given an overlap primitive", t, func() {
		op := NewOverlap()

		Convey("Identical dials overlap at one", func() {
			pair := OverlapPair{
				Probe: PhaseDial{complex(1, 1), complex(2, -1)},
				Entry: PhaseDial{complex(1, 1), complex(2, -1)},
			}

			results := tests.CollectSeq[complex128](op.Next(tests.SliceToSeq([]OverlapPair{pair})))

			So(len(results), ShouldEqual, 1)
			So(real(results[0]), ShouldAlmostEqual, 1.0, 0.000000001)
			So(imag(results[0]), ShouldAlmostEqual, 0.0, 0.000000001)
		})

		Convey("Mismatched or empty dials yield zero", func() {
			pairs := []OverlapPair{
				{Probe: PhaseDial{1, 2}, Entry: PhaseDial{1}},
				{Probe: PhaseDial{}, Entry: PhaseDial{}},
			}

			results := tests.CollectSeq[complex128](op.Next(tests.SliceToSeq(pairs)))

			So(len(results), ShouldEqual, 2)
			So(results[0], ShouldEqual, complex128(0))
			So(results[1], ShouldEqual, complex128(0))
			So(op.Error(), ShouldBeNil)
		})
	})
}

/*
corpusStream streams one command and returns the single result.
*/
func corpusStream(
	op core.Primitive,
	command CorpusCommand[string],
) (CorpusResult[string], bool) {
	var result CorpusResult[string]

	for ptr := range op.Next(tests.SliceToSeq([]CorpusCommand[string]{command})) {
		result = *(*CorpusResult[string])(ptr)

		return result, true
	}

	return result, false
}

/*
insertEntry streams one insert command.
*/
func insertEntry(op core.Primitive, dial PhaseDial, outcome string, at time.Time) bool {
	entry := CorpusEntry[string]{Dial: dial, Outcome: outcome, At: at}
	result, ok := corpusStream(op, CorpusCommand[string]{Insert: &entry})

	return ok && result.Inserted
}

func TestCorpusNext(t *testing.T) {
	Convey("Given a corpus primitive", t, func() {
		op := NewCorpus[string](3)
		base := time.Now()

		Convey("Insertions are acknowledged and counted", func() {
			So(insertEntry(op, PhaseDial{1, 0}, "a", base), ShouldBeTrue)
			So(insertEntry(op, PhaseDial{0, 1}, "b", base.Add(time.Second)), ShouldBeTrue)

			result, ok := corpusStream(op, CorpusCommand[string]{Count: &CorpusCount{}})

			So(ok, ShouldBeTrue)
			So(result.Size, ShouldEqual, 2)
			So(op.Error(), ShouldBeNil)
		})

		Convey("At capacity the oldest entry is evicted", func() {
			So(insertEntry(op, PhaseDial{1, 0}, "a", base), ShouldBeTrue)
			So(insertEntry(op, PhaseDial{0, 1}, "b", base.Add(time.Second)), ShouldBeTrue)
			So(insertEntry(op, PhaseDial{1, 1}, "c", base.Add(2*time.Second)), ShouldBeTrue)
			So(insertEntry(op, PhaseDial{0, 1}, "d", base.Add(3*time.Second)), ShouldBeTrue)

			result, ok := corpusStream(op, CorpusCommand[string]{Count: &CorpusCount{}})

			So(ok, ShouldBeTrue)
			So(result.Size, ShouldEqual, 3)

			query := CorpusQuery{
				Dial:   PhaseDial{1, 0},
				Angles: []float64{0},
				TopK:   3,
			}
			scan, ok := corpusStream(op, CorpusCommand[string]{Query: &query})

			So(ok, ShouldBeTrue)
			So(len(scan.Scan), ShouldEqual, 1)
			So(scan.Scan[0], ShouldHaveLength, 3)

			outcomes := []string{}

			for _, match := range scan.Scan[0] {
				outcomes = append(outcomes, match.Outcome)
			}

			// "a" was evicted by the ring; the diagonally aligned entry "c"
			// ranks first, then the orthogonal "b" and "d" tie-break on time.
			So(outcomes, ShouldResemble, []string{"c", "b", "d"})
		})

		Convey("Dimensions are pinned by the first insert", func() {
			So(insertEntry(op, PhaseDial{1, 0}, "a", base), ShouldBeTrue)

			entry := CorpusEntry[string]{Dial: PhaseDial{1, 0, 0}, Outcome: "bad", At: base}
			_, ok := corpusStream(op, CorpusCommand[string]{Insert: &entry})

			So(ok, ShouldBeFalse)
			So(op.Error(), ShouldNotBeNil)
		})

		Convey("A zero-energy dial is rejected", func() {
			So(insertEntry(op, PhaseDial{0, 0}, "bad", base), ShouldBeFalse)
			So(op.Error(), ShouldNotBeNil)
		})

		Convey("Scan phases ranks by rotated similarity with timestamp tie-break", func() {
			first := base
			second := base.Add(time.Second)

			So(insertEntry(op, PhaseDial{1, 0}, "early", first), ShouldBeTrue)
			So(insertEntry(op, PhaseDial{1, 0}, "late", second), ShouldBeTrue)

			query := CorpusQuery{
				Dial:   PhaseDial{1, 0},
				Angles: []float64{0, math.Pi},
				TopK:   2,
			}
			scan, ok := corpusStream(op, CorpusCommand[string]{Query: &query})

			So(ok, ShouldBeTrue)
			So(len(scan.Scan), ShouldEqual, 2)

			// Equal similarity ties break toward the earlier timestamp.
			So(scan.Scan[0][0].Outcome, ShouldEqual, "early")
			So(scan.Scan[0][1].Outcome, ShouldEqual, "late")
			So(scan.Scan[0][0].Similarity, ShouldAlmostEqual, 1.0, 0.000000001)

			// The opposing rotation flips the ranking.
			So(scan.Scan[1][0].Similarity, ShouldAlmostEqual, -1.0, 0.000000001)
		})

		Convey("Excluded timestamps cannot select themselves", func() {
			resident := base.Add(time.Second)

			So(insertEntry(op, PhaseDial{1, 0}, "resident", resident), ShouldBeTrue)

			query := CorpusQuery{
				Dial:         PhaseDial{1, 0},
				Angles:       []float64{0},
				TopK:         1,
				ExcludeTimes: []time.Time{resident},
			}
			scan, ok := corpusStream(op, CorpusCommand[string]{Query: &query})

			So(ok, ShouldBeTrue)
			So(scan.Scan[0], ShouldHaveLength, 0)
		})

		Convey("An ambiguous command is a shape failure", func() {
			_, ok := corpusStream(op, CorpusCommand[string]{
				Count: &CorpusCount{},
			})

			So(ok, ShouldBeTrue)

			ambiguous := CorpusCommand[string]{
				Count:  &CorpusCount{},
				Insert: &CorpusEntry[string]{Dial: PhaseDial{1}, Outcome: "x", At: base},
			}

			_, ok = corpusStream(op, ambiguous)

			So(ok, ShouldBeFalse)
			So(op.Error(), ShouldNotBeNil)
		})
	})
}

func TestCorpusError(t *testing.T) {
	Convey("Given corpus construction", t, func() {
		Convey("A non-positive capacity is rejected", func() {
			op := NewCorpus[string](0)

			So(op.Error(), ShouldNotBeNil)

			count := 0

			for range op.Next(tests.SliceToSeq([]CorpusCommand[string]{
				{Count: &CorpusCount{}},
			})) {
				count++
			}

			So(count, ShouldEqual, 0)
		})
	})
}
