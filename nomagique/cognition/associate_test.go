package cognition_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestAssociate(t *testing.T) {
	Convey("Associate streams empirical precursor-class associations across transitions", t, func() {
		associate := cognition.NewAssociate()

		transOne := []byte("A->B")
		transTwo := []byte("B->C")
		transThree := []byte("C->D")

		in := func(yield func(unsafe.Pointer) bool) {
			if !yield(unsafe.Pointer(&transOne)) {
				return
			}

			if !yield(unsafe.Pointer(&transTwo)) {
				return
			}

			yield(unsafe.Pointer(&transThree))
		}

		var assocs []cognition.Association
		for out := range associate.Next(in) {
			assocs = append(assocs, *(*cognition.Association)(out))
		}

		So(associate.Error(), ShouldBeNil)
		So(len(assocs), ShouldEqual, 3)

		// First: sensory observation without class
		So(string(assocs[0].Context), ShouldEqual, "A->B")
		So(len(assocs[0].Class), ShouldEqual, 0)

		// Second: empirical transition A->B followed by B->C
		So(string(assocs[1].Context), ShouldEqual, "A->B")
		So(string(assocs[1].Class), ShouldEqual, "B->C")

		// Third: empirical transition B->C followed by C->D
		So(string(assocs[2].Context), ShouldEqual, "B->C")
		So(string(assocs[2].Class), ShouldEqual, "C->D")
	})
}

func TestCurrent(t *testing.T) {
	Convey("Current extracts the active evaluation context", t, func() {
		current := cognition.NewCurrent()

		assocSensory := cognition.Association{Context: []byte("A->B"), Class: nil}
		assocClass := cognition.Association{Context: []byte("A->B"), Class: []byte("B->C")}

		in := func(yield func(unsafe.Pointer) bool) {
			if !yield(unsafe.Pointer(&assocSensory)) {
				return
			}

			yield(unsafe.Pointer(&assocClass))
		}

		var contexts []string
		for out := range current.Next(in) {
			contexts = append(contexts, string(*(*[]byte)(out)))
		}

		So(current.Error(), ShouldBeNil)
		So(len(contexts), ShouldEqual, 2)
		So(contexts[0], ShouldEqual, "A->B")
		So(contexts[1], ShouldEqual, "B->C")
	})
}
