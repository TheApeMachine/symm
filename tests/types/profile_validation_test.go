package types

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateProfiles(t *testing.T) {
	Convey("Given the complete default regime contract", t, func() {
		profiles := CloneProfiles(DefaultProfiles)
		momentum := CloneMomentum(MomentumMap)

		Convey("Every declared regime should be physically valid", func() {
			So(validateProfiles(profiles, momentum), ShouldBeNil)
		})

		Convey("A non-finite distribution parameter should be rejected", func() {
			profile := profiles[RandomWalk]
			profile.Diffusion = math.NaN()
			profiles[RandomWalk] = profile

			So(validateProfiles(profiles, momentum), ShouldNotBeNil)
		})

		Convey("A missing transition speed should be rejected", func() {
			delete(momentum, FastPump)

			So(validateProfiles(profiles, momentum), ShouldNotBeNil)
		})
	})
}
