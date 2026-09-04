package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/* counter is a minimal stateful node: it totals what it is stepped with. */
type counter struct{ total Scalar }

func (node *counter) Step(x Scalar) Scalar {
	node.total += x

	return node.total
}

func TestKeyed(t *testing.T) {
	Convey("Given a Keyed node over two keys", t, func() {
		key := "a"
		keyed := &Keyed{
			Build:  func() Node { return &counter{} },
			Select: func() string { return key },
		}

		Convey("each key accumulates independently", func() {
			So(keyed.Step(Scalar(2)), ShouldEqual, Scalar(2))
			So(keyed.Step(Scalar(3)), ShouldEqual, Scalar(5))

			key = "b"
			So(keyed.Step(Scalar(10)), ShouldEqual, Scalar(10))

			key = "a"
			So(keyed.Step(Scalar(1)), ShouldEqual, Scalar(6))
		})

		Convey("a branch is constructed once per key", func() {
			keyed.Step(Scalar(1))
			keyed.Step(Scalar(1))
			So(keyed.Len(), ShouldEqual, 1)

			key = "b"
			keyed.Step(Scalar(1))
			So(keyed.Len(), ShouldEqual, 2)
		})

		Convey("Active exposes the branch the last Step routed to", func() {
			keyed.Step(Scalar(4))
			active, ok := keyed.Active().(*counter)
			So(ok, ShouldBeTrue)
			So(active.total, ShouldEqual, Scalar(4))
		})
	})

	Convey("Given a Keyed node with no Build or Select", t, func() {
		keyed := &Keyed{}

		Convey("it is the identity and routes nowhere", func() {
			So(keyed.Step(Scalar(7)), ShouldEqual, Scalar(7))
			So(keyed.Active(), ShouldBeNil)
			So(keyed.Len(), ShouldEqual, 0)
		})
	})
}
