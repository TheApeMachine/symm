package cognition

import (
	"strconv"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEngineObserve(t *testing.T) {
	Convey("A cognitive engine classifies the basin a context was observed in", t, func() {
		engine := NewEngine(DefaultConfig())
		context := []byte{0x01, 0x02, 0x03}

		engine.Observe(context, []byte("enter"))
		engine.Observe(context, []byte("enter"))
		engine.Observe(context, []byte("enter"))
		engine.Observe(context, []byte("wait"))

		evaluation := engine.Evaluate(context)
		So(evaluation.WinnerClass, ShouldEqual, "enter")
		So(evaluation.Confidence, ShouldBeGreaterThan, 0)
		So(evaluation.Surprisal, ShouldBeGreaterThanOrEqualTo, 0)
		So(evaluation.IsBreak, ShouldBeFalse)
	})
}

func TestEngineEvaluateUnseen(t *testing.T) {
	Convey("An unseen context carries Dirichlet-baseline surprisal", t, func() {
		engine := NewEngine(DefaultConfig())

		evaluation := engine.Evaluate([]byte{0x09, 0x08})
		So(evaluation.WinnerClass, ShouldEqual, "")
		So(evaluation.Surprisal, ShouldBeGreaterThan, 0)
		So(evaluation.IsBreak, ShouldBeTrue)
	})
}

func TestEngineEvaluateEmpty(t *testing.T) {
	Convey("An empty context cannot be matched and reports a break", t, func() {
		engine := NewEngine(DefaultConfig())

		evaluation := engine.Evaluate(nil)
		So(evaluation.IsBreak, ShouldBeTrue)
	})
}

func TestEngineObserveFeedback(t *testing.T) {
	Convey("Graded feedback inhibits bad actions and reinforces better ones in packed basins", t, func() {
		config := DefaultConfig()
		config.MemoryScale = 1 // Disable decay to isolate feedback in this fixture.
		engine := NewEngine(config)
		sequence := []byte("formation/ignition")
		engine.Observe(sequence, []byte("enter"), 1)
		engine.Observe(sequence, []byte("wait"), -1)
		So(engine.Evaluate(sequence).WinnerClass, ShouldEqual, "enter")
		for range 16 { // A changed outcome regime must reverse the preference.
			engine.Observe(sequence, []byte("enter"), -1)
			engine.Observe(sequence, []byte("wait"), 1)
		}
		So(engine.Evaluate(sequence).WinnerClass, ShouldEqual, "wait")
		before, _ := engine.root.Load().Get(makeBasinKey([]byte("wait"), sequence))
		engine.Observe(sequence, []byte("wait"), 0)
		after, _ := engine.root.Load().Get(makeBasinKey([]byte("wait"), sequence))
		So(after, ShouldResemble, before)
		iterator := engine.root.Load().Root().Iterator()
		for _, value, found := iterator.Next(); found; _, value, found = iterator.Next() {
			So(len(value), ShouldEqual, WeightSize)
		}
	})
}

func TestEngineEvaluateClasses(t *testing.T) {
	Convey("Feasible sizing variants are never silently dropped at scratch capacity", t, func() {
		engine := NewEngine(DefaultConfig())
		sequence := []byte("precursor")
		for index := range maxCandidates + 1 {
			engine.Observe(sequence, []byte("action"+strconv.Itoa(index)), -1)
		}
		engine.Observe(sequence, []byte("winner-after-scratch-capacity"), 1)
		So(engine.Evaluate(sequence).WinnerClass, ShouldEqual, "winner-after-scratch-capacity")
	})
}

func TestEngineObserveConcurrent(t *testing.T) {
	Convey("Parallel writers retain every association while inference reads immutable roots", t, func() {
		config := DefaultConfig()
		config.MemoryScale = 1
		engine := NewEngine(config)
		sequence := []byte("same-context")
		var workers sync.WaitGroup
		for range 4 {
			workers.Go(func() {
				for range 64 {
					engine.Observe(sequence, []byte("enter"), 0.01)
					engine.Evaluate(sequence)
				}
			})
		}
		workers.Wait()
		raw, found := engine.root.Load().Get(makeBasinKey([]byte("enter"), sequence))
		So(found, ShouldBeTrue)
		So(DecodeWeight(raw).Count, ShouldEqual, 256)
		So(engine.Evaluate(sequence).WinnerClass, ShouldEqual, "enter")
	})
}

func BenchmarkEngineObserve(b *testing.B) {
	engine := NewEngine(DefaultConfig())
	sequence, action := []byte("BTC/formation/ignition"), []byte("enter")
	b.ReportAllocs()
	for b.Loop() {
		engine.Observe(sequence, action, 0.01)
	}
}

func BenchmarkEngineEvaluate(b *testing.B) {
	for _, size := range []int{32, 512} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			engine := NewEngine(DefaultConfig())
			for index := range size {
				engine.Observe([]byte("BTC/precursor/"+strconv.Itoa(index)), []byte("enter"), 0.01)
			}
			sequence := []byte("BTC/precursor/0")
			b.ReportAllocs()
			for b.Loop() {
				engine.Evaluate(sequence)
			}
		})
	}
}
