package cognition

import (
	"strconv"
	"sync"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
drive executes one engine command and returns its single result.
*/
func drive(t *testing.T, engine core.Primitive, command *Command) Result {
	t.Helper()

	evaluation := transport.NewEvaluate(engine)
	var result Result

	for out := range evaluation.Next(transport.NewOne(unsafe.Pointer(command)).Next(nil)) {
		result = *(*Result)(out)
	}

	if err := evaluation.Error(); err != nil {
		t.Fatal(err)
	}

	return result
}

/*
observing records one association and ignores the root it published.
*/
func observing(t *testing.T, engine core.Primitive, context, class []byte, feedback ...float64) {
	t.Helper()

	assoc := Association{Context: context, Class: class}

	if len(feedback) > 0 {
		assoc.Feedback = feedback[0]
		assoc.Graded = true
	}

	drive(t, engine, &Command{Observe: &assoc})
}

/*
evaluating classifies one context.
*/
func evaluating(t *testing.T, engine core.Primitive, context []byte) Evaluation {
	t.Helper()

	return drive(t, engine, &Command{Evaluate: &Question{Context: context}}).Evaluation
}

func TestEngineObserve(t *testing.T) {
	Convey("A cognitive engine classifies the basin a context was observed in", t, func() {
		engine := NewEngine(Config{})
		context := []byte{0x01, 0x02, 0x03}

		observing(t, engine, context, []byte("enter"))
		observing(t, engine, context, []byte("enter"))
		observing(t, engine, context, []byte("enter"))
		observing(t, engine, context, []byte("wait"))

		evaluation := evaluating(t, engine, context)
		So(evaluation.WinnerClass, ShouldEqual, "enter")
		So(evaluation.Confidence, ShouldBeGreaterThan, 0)
		So(evaluation.Surprisal, ShouldBeGreaterThanOrEqualTo, 0)
		So(evaluation.IsBreak, ShouldBeFalse)
		So(evaluation.Support, ShouldEqual, 3)
	})
}

func TestEngineKeyLayout(t *testing.T) {
	Convey("Observation writes exactly b/<context>/<class> and s/<context>", t, func() {
		engine := NewEngine(Config{}).(*Engine)
		context := []byte("formation")

		observing(t, engine, context, []byte("enter"))

		root := drive(t, engine, &Command{Root: &Root{}}).Tree
		basin, found := root.Get(makeBasinKey([]byte("enter"), context))
		So(found, ShouldBeTrue)
		So(len(basin), ShouldEqual, WeightSize)
		So(decodeWeight(basin).Count, ShouldEqual, 1)

		_, sensory := root.Get(makeSensoryKey(context))
		So(sensory, ShouldBeTrue)
	})
}

func TestEngineEvaluateUnseen(t *testing.T) {
	Convey("An unseen context carries Dirichlet-baseline surprisal", t, func() {
		engine := NewEngine(Config{})

		evaluation := evaluating(t, engine, []byte{0x09, 0x08})
		So(evaluation.WinnerClass, ShouldEqual, "")
		So(evaluation.Surprisal, ShouldBeGreaterThan, 0)
		So(evaluation.IsBreak, ShouldBeTrue)
	})
}

func TestEngineEvaluateEmpty(t *testing.T) {
	Convey("An empty context cannot be matched and reports a break", t, func() {
		engine := NewEngine(Config{})

		evaluation := evaluating(t, engine, nil)
		So(evaluation.IsBreak, ShouldBeTrue)
	})
}

func TestEngineObserveEmptyContext(t *testing.T) {
	Convey("An observation without a context is a domain failure, not a write", t, func() {
		engine := NewEngine(Config{})
		evaluation := transport.NewEvaluate(engine)
		command := Command{Observe: &Association{Class: []byte("enter")}}

		for range evaluation.Next(transport.NewOne(unsafe.Pointer(&command)).Next(nil)) {
			t.Fatal("an invalid observation must not yield")
		}

		So(evaluation.Error(), ShouldNotBeNil)
	})
}

func TestEngineObserveFeedback(t *testing.T) {
	Convey("Graded feedback inhibits bad actions and reinforces better ones in packed basins", t, func() {
		config := Config{MemoryScale: 1} // Disable decay to isolate feedback in this fixture.
		engine := NewEngine(config)
		sequence := []byte("formation/ignition")

		observing(t, engine, sequence, []byte("enter"), 1)
		observing(t, engine, sequence, []byte("wait"), -1)
		So(evaluating(t, engine, sequence).WinnerClass, ShouldEqual, "enter")

		for range 16 { // A changed outcome regime must reverse the preference.
			observing(t, engine, sequence, []byte("enter"), -1)
			observing(t, engine, sequence, []byte("wait"), 1)
		}

		So(evaluating(t, engine, sequence).WinnerClass, ShouldEqual, "wait")

		root := drive(t, engine, &Command{Root: &Root{}}).Tree
		before, _ := root.Get(makeBasinKey([]byte("wait"), sequence))
		observing(t, engine, sequence, []byte("wait"), 0)
		root = drive(t, engine, &Command{Root: &Root{}}).Tree
		after, _ := root.Get(makeBasinKey([]byte("wait"), sequence))
		So(after, ShouldResemble, before)

		iterator := root.Root().Iterator()

		for _, value, found := iterator.Next(); found; _, value, found = iterator.Next() {
			So(len(value), ShouldEqual, WeightSize)
		}
	})
}

func TestEngineEvaluateClasses(t *testing.T) {
	Convey("Feasible class variants are never silently dropped at accumulator capacity", t, func() {
		engine := NewEngine(Config{})
		sequence := []byte("precursor")

		for index := range maxCandidates + 1 {
			observing(t, engine, sequence, []byte("action"+strconv.Itoa(index)), -1)
		}

		observing(t, engine, sequence, []byte("winner-after-scratch-capacity"), 1)
		So(evaluating(t, engine, sequence).WinnerClass, ShouldEqual, "winner-after-scratch-capacity")
	})
}

func TestEngineBackoffRecall(t *testing.T) {
	Convey("Suffix and prefix backoff answer contexts the exact key does not", t, func() {
		engine := NewEngine(Config{})

		for range 8 {
			observing(t, engine, []byte("A\x00B\x00C"), []byte("enter"))
		}

		observing(t, engine, []byte("A\x00B\x00C"), []byte("wait"))

		reading := evaluating(t, engine, []byte("A\x00B\x00C"))
		So(reading.WinnerClass, ShouldEqual, "enter")
		So(reading.RunnerUp, ShouldEqual, "wait")
		So(reading.Contrast, ShouldBeGreaterThan, 0)
		So(reading.Support, ShouldEqual, 8)

		if unseen := evaluating(t, engine, []byte("Z\x00A\x00B\x00C")); unseen.WinnerClass != "enter" {
			t.Fatalf("expected the suffix to answer, received %q", unseen.WinnerClass)
		}

		if foreign := evaluating(t, engine, []byte("X\x00Y")); foreign.WinnerClass != "" {
			t.Fatalf("expected no association, received %q", foreign.WinnerClass)
		}
	})
}

func TestEngineUncertaintyOnSparseEvidence(t *testing.T) {
	Convey("A single observation expresses uncertainty the prior reserves for it", t, func() {
		engine := NewEngine(Config{})

		observing(t, engine, []byte("CTX1"), []byte("enter_long"))

		reading := evaluating(t, engine, []byte("CTX1"))
		So(reading.WinnerClass, ShouldEqual, "enter_long")
		So(reading.Confidence, ShouldBeGreaterThan, 0)
		So(reading.Confidence, ShouldBeLessThan, 0.90)
		So(reading.Support, ShouldEqual, 1)
		So(reading.RunnerUp, ShouldEqual, "prior")

		for step := 2; step <= 20; step++ {
			observing(t, engine, []byte("CTX2"), []byte("enter_long"))
		}

		mature := evaluating(t, engine, []byte("CTX2"))
		So(mature.Confidence, ShouldBeGreaterThan, reading.Confidence)
		So(mature.Confidence, ShouldBeGreaterThanOrEqualTo, 0.70)
	})
}

func TestEngineBeamSearch(t *testing.T) {
	Convey("The lookahead extends observed sensory suffixes beyond the prefix", t, func() {
		engine := NewEngine(Config{})

		observing(t, engine, []byte("aa"), []byte("enter"))
		observing(t, engine, []byte("aab"), []byte("enter"))

		lookahead := evaluating(t, engine, []byte("aa")).Lookahead
		So(len(lookahead), ShouldBeGreaterThan, 0)
		So(lookahead[0].Sequence, ShouldEqual, "aab")
	})
}

func TestEngineObserveConcurrent(t *testing.T) {
	Convey("Parallel writers retain every association while inference reads immutable roots", t, func() {
		config := Config{MemoryScale: 1}
		engine := NewEngine(config)
		sequence := []byte("same-context")
		var workers sync.WaitGroup

		for range 4 {
			workers.Go(func() {
				for range 64 {
					observing(t, engine, sequence, []byte("enter"), 0.01)

					evaluation := transport.NewEvaluate(engine)
					command := Command{Evaluate: &Question{Context: sequence}}

					for range evaluation.Next(transport.NewOne(unsafe.Pointer(&command)).Next(nil)) {
					}

					if err := evaluation.Error(); err != nil {
						t.Error(err)
						return
					}
				}
			})
		}

		workers.Wait()

		root := drive(t, engine, &Command{Root: &Root{}}).Tree
		raw, found := root.Get(makeBasinKey([]byte("enter"), sequence))
		So(found, ShouldBeTrue)
		So(decodeWeight(raw).Count, ShouldEqual, 256)
		So(evaluating(t, engine, sequence).WinnerClass, ShouldEqual, "enter")
	})
}

func TestEngineCensus(t *testing.T) {
	Convey("The census counts each newly observed class once", t, func() {
		engine := NewEngine(Config{})

		observing(t, engine, []byte("ctx"), []byte("enter"))
		observing(t, engine, []byte("ctx"), []byte("enter"))
		observing(t, engine, []byte("ctx"), []byte("wait"))

		classes := drive(t, engine, &Command{Census: &Census{}}).Classes
		So(classes, ShouldResemble, map[string]int32{"enter": 1, "wait": 1})
	})
}

func BenchmarkEngineObserve(b *testing.B) {
	engine := NewEngine(Config{}).(*Engine)
	sequence, action := []byte("BTC/formation/ignition"), []byte("enter")
	b.ReportAllocs()

	for b.Loop() {
		observeForBenchmark(engine, sequence, action, 0.01)
	}
}

func BenchmarkEngineEvaluate(b *testing.B) {
	for _, size := range []int{32, 512} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			engine := NewEngine(Config{}).(*Engine)

			for index := range size {
				engine.observe(Association{
					Context:  []byte("BTC/precursor/" + strconv.Itoa(index)),
					Class:    []byte("enter"),
					Feedback: 0.01,
					Graded:   true,
				})
			}

			sequence := []byte("BTC/precursor/0")
			b.ReportAllocs()

			for b.Loop() {
				if _, err := engine.evaluate(sequence); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

/*
observeForBenchmark drives one association without the testing helper.
*/
func observeForBenchmark(engine *Engine, context, class []byte, feedback float64) {
	engine.observe(Association{Context: context, Class: class, Feedback: feedback, Graded: true})
}
