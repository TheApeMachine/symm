package opportunity

import (
	"runtime"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
activeBatch builds a ranked category batch whose listed categories all carry
positive support, so the opportunity tracker reads them as active.
*/
func activeBatch(symbol string, categories ...types.CategoryType) []types.Category {
	batch := make([]types.Category, 0, len(categories))

	for _, category := range categories {
		batch = append(batch, types.Category{
			Symbol:   symbol,
			Type:     category,
			Strength: 1,
			Maturity: 0.5,
		})
	}

	return batch
}

/*
stepBatch runs one category batch through the solver via an envelope and
returns the resulting Opportunities (nil when none were produced).
*/
func stepBatch(solver *Solver, categories []types.Category) []*types.OpportunityCandidate {
	envelope := types.NewEnvelope(types.EnvelopeUnknown)
	envelope.Categories = categories

	return solver.Step(envelope).Opportunities
}

func TestSolverStep(t *testing.T) {
	Convey("Given an opportunity synthesizer", t, func() {
		solver := NewSolver(t.Context())

		Convey("a batch with no categories yields nothing", func() {
			So(stepBatch(solver, nil), ShouldBeNil)
		})

		Convey("a dormant symbol with no precursor yields nothing", func() {
			candidates := stepBatch(solver, activeBatch("DORMANT", types.StochasticNoise))

			So(candidates, ShouldBeNil)
		})

		Convey("one active precursor forms a candidate", func() {
			candidates := stepBatch(solver, activeBatch("HMAID", types.CoiledCompression))

			So(candidates, ShouldHaveLength, 1)

			candidate := candidates[0]

			So(candidate.Symbol, ShouldEqual, "HMAID")
			So(candidate.Archetype, ShouldEqual, types.ArchetypeVerticalIgnition)
			So(candidate.Phase, ShouldEqual, types.PhaseForming)
			So(candidate.Direction, ShouldEqual, types.DirectionLong)
			So(candidate.Sequence, ShouldEqual, 1)
			So(candidate.Provenance, ShouldEqual, types.ProvenanceCategory)
			So(candidate.Economics, ShouldBeNil)
		})

		Convey("two agreeing precursors arm the candidate", func() {
			candidates := stepBatch(solver, activeBatch(
				"HMAID",
				types.CoiledCompression,
				types.HiddenAbsorption,
			))

			So(candidates, ShouldHaveLength, 1)
			So(candidates[0].Phase, ShouldEqual, types.PhaseArmed)
		})

		Convey("the confirmation category ignites the candidate", func() {
			candidates := stepBatch(solver, activeBatch(
				"HMAID",
				types.CoiledCompression,
				types.VerticalIgnition,
			))

			So(candidates, ShouldHaveLength, 1)
			So(candidates[0].Phase, ShouldEqual, types.PhaseIgnition)
		})

		Convey("a formed candidate advances its sequence on update", func() {
			formed := stepBatch(solver, activeBatch("HMAID", types.CoiledCompression))

			candidates := stepBatch(solver, activeBatch(
				"HMAID",
				types.CoiledCompression,
				types.HiddenAbsorption,
			))

			So(candidates, ShouldHaveLength, 1)
			So(candidates[0].Phase, ShouldEqual, types.PhaseArmed)
			So(candidates[0].Sequence, ShouldEqual, 2)
			So(formed[0].Phase, ShouldEqual, types.PhaseForming)
			So(formed[0].Sequence, ShouldEqual, 1)
		})

		Convey("concurrent workloads advance one atomic candidate sequence", func() {
			workerCount := runtime.GOMAXPROCS(0)
			sequences := make(chan uint64, workerCount)
			var waitGroup sync.WaitGroup

			for range workerCount {
				waitGroup.Add(1)

				go func() {
					defer waitGroup.Done()

					candidates := stepBatch(solver, activeBatch(
						"HMAID",
						types.CoiledCompression,
					))
					sequences <- candidates[0].Sequence
				}()
			}

			waitGroup.Wait()
			close(sequences)

			observed := make(map[uint64]bool, workerCount)

			for sequence := range sequences {
				observed[sequence] = true
			}

			So(observed, ShouldHaveLength, workerCount)

			for sequence := uint64(1); sequence <= uint64(workerCount); sequence++ {
				So(observed[sequence], ShouldBeTrue)
			}
		})

		Convey("a dissolved precursor state invalidates exactly once", func() {
			stepBatch(solver, activeBatch("HMAID", types.CoiledCompression))

			invalidated := stepBatch(solver, activeBatch("HMAID", types.StochasticNoise))

			So(invalidated, ShouldHaveLength, 1)
			So(invalidated[0].Phase, ShouldEqual, types.PhaseInvalidated)

			again := stepBatch(solver, activeBatch("HMAID", types.StochasticNoise))

			So(again, ShouldBeNil)
		})

		Convey("re-forming after invalidation starts a fresh candidate", func() {
			stepBatch(solver, activeBatch("HMAID", types.CoiledCompression))
			stepBatch(solver, activeBatch("HMAID", types.StochasticNoise))

			candidates := stepBatch(solver, activeBatch("HMAID", types.BookThinning))

			So(candidates, ShouldHaveLength, 1)
			So(candidates[0].Phase, ShouldEqual, types.PhaseForming)
			So(candidates[0].Sequence, ShouldEqual, 1)
		})
	})
}

func TestActiveCategories(t *testing.T) {
	Convey("Given a ranked category batch", t, func() {
		batch := []types.Category{
			{Symbol: "A", Type: types.CoiledCompression, Strength: 1, Maturity: 0.5},
			{Symbol: "A", Type: types.StochasticNoise, Strength: 0, Maturity: 0.5},
			{Symbol: "A", Type: types.HiddenAbsorption, Strength: 2, Maturity: 0.9},
		}

		Convey("activeCategories keeps only positive-strength categories", func() {
			active := activeCategories(batch)

			So(active, ShouldContainKey, types.CoiledCompression)
			So(active, ShouldContainKey, types.HiddenAbsorption)
			So(active, ShouldNotContainKey, types.StochasticNoise)
			So(active[types.HiddenAbsorption], ShouldEqual, 0.9)
		})
	})
}

func TestPhaseFor(t *testing.T) {
	declared := families[0]

	Convey("Given the vertical ignition family", t, func() {
		Convey("no active evidence is dormant", func() {
			phase, found := phaseFor(declared, map[types.CategoryType]float64{})

			So(found, ShouldBeFalse)
			So(phase, ShouldEqual, types.PhaseDormant)
		})

		Convey("one precursor is forming", func() {
			phase, found := phaseFor(declared, activeCategories(activeBatch(
				"A",
				types.BookThinning,
			)))

			So(found, ShouldBeTrue)
			So(phase, ShouldEqual, types.PhaseForming)
		})

		Convey("two precursors are armed", func() {
			phase, _ := phaseFor(declared, activeCategories(activeBatch(
				"A",
				types.BookThinning,
				types.Frenzy,
			)))

			So(phase, ShouldEqual, types.PhaseArmed)
		})

		Convey("confirmation dominates precursors", func() {
			phase, _ := phaseFor(declared, activeCategories(activeBatch(
				"A",
				types.BookThinning,
				types.VerticalIgnition,
			)))

			So(phase, ShouldEqual, types.PhaseIgnition)
		})
	})
}

func TestFamilyMaturity(t *testing.T) {
	declared := families[0]

	Convey("Given active evidence with differing maturity", t, func() {
		active := map[types.CategoryType]float64{
			types.CoiledCompression: 0.3,
			types.HiddenAbsorption:  0.8,
		}

		Convey("familyMaturity reports the strongest maturity", func() {
			So(familyMaturity(declared, active), ShouldEqual, 0.8)
		})

		Convey("familyMaturity reports zero with no evidence", func() {
			So(familyMaturity(declared, map[types.CategoryType]float64{}), ShouldEqual, 0.0)
		})
	})
}

func BenchmarkSolverStep(b *testing.B) {
	solver := NewSolver(b.Context())
	categories := activeBatch(
		"HMAID",
		types.CoiledCompression,
		types.HiddenAbsorption,
	)

	b.ResetTimer()

	for range b.N {
		stepBatch(solver, categories)
	}
}
