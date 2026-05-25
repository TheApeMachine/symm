package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCompressionNext(t *testing.T) {
	t.Parallel()

	Convey("Given a Compression dynamic", t, func() {
		compression := NewCompression(0)

		Convey("It should score tighter spreads above one", func() {
			out, err := compression.Next(0, 10, 20)

			So(err, ShouldBeNil)
			So(out, ShouldBeGreaterThan, 1)
		})

		Convey("It should zero when current exceeds baseline", func() {
			out, err := compression.Next(0, 25, 20)

			So(err, ShouldBeNil)
			So(out, ShouldEqual, 0)
		})

		Convey("It should compare current to chained baseline", func() {
			out, err := compression.Next(20, 10)

			So(err, ShouldBeNil)
			So(out, ShouldBeGreaterThan, 1)
		})
	})
}
