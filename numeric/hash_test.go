package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHashString(t *testing.T) {
	t.Parallel()

	Convey("HashString is deterministic for the same payload", t, func() {
		a, err := HashString("kadabra-field")

		So(err, ShouldBeNil)

		b, err := HashString("kadabra-field")

		So(err, ShouldBeNil)
		So(a, ShouldEqual, b)
	})

	Convey("HashString distinguishes nearby strings", t, func() {
		left, err := HashString("payload-a")

		So(err, ShouldBeNil)

		right, err := HashString("payload-b")

		So(err, ShouldBeNil)
		So(left, ShouldNotEqual, right)
	})
}

func BenchmarkHashString(b *testing.B) {
	const payload = "benchmark numeric hash string payload"

	var sink uint64

	var err error

	b.ResetTimer()

	for b.Loop() {
		sink, err = HashString(payload)

		if err != nil {
			b.Fatal(err)
		}
	}

	_ = sink
}
