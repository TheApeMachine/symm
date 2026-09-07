package transport_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestDiscardNext(t *testing.T) {
	Convey("Given a discard operation", t, func() {
		discard := transport.NewDiscard()
		Convey("It drains opaque runs, including empty and repeated runs", func() {
			for range 3 {
				So(discard.Next(transport.NewIO(tests.NewOpaque(), tests.NewOpaque())), ShouldBeNil)
				So(discard.Next(nil), ShouldBeNil)
				So(discard.Error(), ShouldBeNil)
			}
		})
		Convey("It retains a source error reported on final nil", func() {
			marker := errors.New("terminal failure")
			var source *transport.Generator[float64]
			source = transport.NewGenerator(func(yield func(float64) bool) {
				yield(2)
				source.Error(marker)
			})
			So(discard.Next(source), ShouldBeNil)
			So(errors.Is(discard.Error(), marker), ShouldBeTrue)
		})
	})
}

func BenchmarkDiscardNext(b *testing.B) {
	discard := transport.NewDiscard()
	input := transport.NewIO(core.From(1.0), core.From(2.0), core.From(3.0))
	b.ReportAllocs()

	for b.Loop() {
		if discard.Next(input) != nil || discard.Error() != nil {
			b.Fatal("discard failed", discard.Error())
		}
	}
}
