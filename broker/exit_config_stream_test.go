package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestExitConfigStream(t *testing.T) {
	Convey("Given an immutable exit config stream", t, func() {
		stream, err := NewExitConfigStream(config.ExitConfig{
			TrailDefault: 0.015,
			StopFloor:    0.012,
			SpreadScale:  0.5,
		})

		So(err, ShouldBeNil)
		defer func() { So(stream.Close(), ShouldBeNil) }()

		Convey("It should expose the initial snapshot", func() {
			snapshot := stream.Load()

			So(snapshot.TrailDefault, ShouldEqual, 0.015)
			So(snapshot.StopFloor, ShouldEqual, 0.012)
			So(snapshot.SpreadScale, ShouldEqual, 0.5)
		})

		Convey("It should publish the latest snapshot synchronously", func() {
			next := config.ExitConfig{
				TrailDefault: 0.02,
				StopFloor:    0.018,
				SpreadScale:  0.8,
			}

			So(stream.Publish(next), ShouldBeNil)
			So(stream.Load(), ShouldResemble, next)
		})
	})
}

func BenchmarkExitConfigStreamLoad(b *testing.B) {
	stream, err := NewExitConfigStream(config.ExitConfig{
		TrailDefault: 0.015,
		StopFloor:    0.012,
		SpreadScale:  0.5,
	})

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
