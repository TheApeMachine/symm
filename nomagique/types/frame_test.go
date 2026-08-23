package types

import (
	"math"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSymbolRegistryContracts(t *testing.T) {
	Convey("A symbol registry assigns stable identities", t, func() {
		registry := newSymbolRegistry(2)
		first, err := registry.intern("value")
		So(err, ShouldBeNil)
		second, err := registry.intern("value")
		So(err, ShouldBeNil)
		So(second, ShouldEqual, first)
		name, found := registry.name(first)
		So(found, ShouldBeTrue)
		So(name, ShouldEqual, "value")

		Convey("blank and whitespace-only names are rejected", func() {
			_, blankErr := registry.intern(" \t\n")
			So(blankErr, ShouldNotBeNil)
		})

		Convey("capacity is enforced without corrupting existing identities", func() {
			other, otherErr := registry.intern("other")
			So(otherErr, ShouldBeNil)
			So(other, ShouldNotEqual, first)
			_, fullErr := registry.intern("overflow")
			So(fullErr, ShouldNotBeNil)
			again, againErr := registry.intern("value")
			So(againErr, ShouldBeNil)
			So(again, ShouldEqual, first)
		})
	})

	Convey("Concurrent registration of one name cannot fork its identity", t, func() {
		registry := newSymbolRegistry(64)
		const workers = 64
		results := make(chan Symbol, workers)
		failures := make(chan error, workers)
		var wait sync.WaitGroup

		for range workers {
			wait.Add(1)
			go func() {
				defer wait.Done()
				symbol, err := registry.intern("shared")
				if err != nil {
					failures <- err
					return
				}
				results <- symbol
			}()
		}

		wait.Wait()
		close(results)
		close(failures)
		So(len(failures), ShouldEqual, 0)
		var expected Symbol
		initialized := false
		for symbol := range results {
			if !initialized {
				expected = symbol
				initialized = true
			}
			So(symbol, ShouldEqual, expected)
		}
		So(registry.registered(), ShouldEqual, 1)
	})
}

func TestFrameContracts(t *testing.T) {
	first := MustIntern("test/frame/first")
	second := MustIntern("test/frame/second")
	third := MustIntern("test/frame/third")

	Convey("A Frame is a copyable fixed-slot fact set", t, func() {
		frame := Frame{}
		frame.Put(first, 3)
		frame.Put(second, -0.0)

		value, found := frame.Get(first)
		So(found, ShouldBeTrue)
		So(value, ShouldEqual, 3.0)
		So(frame.Count(), ShouldEqual, 2)

		copy := frame
		copy.Put(first, 9)
		So(frame.MustGet(first), ShouldEqual, 3.0)
		So(copy.MustGet(first), ShouldEqual, 9.0)
	})

	Convey("Merge overlays only populated facts and Delete removes presence", t, func() {
		frame := Frame{}.Set(first, 1).Set(second, 2)
		overlay := Frame{}.Set(second, 20).Set(third, 30)
		frame.Merge(overlay)
		So(frame.MustGet(first), ShouldEqual, 1.0)
		So(frame.MustGet(second), ShouldEqual, 20.0)
		So(frame.MustGet(third), ShouldEqual, 30.0)
		frame.Delete(second)
		So(frame.Has(second), ShouldBeFalse)
		So(frame.Count(), ShouldEqual, 2)
	})

	Convey("All visits every populated slot in ascending symbol order", t, func() {
		frame := Frame{}.Set(third, 3).Set(first, 1).Set(second, 2)
		previous := -1
		visited := 0
		for symbol, value := range frame.All() {
			So(int(symbol), ShouldBeGreaterThan, previous)
			So(value, ShouldBeIn, 1.0, 2.0, 3.0)
			previous = int(symbol)
			visited++
		}
		So(visited, ShouldEqual, 3)
	})

	Convey("Finite actively rejects NaN and both infinities", t, func() {
		So(Frame{}.Set(first, 1).Finite(), ShouldBeTrue)
		So(Frame{}.Set(first, math.NaN()).Finite(), ShouldBeFalse)
		So(Frame{}.Set(first, math.Inf(1)).Finite(), ShouldBeFalse)
		So(Frame{}.Set(first, math.Inf(-1)).Finite(), ShouldBeFalse)
	})

	Convey("Equal compares presence and exact IEEE-754 representations", t, func() {
		positiveZero := Frame{}.Set(first, 0)
		negativeZero := Frame{}.Set(first, math.Copysign(0, -1))
		So(positiveZero.Equal(negativeZero), ShouldBeFalse)
		So(positiveZero.Equal(Frame{}.Set(first, 0)), ShouldBeTrue)
		So(positiveZero.Equal(Frame{}), ShouldBeFalse)
		nanBits := math.Float64frombits(0x7ff8000000000042)
		So(Frame{}.Set(first, nanBits).Equal(Frame{}.Set(first, nanBits)), ShouldBeTrue)
	})

	Convey("Out-of-capacity access cannot alias a valid slot", t, func() {
		invalid := Symbol(MaxSlots)
		frame := Frame{}.Set(first, 1)
		_, found := frame.Get(invalid)
		So(found, ShouldBeFalse)
		So(func() { frame.Delete(invalid) }, ShouldNotPanic)
		So(func() { frame.Put(invalid, 9) }, ShouldPanic)
		var nilFrame *Frame
		So(func() { nilFrame.Put(first, 1) }, ShouldPanic)
	})

	Convey("Hot-path Put and Get do not allocate", t, func() {
		frame := Frame{}
		allocations := testing.AllocsPerRun(1000, func() {
			frame.Put(first, 7)
			_, _ = frame.Get(first)
		})
		So(allocations, ShouldEqual, 0.0)
	})
}

func BenchmarkFrameGet(b *testing.B) {
	symbol := MustIntern("benchmark/frame/value")
	frame := Frame{}.Set(symbol, 7)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_, _ = frame.Get(symbol)
	}
}
