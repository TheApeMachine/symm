package cognition_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestTrieNext(t *testing.T) {
	Convey("Trie reinforces associations into the radix trie and yields active context", t, func() {
		trie := cognition.NewTrie()

		assoc := cognition.Association{
			Context:  []byte("r1/r2"),
			Class:    []byte("enter"),
			Feedback: 1.0,
			Graded:   true,
		}

		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&assoc))
		}

		var contexts []string
		for out := range trie.Next(in) {
			contexts = append(contexts, string(*(*[]byte)(out)))
		}

		So(trie.Error(), ShouldBeNil)
		So(len(contexts), ShouldEqual, 1)
		So(contexts[0], ShouldEqual, "enter")
		So(trie.StepCounter.Load(), ShouldEqual, 1)

		tree := trie.Root.Load()
		basinVal, foundBasin := tree.Get([]byte("b/r1/r2/enter"))
		So(foundBasin, ShouldBeTrue)

		weightDecoder := cognition.NewWeight()
		inB := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&basinVal))
		}
		var unpacked cognition.PackedWeight
		for out := range weightDecoder.Next(inB) {
			unpacked = *(*cognition.PackedWeight)(out)
		}
		So(unpacked.Count, ShouldEqual, 1)
		So(unpacked.Mass, ShouldEqual, 1.0)

		sensoryVal, foundSensory := tree.Get([]byte("s/r1/r2"))
		So(foundSensory, ShouldBeTrue)

		inS := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&sensoryVal))
		}
		var unpackedSensory cognition.PackedWeight
		for out := range weightDecoder.Next(inS) {
			unpackedSensory = *(*cognition.PackedWeight)(out)
		}
		So(unpackedSensory.Count, ShouldEqual, 1)
		So(unpackedSensory.Mass, ShouldEqual, 1.0)
	})

	Convey("Trie yields sensory context when class is absent", t, func() {
		trie := cognition.NewTrie()

		sensoryAssoc := cognition.Association{
			Context: []byte("initial_context"),
			Class:   nil,
		}

		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&sensoryAssoc))
		}

		var contexts []string
		for out := range trie.Next(in) {
			contexts = append(contexts, string(*(*[]byte)(out)))
		}

		So(trie.Error(), ShouldBeNil)
		So(len(contexts), ShouldEqual, 1)
		So(contexts[0], ShouldEqual, "initial_context")
	})

	Convey("negative feedback inhibits mass so +1 followed by -1 cancels", t, func() {
		trie := cognition.NewTrie()

		posAssoc := cognition.Association{
			Context:  []byte("ctx1"),
			Class:    []byte("enter"),
			Feedback: 1.0,
			Graded:   true,
		}

		negAssoc := cognition.Association{
			Context:  []byte("ctx1"),
			Class:    []byte("enter"),
			Feedback: -1.0,
			Graded:   true,
		}

		in := func(yield func(unsafe.Pointer) bool) {
			if !yield(unsafe.Pointer(&posAssoc)) {
				return
			}

			yield(unsafe.Pointer(&negAssoc))
		}

		for range trie.Next(in) {
		}

		So(trie.StepCounter.Load(), ShouldEqual, 2)

		tree := trie.Root.Load()
		basinVal, foundBasin := tree.Get([]byte("b/ctx1/enter"))
		So(foundBasin, ShouldBeTrue)

		weightDecoder := cognition.NewWeight()
		inB := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&basinVal))
		}

		var unpacked cognition.PackedWeight
		for out := range weightDecoder.Next(inB) {
			unpacked = *(*cognition.PackedWeight)(out)
		}

		So(unpacked.Count, ShouldEqual, 2)
		So(unpacked.Mass, ShouldEqual, 0.0)
	})

	Convey("CAS contention advances logical observation count once per commit", t, func() {
		trie := cognition.NewTrie()

		const concurrentWriters = 8
		done := make(chan struct{}, concurrentWriters)

		for i := 0; i < concurrentWriters; i++ {
			go func(writerID int) {
				defer func() { done <- struct{}{} }()

				writerAssoc := cognition.Association{
					Context:  []byte("ctx"),
					Class:    []byte("enter"),
					Feedback: 1.0,
					Graded:   true,
				}

				in := func(yield func(unsafe.Pointer) bool) {
					yield(unsafe.Pointer(&writerAssoc))
				}

				for range trie.Next(in) {
				}
			}(i)
		}

		for i := 0; i < concurrentWriters; i++ {
			<-done
		}

		So(trie.StepCounter.Load(), ShouldEqual, uint64(concurrentWriters))
	})
}
