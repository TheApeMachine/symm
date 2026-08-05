package types

import (
	"encoding/json"
	"runtime"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReadinessStamp(t *testing.T) {
	Convey("Given readiness publishing to the dashboard channel", t, func() {
		ui := make(chan []byte, 1)
		readiness := NewReadiness(ui)
		stamped := make(chan struct{})

		go func() {
			readiness.Stamp(SourcePumpDump)
			close(stamped)
		}()

		select {
		case <-stamped:
		case <-time.After(time.Second):
			t.Fatal("readiness stamp blocked while publishing its snapshot")
		}

		var frame struct {
			Readiness Readiness `json:"readiness"`
		}

		select {
		case payload := <-ui:
			So(json.Unmarshal(payload, &frame), ShouldBeNil)
		case <-time.After(time.Second):
			t.Fatal("readiness stamp did not publish a dashboard frame")
		}

		Convey("It should publish the completed stamp without re-entering its mutex", func() {
			So(frame.Readiness.PumpDump, ShouldBeTrue)
			So(readiness.Snapshot().PumpDump, ShouldBeTrue)
		})
	})
}

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
		readiness := NewReadiness(nil)
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
		thesis := NewThesis(nil)
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
		thesis := NewThesis(nil)
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
		thesis := NewThesis(nil)
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
		thesis := NewThesis(nil)
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

func TestPrepareNextEvaluation(t *testing.T) {
	Convey("Given a thesis with epoch state and retained cycle evidence", t, func() {
		thesis := NewThesis(nil)
		thesis.Tick = 77
		thesis.Measurements.Store(SourceCVD, []*Measurement{{Source: SourceCVD, Symbol: "BTC/USD"}})
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
		stampAllSignals(&thesis.Readiness)
		thesis.Readiness.Manifold = true

		preparedAt := thesis.PrepareNextEvaluation().At

		Convey("It should clear only epoch state", func() {
			So(thesis.Tick, ShouldEqual, 77)
			So(thesis.CrossSection, ShouldNotBeNil)
			So(preparedAt.IsZero(), ShouldBeFalse)
			So(len(thesis.Decisions), ShouldEqual, 0)
			So(len(thesis.Findings), ShouldEqual, 1)
			So(len(thesis.Hypotheses), ShouldEqual, 1)
			So(len(thesis.Categories), ShouldEqual, 0)

			_, foundMeasurement := thesis.Measurements.Load(SourceCVD)
			So(foundMeasurement, ShouldBeTrue)

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

func TestCloseCycle(t *testing.T) {
	Convey("Given a thesis whose completed decision set has been emitted", t, func() {
		thesis := NewThesis(nil)
		stampAt := time.Unix(1, 0).UTC()
		thesis.Measurements.Store(SourceCVD, []*Measurement{{
			Source: SourceCVD, Symbol: "BTC/USD", At: stampAt,
		}})
		thesis.Findings = []Finding{{}}
		thesis.Hypotheses = []Hypothesis{{}}
		thesis.NoteLifecycle("BTC/USD", LifecycleManaging, stampAt)

		Convey("It should clear evidence without waiting for order settlement", func() {
			So(thesis.CloseCycle(), ShouldEqual, thesis)
			So(thesis.Series("BTC/USD"), ShouldBeEmpty)
			So(thesis.Findings, ShouldBeEmpty)
			So(thesis.Hypotheses, ShouldBeEmpty)
			_, foundLifecycle := thesis.Lifecycle.Load("BTC/USD")
			So(foundLifecycle, ShouldBeFalse)
		})
	})
}

func TestNoteLifecycle(t *testing.T) {
	Convey("Given a thesis phase transition", t, func() {
		thesis := NewThesis(nil)
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
	thesis := NewThesis(nil)
	at := time.Unix(1, 0).UTC()

	b.ReportAllocs()

	for b.Loop() {
		thesis.NoteLifecycle("BTC/USD", LifecycleManaging, at)
	}
}

func BenchmarkPrepareNextEvaluation(b *testing.B) {
	thesis := NewThesis(nil)

	b.ReportAllocs()

	for b.Loop() {
		thesis.PrepareNextEvaluation()
	}
}

/*
BenchmarkMeasurements measures the locked shared-Thesis signal publish path.
*/
func BenchmarkMeasurements(b *testing.B) {
	thesis := NewThesis(nil)
	row := &Measurement{Source: SourceCVD, Symbol: "BTC/USD", At: time.Unix(1, 0)}

	b.ReportAllocs()

	for b.Loop() {
		thesis.Measurements.Store(SourceCVD, []*Measurement{row})
	}
}
