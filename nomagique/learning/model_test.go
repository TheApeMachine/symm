package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestModelIssue(t *testing.T) {
	Convey("Given an ordered context and a parameterized action", t, func() {
		model := NewModel[string, [2]int]()
		context := []uint64{1, 2, 3}
		action := [2]int{1, 10}
		authority := 9.0 / 16
		identity, err := model.Issue("first", context, action, authority)
		So(err, ShouldBeNil)
		So(identity, ShouldNotEqual, 0)
		So(model.Recall("first", context, action), ShouldResemble, PriorReading{Pending: 1})

		Convey("input reuse cannot rewrite an issued context or its authority", func() {
			context[0] = 9
			authority = 0
			action[1] = 20
			reading, err := model.Resolve(identity, 6)
			So(err, ShouldBeNil)
			So(reading.Defined, ShouldBeTrue)
			So(reading.Mean, ShouldEqual, 6)
			So(model.Recall("first", []uint64{1, 2, 3}, [2]int{1, 10}), ShouldResemble, reading)
			So(model.Recall("first", context, action).Defined, ShouldBeFalse)
		})

		Convey("multiple matching pending actions retain separate identities", func() {
			second, err := model.Issue("first", context, action, authority)
			So(err, ShouldBeNil)
			So(second, ShouldNotEqual, identity)
			So(len(model.pending), ShouldEqual, 2)
			_, err = model.Resolve(second, 6)
			So(err, ShouldBeNil)
			reading, err := model.Resolve(identity, -2)
			So(err, ShouldBeNil)
			So(reading.Samples, ShouldEqual, 2)
			So(reading.Mean, ShouldEqual, 2)
			So(reading.Variance, ShouldEqual, 32)
			So(len(model.pending), ShouldEqual, 0)
		})

		Convey("invalid authority cannot register context or pending state", func() {
			sequence := model.sequence
			for _, invalid := range []float64{-0.1, 1.1} {
				_, err := model.Issue("new", context, action, invalid)
				So(err, ShouldNotBeNil)
			}
			So(model.sequence, ShouldEqual, sequence)
			So(len(model.contexts), ShouldEqual, 1)
			So(len(model.pending), ShouldEqual, 1)
		})

	})
}

func TestModelResolve(t *testing.T) {
	Convey("Given recurring contexts with outcomes observed later", t, func() {
		model := NewModel[int, int]()
		context := []uint64{3, 2, 1}
		authority := 9.0 / 16

		for _, value := range []float64{2, 4, 6, -4} {
			before := model.Recall(1, context, 1)
			identity, err := model.Issue(1, context, 1, authority)
			So(err, ShouldBeNil)
			before.Pending++
			So(model.Recall(1, context, 1), ShouldResemble, before)
			_, err = model.Resolve(identity, value)
			So(err, ShouldBeNil)
		}

		reading := model.Recall(1, context, 1)
		So(reading.Samples, ShouldEqual, 4)
		So(reading.Mean, ShouldEqual, 2)
		So(reading.Variance, ShouldAlmostEqual, 56.0/3)
		So(reading.Support, ShouldEqual, 4)
		So(len(model.pending), ShouldEqual, 0)

		Convey("unknown and already resolved actions cannot train twice", func() {
			for _, identity := range []uint64{0, model.sequence, model.sequence + 1} {
				_, err := model.Resolve(identity, 100)
				So(err, ShouldNotBeNil)
				So(model.Recall(1, context, 1), ShouldResemble, reading)
			}
		})

		Convey("lower quality outcomes have proportionally less influence", func() {
			// Four earlier outcomes each have authority 9/16. The new
			// outcome has authority 1/4, producing (4*9/16*2+1/4*12)/2.5 = 3.
			authority = 0.25
			identity, err := model.Issue(1, context, 1, authority)
			So(err, ShouldBeNil)
			reading, err := model.Resolve(identity, 12)
			So(err, ShouldBeNil)
			So(reading.Mean, ShouldAlmostEqual, 3)
			So(reading.Support, ShouldBeLessThan, 5)
		})
	})
}

func TestModelRecall(t *testing.T) {
	Convey("Given priors indexed by key, ordered context, and complete action", t, func() {
		model := NewModel[string, [2]int]()
		authority := 9.0 / 16
		contexts := [][]uint64{{1, 2, 3}, {3, 2, 1}, {1, 2}, nil}

		for index, context := range contexts {
			identity, err := model.Issue("first", context, [2]int{1, 10}, authority)
			So(err, ShouldBeNil)
			_, err = model.Resolve(identity, float64(index+1))
			So(err, ShouldBeNil)
		}

		Convey("a reliable shorter prefix outranks a single unrepeated exact match", func() {
			// {1,2,3} and {1,2} were each seen once, so neither can state a
			// dispersion of its own. Their shared prefix {1,2} carries both
			// outcomes and is the most specific answer that can.
			for _, context := range [][]uint64{{1, 2, 3}, {1, 2}, {1, 2, 3, 4}} {
				reading := model.Recall("first", context, [2]int{1, 10})
				So(reading.Depth, ShouldEqual, 2)
				So(reading.Samples, ShouldEqual, 2)
				So(reading.VarianceDefined, ShouldBeTrue)
			}
		})

		Convey("a context sharing no prefix falls back to unconditioned evidence", func() {
			// {3,2,1} shares nothing with the others past the empty context,
			// and its own chain was seen once, so the key's own evidence is
			// the only thing here that can state a spread.
			for _, context := range [][]uint64{{3, 2, 1}, {99}, nil} {
				reading := model.Recall("first", context, [2]int{1, 10})
				So(reading.Depth, ShouldEqual, 0)
				So(reading.Samples, ShouldEqual, 4)
			}
		})

		Convey("a prefix never issued on its own still carries what extends it", func() {
			prefix := model.Recall("first", []uint64{1}, [2]int{1, 10})
			So(prefix.Defined, ShouldBeTrue)
			So(prefix.Depth, ShouldEqual, 1)
			So(prefix.Samples, ShouldEqual, 2)
		})

		Convey("reordered tokens of an existing set match the canonical evidence", func() {
			reordered := model.Recall("first", []uint64{2, 1}, [2]int{1, 10})
			So(reordered.Defined, ShouldBeTrue)
			So(reordered.Depth, ShouldEqual, 2)
			So(reordered.Samples, ShouldEqual, 2)
			So(reordered.Mean, ShouldEqual, 2)
		})

		Convey("matching never reuses tokens beyond the former 32-bit bookkeeping limit", func() {
			longModel := NewModel[string, int]()
			learned := make([]uint64, 40)
			query := make([]uint64, len(learned))

			for index := range learned {
				learned[index] = 1
				query[index] = 2
			}

			query[32] = 1

			for _, outcome := range []float64{1, 2} {
				So(longModel.Observe("market", learned, 1, outcome, 1), ShouldBeNil)
			}

			So(longModel.Recall("market", query, 1).Depth, ShouldEqual, 1)
		})

		Convey("a different key or action shares no evidence at any depth", func() {
			for _, context := range contexts {
				So(model.Recall("second", context, [2]int{1, 10}).Defined, ShouldBeFalse)
				So(model.Recall("first", context, [2]int{2, 10}).Defined, ShouldBeFalse)
				So(model.Recall("first", context, [2]int{1, 20}).Defined, ShouldBeFalse)
			}
		})

		So(len(model.contexts), ShouldEqual, 1)

		Convey("different completed actions and parameters retain their own estimates", func() {
			for index, action := range [][2]int{{2, 10}, {1, 20}} {
				identity, err := model.Issue("first", contexts[0], action, authority)
				So(err, ShouldBeNil)
				_, err = model.Resolve(identity, float64(index+5))
				So(err, ShouldBeNil)
			}

			// Each action keeps its own estimate at every depth. The first
			// action has two observations along this chain and answers from
			// their shared prefix; the other two were seen once and answer
			// from their own exact context, which is all they have.
			So(model.Recall("first", contexts[0], [2]int{1, 10}).Mean, ShouldEqual, 2)
			So(model.Recall("first", contexts[0], [2]int{2, 10}).Mean, ShouldEqual, 5)
			So(model.Recall("first", contexts[0], [2]int{1, 20}).Mean, ShouldEqual, 6)
		})
	})
}

func TestModelRecallRetainedEvidence(t *testing.T) {
	Convey("Given a deep context aging while its broad context refreshes", t, func() {
		model := NewModel[string, int](8) // Eight-resolution retention fixture.
		deep := []uint64{1, 2, 3, 4, 5, 6, 7, 8}
		for _, outcome := range []float64{9, 11, 10} {
			So(model.Observe("market", deep, 1, outcome, 1), ShouldBeNil)
		}
		So(model.Recall("market", deep, 1).Depth, ShouldEqual, len(deep))
		for range 64 { // Eight fixture memory spans make the leaf dormant.
			So(model.Observe("market", []uint64{1, 2}, 1, 0, 1), ShouldBeNil)
		}
		reading := model.Recall("market", deep, 1)
		So(reading.Depth, ShouldEqual, 2)
		So(reading.Mean, ShouldBeLessThan, 0.01)
		So(reading.VarianceDefined, ShouldBeTrue)
		for range 64 {
			So(model.Observe("market", deep, 1, -5, 1), ShouldBeNil)
		}
		So(model.Recall("market", deep, 1).Depth, ShouldEqual, len(deep))
	})
}

func TestModelObserve(t *testing.T) {
	Convey("Given historical events to warm up the model", t, func() {
		model := NewModel[string, [2]int]()
		context := []uint64{10, 20, 30}
		action := [2]int{1, 5}

		So(model.Recall("test", context, action).Defined, ShouldBeFalse)

		err := model.Observe("test", context, action, 12.0, 0.8)
		So(err, ShouldBeNil)

		reading := model.Recall("test", context, action)
		So(reading.Defined, ShouldBeTrue)
		So(reading.Mean, ShouldEqual, 12.0)
		So(reading.Depth, ShouldEqual, 3)

		prefixReading := model.Recall("test", []uint64{10}, action)
		So(prefixReading.Defined, ShouldBeTrue)
		So(prefixReading.Mean, ShouldEqual, 12.0)
		So(prefixReading.Depth, ShouldEqual, 1)

		So(model.Observe("test", context, action, 0, -0.1), ShouldNotBeNil)
		So(model.Observe("test", context, action, 0, 1.5), ShouldNotBeNil)
	})
}

func BenchmarkModelResolve(b *testing.B) {
	model := NewModel[int, [2]int]()
	authority := 9.0 / 16
	// Match the 640 independent input contexts in the host's current workload.
	const keys = 640
	contexts := [][3]uint64{{1, 2, 3}, {3, 2, 1}, {2, 1, 3}}
	values := [...]float64{-2, 4, 6, -4}

	// Admit every key/context/action before measuring repeated decisions.
	for index := range keys * len(contexts) * len(values) {
		key := index % keys
		context := contexts[index/keys%len(contexts)][:]
		action := [2]int{index / (keys * len(contexts)), 10}
		identity, err := model.Issue(key, context, action, authority)

		if err != nil {
			b.Fatal(err)
		}

		if _, err := model.Resolve(identity, 0); err != nil {
			b.Fatal(err)
		}
	}

	index := 0
	b.ReportAllocs()

	for b.Loop() {
		key := index % keys
		context := contexts[index/keys%len(contexts)][:]
		action := [2]int{index / (keys * len(contexts)) % len(values), 10}
		identity, err := model.Issue(key, context, action, authority)

		if err != nil {
			b.Fatal(err)
		}

		if _, err := model.Resolve(identity, values[index%len(values)]); err != nil {
			b.Fatal(err)
		}

		model.Recall(key, context, action)
		index++
	}
}

func BenchmarkModelRecall(b *testing.B) {
	model := NewModel[string, int](8)
	context := []uint64{1, 2, 3, 4, 5, 6, 7, 8}
	for _, outcome := range []float64{-1, 2} {
		if err := model.Observe("market", context, 1, outcome, 1); err != nil {
			b.Fatal(err)
		}
	}
	query := []uint64{2, 1, 3, 4, 5, 6, 7, 8}
	b.ReportAllocs()
	for b.Loop() {
		model.Recall("market", query, 1)
	}
}
