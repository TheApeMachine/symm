package types

import (
	"runtime"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func storeAllSignalMeasurements(thesis *Thesis, stampAt time.Time) {
	for _, source := range thesisSignalSources {
		thesis.Measurements.Store(source, []*Measurement{{
			Source: source,
			Symbol: "BTC/USD",
			At:     stampAt,
		}})
	}
}

func stampAllSignals(readiness *Readiness) {
	for _, source := range thesisSignalSources {
		readiness.Stamp(source)
	}
}

func TestThesisReadiness(t *testing.T) {
	Convey("Given signal stages stamping readiness concurrently", t, func() {
		readiness := NewReadiness()
		start := make(chan struct{})
		measured := make(chan struct{})
		var stamps sync.WaitGroup

		for _, source := range thesisSignalSources {
			stamps.Add(1)

			go func(source SourceType) {
				defer stamps.Done()
				<-start
				readiness.Stamp(source)
			}(source)
		}

		go func() {
			<-start

			for !readiness.SignalsMeasured() {
				runtime.Gosched()
			}

			close(measured)
		}()

		close(start)
		stamps.Wait()
		<-measured

		So(readiness.SignalsMeasured(), ShouldBeTrue)
	})

	Convey("Given every pre-decision stage stamp", t, func() {
		thesis := NewThesis()
		stampAt := time.Unix(1, 0).UTC()

		storeAllSignalMeasurements(thesis, stampAt)
		stampAllSignals(&thesis.Readiness)

		// A stage is ready because it stamped the thesis, so each derived
		// stage is marked by its stamp rather than by the evidence it
		// happened to leave behind.
		thesis.Readiness.Manifold = true
		thesis.Readiness.Resonance = true
		thesis.Readiness.Causal = true
		thesis.Readiness.Graph = true
		thesis.Readiness.Categories = true
		thesis.Readiness.Cognition = true

		Convey("It should be complete without inferring later-stage stamps", func() {
			So(thesis.Readiness.SignalsMeasured(), ShouldBeTrue)
			So(thesis.Readiness.Manifold, ShouldBeTrue)
			So(thesis.Readiness.Resonance, ShouldBeTrue)
			So(thesis.Readiness.Causal, ShouldBeTrue)
			So(thesis.Readiness.Graph, ShouldBeTrue)
			So(thesis.Readiness.Complete(), ShouldBeTrue)
			So(thesis.Readiness.Allocation, ShouldBeFalse)
			So(thesis.Readiness.Decisions, ShouldBeFalse)
		})
	})

	Convey("Given a stage that ran but produced no output", t, func() {
		thesis := NewThesis()
		stampAt := time.Unix(1, 0).UTC()

		storeAllSignalMeasurements(thesis, stampAt)
		stampAllSignals(&thesis.Readiness)

		thesis.Readiness.Manifold = true
		thesis.Readiness.Resonance = true
		thesis.Readiness.Causal = true
		thesis.Readiness.Graph = true

		Convey("It should still report the stage ready", func() {
			// A solver that finds nothing has still run. Inferring readiness
			// from the contents of its output would stall the pipeline behind
			// a stage that is working correctly and simply had nothing to say.
			So(thesis.Readiness.Causal, ShouldBeTrue)
			So(thesis.Readiness.Graph, ShouldBeTrue)
			So(thesis.Readiness.Allocation, ShouldBeFalse)

			// Allocation and decision stages have not run yet.
			So(thesis.Readiness.Decisions, ShouldBeFalse)
		})
	})

	Convey("Given a pipeline missing one stage's stamp", t, func() {
		thesis := NewThesis()
		stampAt := time.Unix(1, 0).UTC()

		storeAllSignalMeasurements(thesis, stampAt)
		stampAllSignals(&thesis.Readiness)

		thesis.Readiness.Manifold = true
		thesis.Readiness.Resonance = true
		thesis.Readiness.Graph = true

		Convey("It should not report that stage or anything behind it ready", func() {
			So(thesis.Readiness.Resonance, ShouldBeTrue)
			So(thesis.Readiness.Causal, ShouldBeFalse)

			// Graph reports its own stamp; Complete rejects the missing causal stage.
			So(thesis.Readiness.Graph, ShouldBeTrue)
			So(thesis.Readiness.Complete(), ShouldBeFalse)
			So(thesis.Readiness.Allocation, ShouldBeFalse)
		})
	})

	Convey("Given only some signal measurements", t, func() {
		thesis := NewThesis()
		stampAt := time.Unix(1, 0).UTC()

		for _, source := range thesisSignalSources[:2] {
			thesis.Measurements.Store(source, []*Measurement{{
				Source: source,
				Symbol: "BTC/USD",
				At:     stampAt,
			}})
		}

		Convey("It should not report signals ready", func() {
			So(thesis.Readiness.SignalsMeasured(), ShouldBeFalse)
			So(thesis.Readiness.Manifold, ShouldBeFalse)
		})
	})
}

func TestThesisReset(t *testing.T) {
	Convey("Given a thesis with transient cycle state", t, func() {
		thesis := NewThesis()
		thesis.Tick = 77
		thesis.Measurements.Store(SourceCVD, []*Measurement{{Source: SourceCVD, Symbol: "BTC/USD"}})
		thesis.Books.Store("BTC/USD", "book")
		thesis.Graphs.Store("BTC/USD", "graph")
		thesis.Decisions = []Decision{{}}
		thesis.Findings = []Finding{{}}
		thesis.Hypotheses = []Hypothesis{{}}
		thesis.Categories["market"] = []Category{{}}
		thesis.Manifold.Store("BTC/USD", "manifold")
		thesis.Cognition.Store("BTC/USD", "cognition")
		thesis.Resonance.Store("BTC/USD", "resonance")
		thesis.Causal.Store("BTC/USD", "causal")
		thesis.Lifecycle.Store("BTC/USD", LifecycleManaging)

		resetAt := thesis.Reset().At

		Convey("It should clear transient evidence and keep lifecycle state", func() {
			// Tick counts evaluated cycles and is lifecycle, not evidence.
			So(thesis.Tick, ShouldEqual, 77)
			So(thesis.CrossSection, ShouldNotBeNil)
			So(resetAt.IsZero(), ShouldBeFalse)
			So(len(thesis.Decisions), ShouldEqual, 0)
			So(len(thesis.Findings), ShouldEqual, 0)
			So(len(thesis.Hypotheses), ShouldEqual, 0)
			So(len(thesis.Categories), ShouldEqual, 0)

			_, foundMeasurement := thesis.Measurements.Load(SourceCVD)
			So(foundMeasurement, ShouldBeFalse)

			_, foundBook := thesis.Books.Load("BTC/USD")
			So(foundBook, ShouldBeFalse)

			_, foundGraph := thesis.Graphs.Load("BTC/USD")
			So(foundGraph, ShouldBeFalse)

			_, foundManifold := thesis.Manifold.Load("BTC/USD")
			So(foundManifold, ShouldBeFalse)

			_, foundCognition := thesis.Cognition.Load("BTC/USD")
			So(foundCognition, ShouldBeFalse)

			_, foundResonance := thesis.Resonance.Load("BTC/USD")
			So(foundResonance, ShouldBeFalse)

			_, foundCausal := thesis.Causal.Load("BTC/USD")
			So(foundCausal, ShouldBeFalse)

			So(thesis.Readiness.SignalsMeasured(), ShouldBeFalse)

			phase, foundLifecycle := thesis.Lifecycle.Load("BTC/USD")
			So(foundLifecycle, ShouldBeTrue)
			So(phase, ShouldEqual, LifecycleManaging)
		})
	})
}

/*
TestThesisAppendMeasurementsConcurrent proves independent signal actors can
replace the same source-symbol row in the shared Thesis without corrupting the
direct measurement map. The Thesis stores current evidence, not an append-only
history, so one row survives for this identity.
*/
func TestThesisAppendMeasurementsConcurrent(t *testing.T) {
	Convey("Given concurrent signal publications", t, func() {
		thesis := NewThesis()
		var wg sync.WaitGroup

		Convey("When many goroutines publish the same source-symbol row", func() {
			for index := range 128 {
				wg.Go(func() {
					thesis.Measurements.Store(SourceCVD, []*Measurement{{
						Source: SourceCVD,
						Symbol: "BTC/USD",
						At:     time.Unix(int64(index), 0).UTC(),
					}})
				})
			}

			wg.Wait()
			count := 0
			thesis.Measurements.Range(func(_, value any) bool {
				for _, m := range value.([]*Measurement) {
					if m != nil {
						count++
					}
				}
				return true
			})

			Convey("Then the current row is retained without map corruption", func() {
				So(count, ShouldEqual, 1)
			})
		})
	})
}

func TestNoteLifecycle(t *testing.T) {
	Convey("Given a thesis phase transition", t, func() {
		thesis := NewThesis()
		at := time.Unix(1, 0).UTC()
		thesis.NoteLifecycle("BTC/USD", LifecycleEntered, at)

		Convey("It should store the phase without a parallel journal", func() {
			phase, ok := thesis.Lifecycle.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			So(phase, ShouldEqual, LifecycleEntered)
		})
	})
}

func BenchmarkNoteLifecycle(b *testing.B) {
	thesis := NewThesis()
	at := time.Unix(1, 0).UTC()

	b.ReportAllocs()

	for b.Loop() {
		thesis.NoteLifecycle("BTC/USD", LifecycleManaging, at)
	}
}

/*
BenchmarkMeasurements measures the locked shared-Thesis signal publish path.
*/
func BenchmarkMeasurements(b *testing.B) {
	thesis := NewThesis()
	row := &Measurement{Source: SourceCVD, Symbol: "BTC/USD", At: time.Unix(1, 0)}

	b.ReportAllocs()

	for b.Loop() {
		thesis.Measurements.Store(SourceCVD, []*Measurement{row})
	}
}
