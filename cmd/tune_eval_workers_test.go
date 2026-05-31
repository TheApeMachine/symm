package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResolveTuneEvalWorkersFor(t *testing.T) {
	Convey("Given one tune worker", t, func() {
		workers := resolveTuneEvalWorkersFor(1, 16)

		Convey("It should give the eval the whole CPU budget", func() {
			So(workers, ShouldEqual, 16)
		})
	})

	Convey("Given many tune workers", t, func() {
		workers := resolveTuneEvalWorkersFor(8, 16)

		Convey("It should divide CPUs across eval subprocesses", func() {
			So(workers, ShouldEqual, 2)
		})
	})

	Convey("Given more tune workers than CPUs", t, func() {
		workers := resolveTuneEvalWorkersFor(32, 16)

		Convey("It should keep at least one CPU per eval", func() {
			So(workers, ShouldEqual, 1)
		})
	})
}

func BenchmarkResolveTuneEvalWorkersFor(b *testing.B) {
	for b.Loop() {
		_ = resolveTuneEvalWorkersFor(8, 16)
	}
}
