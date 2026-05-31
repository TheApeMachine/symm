package config

import (
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTunablesSearchObserve(t *testing.T) {
	Convey("Given reward feedback", t, func() {
		search := NewTunablesSearch(NewConfig(), rand.New(rand.NewSource(1)))
		first := MutateTunables(NewConfig(), rand.New(rand.NewSource(2)))
		second := MutateTunables(NewConfig(), rand.New(rand.NewSource(3)))

		search.Observe(first, 4)
		search.Observe(second, 9)
		search.Observe(first, 6)

		Convey("It should keep the highest holdout reward", func() {
			So(search.BestReward(), ShouldEqual, 9)
		})
	})
}

func TestMutateTunablesNear(t *testing.T) {
	Convey("Given a base overlay", t, func() {
		base := ExtractTunables(NewConfig())
		near := MutateTunablesNear(base, rand.New(rand.NewSource(3)))

		Convey("It should keep most fields within spec bounds", func() {
			So(near.EntryEdgeMultiple, ShouldNotBeNil)
			So(*near.EntryEdgeMultiple, ShouldBeGreaterThanOrEqualTo, 1.0)
			So(*near.EntryEdgeMultiple, ShouldBeLessThanOrEqualTo, 4.0)
		})
	})
}
