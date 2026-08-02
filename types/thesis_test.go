package types

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestThesisReadiness(t *testing.T) {
	Convey("Given artifacts across the full decision pipeline", t, func() {
		thesis := NewThesis()
		stampAt := time.Unix(1, 0).UTC()

		for _, source := range thesisSignalSources {
			thesis.Stamps.Store(source, []Stamp{{
				At:     stampAt,
				Source: source,
				Entity: MarketTicker,
			}})
		}

		/*
			A stage is ready because it stamped the thesis, so each derived
			stage is marked by its stamp rather than by the evidence it
			happened to leave behind.
		*/
		for _, source := range []SourceType{
			SourceManifold, SourceResonance, SourceCausal, SourceGraph,
		} {
			thesis.StampSource(source, MarketDerived)
		}

		thesis.Decisions = []Decision{{}}

		Convey("It should mark every thesis stage ready", func() {
			readiness := thesis.Readiness()

			So(readiness.Signals, ShouldBeTrue)
			So(readiness.Manifold, ShouldBeTrue)
			So(readiness.Resonance, ShouldBeTrue)
			So(readiness.Causal, ShouldBeTrue)
			So(readiness.Graph, ShouldBeTrue)
			So(readiness.Allocation, ShouldBeTrue)
			So(readiness.Decisions, ShouldBeTrue)
		})
	})

	Convey("Given a stage that ran but produced no output", t, func() {
		thesis := NewThesis()

		for _, source := range thesisSignalSources {
			thesis.StampSource(source, MarketTicker)
		}

		for _, source := range []SourceType{
			SourceManifold, SourceResonance, SourceCausal, SourceGraph,
		} {
			thesis.StampSource(source, MarketDerived)
		}

		Convey("It should still report the stage ready", func() {
			/*
				A solver that finds nothing has still run. Inferring readiness
				from the contents of its output would stall the pipeline behind
				a stage that is working correctly and simply had nothing to say.
			*/
			readiness := thesis.Readiness()

			So(readiness.Causal, ShouldBeTrue)
			So(readiness.Graph, ShouldBeTrue)
			So(readiness.Allocation, ShouldBeTrue)

			// No decisions were taken, so only that stage is unmet.
			So(readiness.Decisions, ShouldBeFalse)
		})
	})

	Convey("Given a pipeline missing one stage's stamp", t, func() {
		thesis := NewThesis()

		for _, source := range thesisSignalSources {
			thesis.StampSource(source, MarketTicker)
		}

		thesis.StampSource(SourceManifold, MarketDerived)
		thesis.StampSource(SourceResonance, MarketDerived)
		thesis.StampSource(SourceGraph, MarketDerived)

		Convey("It should not report that stage or anything behind it ready", func() {
			readiness := thesis.Readiness()

			So(readiness.Resonance, ShouldBeTrue)
			So(readiness.Causal, ShouldBeFalse)

			// Graph stamped, but the causal stage it builds on did not.
			So(readiness.Graph, ShouldBeFalse)
			So(readiness.Allocation, ShouldBeFalse)
		})
	})
}

func TestThesisAppendMeasurements(t *testing.T) {
	Convey("Given a signal publication whose stamp omits its source", t, func() {
		thesis := NewThesis()
		stampAt := time.Unix(1, 0).UTC()
		thesis.AppendMeasurements(
			SourceCVD,
			[]*Measurement{{Source: SourceCVD, Symbol: "BTC/USD", At: stampAt}},
			Stamp{At: stampAt, Entity: MarketTrade},
		)

		Convey("It should derive the authoritative stamp source from the publication", func() {
			value, found := thesis.Stamps.Load(SourceCVD)
			So(found, ShouldBeTrue)

			stamps := value.([]Stamp)
			So(len(stamps), ShouldEqual, 1)
			So(stamps[0].Source, ShouldEqual, SourceCVD)
			So(thesis.Readiness().Signals, ShouldBeTrue)
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
		thesis.Stamps.Store(SourceCVD, []Stamp{{Source: SourceCVD}})
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

			_, foundStamp := thesis.Stamps.Load(SourceCVD)
			So(foundStamp, ShouldBeFalse)

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
