package transport_test

import (
	"context"
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/data/transport"
)

func TestStream(t *testing.T) {
	ctx := context.Background()

	Convey("Stream as io.ReadWriteCloser", t, func() {
		Convey("passes Value through io.Copy and applies transformation fn", func() {
			val := data.NewValue("test", "tick", "agg")
			val.Append([]byte("hello"))

			modified := false
			s := transport.NewStream(ctx, func(v *data.Value) *data.Value {
				modified = true
				v.Append([]byte("world"))
				return v
			})

			// 1. Copy Value into Stream
			n, err := io.Copy(s, val)
			So(err, ShouldBeNil)
			So(n, ShouldBeGreaterThan, 0)

			// 2. Copy out of Stream into another Value
			out := data.New()
			nOut, err := io.Copy(out, s)
			So(err, ShouldBeNil)
			So(nOut, ShouldBeGreaterThan, 0)
			So(modified, ShouldBeTrue)
			So(len(out.Layers), ShouldEqual, 2)
		})

		Convey("produces Values when acting as a source via fn(nil)", func() {
			count := 0
			source := transport.NewStream(ctx, func(v *data.Value) *data.Value {
				if count >= 3 {
					return nil
				}
				count++
				val := data.NewValue("source", "tick", "raw")
				val.Append([]byte("event"))
				return val
			})

			for range 3 {
				out := data.New()
				n, err := io.Copy(out, source)
				So(err, ShouldBeNil)
				So(n, ShouldBeGreaterThan, 0)
				So(out.Origin, ShouldEqual, "source")
			}

			// 4th read returns EOF
			out := data.New()
			n, err := io.Copy(out, source)
			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, 0)
		})
	})
}
