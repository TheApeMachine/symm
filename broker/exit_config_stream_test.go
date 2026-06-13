package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestExitConfigStream(t *testing.T) {
	Convey("Given an immutable exit config stream", t, func() {
		stream, err := NewExitConfigStream(config.ExitConfig{})

		So(err, ShouldBeNil)
		defer func() { So(stream.Close(), ShouldBeNil) }()

		Convey("It should expose the initial snapshot", func() {
			snapshot := stream.Load()

			So(snapshot, ShouldResemble, config.ExitConfig{})
		})

		Convey("It should publish the latest snapshot synchronously", func() {
			next := config.ExitConfig{}

			So(stream.Publish(next), ShouldBeNil)
			So(stream.Load(), ShouldResemble, next)
		})
	})
}

func BenchmarkExitConfigStreamLoad(b *testing.B) {
	stream, err := NewExitConfigStream(config.ExitConfig{})

	if err != nil {
		b.Fatal(err)
	}

	defer func() { _ = stream.Close() }()

	b.ReportAllocs()

	for b.Loop() {
		snapshot := stream.Load()
		_ = snapshot
	}
}
