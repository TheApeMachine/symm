package data

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReadout(t *testing.T) {
	Convey("Given a Readout observation", t, func() {
		now := time.Unix(1_700_000_000, 0)

		Convey("an immature observation carries zero authority", func() {
			readout := NewReadout("cvd", "signed_net_fraction", 0.8, 0.0, 10.0, true, true, now)

			So(readout.Authority(), ShouldEqual, 0.0)
			So(readout.Value(), ShouldEqual, 0.0)
		})

		Convey("an observation with undefined state carries zero authority", func() {
			readout := NewReadout("cvd", "signed_net_fraction", 0.8, 1.0, 10.0, true, true, now)
			readout.Defined = false

			So(readout.Authority(), ShouldEqual, 0.0)
			So(readout.Value(), ShouldEqual, 0.0)
		})

		Convey("stateless direct observation carries full authority", func() {
			readout := NewReadout("market", "price", 100.0, 1.0, 0.0, false, false, now)

			So(readout.Authority(), ShouldEqual, 1.0)
			So(readout.Value(), ShouldEqual, 100.0)
		})

		Convey("authority scales with maturity and SNR", func() {
			lowMaturity := NewReadout("cvd", "signed_net_fraction", 10.0, 0.2, 5.0, true, true, now)
			highMaturity := NewReadout("cvd", "signed_net_fraction", 10.0, 0.8, 5.0, true, true, now)

			So(lowMaturity.Authority(), ShouldBeLessThan, highMaturity.Authority())
			So(lowMaturity.Value(), ShouldBeLessThan, highMaturity.Value())
		})

		Convey("weak corroborating support provides weak corroboration", func() {
			base := NewReadout("cvd", "flow", 10.0, 0.5, 2.0, true, true, now)
			baseAuthority := base.Authority()

			weakSupport := NewReadout("hawkes", "excitation", 1.0, 0.1, 0.5, true, true, now)
			base.WithSupport(weakSupport)

			boostedAuthority := base.Authority()
			So(boostedAuthority, ShouldBeGreaterThan, baseAuthority)
			// Boost is modest because the support itself has low maturity.
			So(boostedAuthority-baseAuthority, ShouldBeLessThan, 0.1)
		})

		Convey("a mature high-quality contradiction substantially suppresses authority", func() {
			base := NewReadout("cvd", "flow", 10.0, 0.8, 5.0, true, true, now)
			baseAuthority := base.Authority()

			strongContradiction := NewReadout("toxicity", "retreat", -1.0, 1.0, 10.0, true, true, now)
			base.WithContradiction(strongContradiction)

			suppressedAuthority := base.Authority()
			So(suppressedAuthority, ShouldBeLessThan, baseAuthority*0.5)
			So(base.Value(), ShouldBeLessThan, 10.0*baseAuthority*0.5)
		})

		Convey("loss of credibility degrades authority", func() {
			base := NewReadout("cvd", "flow", 10.0, 0.8, 5.0, true, true, now)
			base.WithCredibility(0.2)

			So(base.Authority(), ShouldBeLessThan, 0.2)
		})

		Convey("CorroborateWith attaches supports and contradictions simultaneously", func() {
			base := NewReadout("cvd", "flow", 10.0, 0.8, 5.0, true, true, now)
			support := NewReadout("hawkes", "poisson_gain", 2.0, 0.9, 10.0, true, true, now)
			contradiction := NewReadout("morphology", "dislocation", 3.0, 0.9, 10.0, true, true, now)

			base.CorroborateWith([]*Readout{support}, []*Readout{contradiction})

			So(base.Supports, ShouldHaveLength, 1)
			So(base.Contradictions, ShouldHaveLength, 1)
			// Contradiction has 2x denominator weight, so authority is discounted despite support
			So(base.Authority(), ShouldBeLessThan, 0.8)
		})
	})
}
