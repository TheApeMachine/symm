package response

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLatencyLoad(testingTB *testing.T) {
	Convey("Given a latency profile file", testingTB, func() {
		path := filepath.Join(testingTB.TempDir(), "latency.txt")

		So(os.WriteFile(path, []byte("25\n\n30\n"), 0o644), ShouldBeNil)

		latency := NewLatency().Load(path)

		Convey("It should load positive millisecond samples", func() {
			So(latency.Error(), ShouldBeNil)
			So(latency.timings, ShouldNotBeNil)
		})
	})

	Convey("Given a missing latency profile", testingTB, func() {
		latency := NewLatency().Load(filepath.Join(testingTB.TempDir(), "missing.txt"))

		Convey("It should report unreadable profile", func() {
			So(latency.Error(), ShouldNotBeNil)
		})
	})

	Convey("Given a JSON latency profile", testingTB, func() {
		path := filepath.Join(testingTB.TempDir(), "latency.json")

		So(os.WriteFile(path, []byte(`{"latencies":[22,28,25]}`), 0o644), ShouldBeNil)

		latency := NewLatency().Load(path)

		Convey("It should load positive millisecond samples", func() {
			So(latency.Error(), ShouldBeNil)
			So(latency.timings, ShouldNotBeNil)
		})
	})
}
