package config

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestSyncPerspectives(t *testing.T) {
	convey.Convey("Given a mutated noise floor", t, func() {
		original := System.NoiseFloorSNR
		t.Cleanup(func() {
			System.NoiseFloorSNR = original
			SyncRuntime()
		})

		System.NoiseFloorSNR = 1.25
		SyncRuntime()

		convey.Convey("It should push the threshold into perspectives", func() {
			convey.So(perspectives.NoiseFloorSNR(), convey.ShouldEqual, 1.25)
		})
	})
}
