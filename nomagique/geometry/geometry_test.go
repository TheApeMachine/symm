package geometry

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
)

func TestPhasePathNext(t *testing.T) {
	Convey("Given a phase path primitive", t, func() {
		op := NewPhasePath()

		Convey("One sample yields the origin angle", func() {
			reading := op(1)

			So(len(reading.Angles), ShouldEqual, 1)
			So(reading.Angles, ShouldResemble, []float64{0})
		})

		Convey("Four samples exclude the repeated endpoint", func() {
			reading := op(4)

			So(len(reading.Angles), ShouldEqual, 4)
			So(reading.Angles[0], ShouldEqual, 0)
			So(reading.Angles[1], ShouldEqual, math.Pi/2)
			So(reading.Angles[2], ShouldEqual, math.Pi)
			So(reading.Angles[3], ShouldEqual, 3*math.Pi/2)
		})

		Convey("A non-positive sample count yields empty angles", func() {
			reading := op(0)

			So(len(reading.Angles), ShouldEqual, 0)
		})
	})
}

func TestNormalizeNext(t *testing.T) {
	Convey("Given a normalize primitive", t, func() {
		op := NewNormalize()

		Convey("A scaled dial is normalized to unit energy", func() {
			dial := PhaseDial{complex(3, 0), complex(0, 4)}
			result := op(dial)

			So(dialNorm(result), ShouldAlmostEqual, core.Unit, 0.000000001)
			So(real(result[0]), ShouldAlmostEqual, 0.6, 0.000000001)
			So(real(result[1]), ShouldEqual, 0)
			So(imag(result[1]), ShouldAlmostEqual, 0.8, 0.000000001)
		})

		Convey("A zero-energy dial passes through unchanged", func() {
			dial := PhaseDial{0, 0}
			result := op(dial)

			So(result, ShouldResemble, PhaseDial{0, 0})
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

			result := op(pair)

			So(real(result), ShouldAlmostEqual, core.Unit, 0.000000001)
			So(imag(result), ShouldAlmostEqual, 0.0, 0.000000001)
		})

		Convey("Mismatched or empty dials yield zero", func() {
			pair1 := OverlapPair{Probe: PhaseDial{1, 2}, Entry: PhaseDial{1}}
			pair2 := OverlapPair{Probe: PhaseDial{}, Entry: PhaseDial{}}

			So(op(pair1), ShouldEqual, complex128(0))
			So(op(pair2), ShouldEqual, complex128(0))
		})
	})
}

func insertEntry(op Corpus[string], dial PhaseDial, outcome string, at time.Time) bool {
	entry := CorpusEntry[string]{Dial: dial, Outcome: outcome, At: at}
	res := op(CorpusCommand[string]{Insert: &entry})
	return res.Inserted
}

func TestCorpusNext(t *testing.T) {
	Convey("Given a corpus primitive", t, func() {
		op := NewCorpus[string](3)
		base := time.Now()

		Convey("Insertions are acknowledged and counted", func() {
			So(insertEntry(op, PhaseDial{1, 0}, "a", base), ShouldBeTrue)
			So(insertEntry(op, PhaseDial{0, 1}, "b", base.Add(time.Second)), ShouldBeTrue)

			result := op(CorpusCommand[string]{Count: &CorpusCount{}})

			So(result.Size, ShouldEqual, 2)
		})

		Convey("At capacity the oldest entry is evicted", func() {
			So(insertEntry(op, PhaseDial{1, 0}, "a", base), ShouldBeTrue)
			So(insertEntry(op, PhaseDial{0, 1}, "b", base.Add(time.Second)), ShouldBeTrue)
			So(insertEntry(op, PhaseDial{1, 1}, "c", base.Add(2*time.Second)), ShouldBeTrue)
			So(insertEntry(op, PhaseDial{0, 1}, "d", base.Add(3*time.Second)), ShouldBeTrue)

			result := op(CorpusCommand[string]{Count: &CorpusCount{}})

			So(result.Size, ShouldEqual, 3)

			query := CorpusQuery{
				Dial:   PhaseDial{1, 0},
				Angles: []float64{0},
				TopK:   3,
			}
			scan := op(CorpusCommand[string]{Query: &query})

			So(len(scan.Scan), ShouldEqual, 1)
			So(scan.Scan[0], ShouldHaveLength, 3)

			outcomes := []string{}
			for _, match := range scan.Scan[0] {
				outcomes = append(outcomes, match.Outcome)
			}

			// "a" was evicted by the ring; "c" ranks first, then "b" and "d"
			So(outcomes, ShouldResemble, []string{"c", "b", "d"})
		})

		Convey("Dimensions are pinned by the first insert", func() {
			So(insertEntry(op, PhaseDial{1, 0}, "a", base), ShouldBeTrue)

			entry := CorpusEntry[string]{Dial: PhaseDial{1, 0, 0}, Outcome: "bad", At: base}
			res := op(CorpusCommand[string]{Insert: &entry})

			So(res.Inserted, ShouldBeFalse)
		})

		Convey("A zero-energy dial is rejected", func() {
			So(insertEntry(op, PhaseDial{0, 0}, "bad", base), ShouldBeFalse)
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
			scan := op(CorpusCommand[string]{Query: &query})

			So(len(scan.Scan), ShouldEqual, 2)
			So(scan.Scan[0][0].Outcome, ShouldEqual, "early")
			So(scan.Scan[0][1].Outcome, ShouldEqual, "late")
			So(scan.Scan[0][0].Similarity, ShouldAlmostEqual, core.Unit, 0.000000001)
			So(scan.Scan[1][0].Similarity, ShouldAlmostEqual, -core.Unit, 0.000000001)
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
			scan := op(CorpusCommand[string]{Query: &query})

			So(scan.Scan[0], ShouldHaveLength, 0)
		})

		Convey("An ambiguous command is rejected", func() {
			ambiguous := CorpusCommand[string]{
				Count:  &CorpusCount{},
				Insert: &CorpusEntry[string]{Dial: PhaseDial{1}, Outcome: "x", At: base},
			}

			res := op(ambiguous)
			So(res.Inserted, ShouldBeFalse)
			So(res.Size, ShouldEqual, 0)
		})
	})
}
