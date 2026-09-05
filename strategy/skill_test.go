package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestSkillMeterAuthority(t *testing.T) {
	at := time.Unix(1000, 0)

	Convey("Given a meter attached to a real account", t, func() {
		meter := NewSkillMeter(AccountReal, at)

		Convey("It starts learning-only with no evidence", func() {
			reading := meter.Reading()
			So(meter.Mode(), ShouldEqual, ModeLearning)
			So(reading.Defined, ShouldBeFalse)
			So(reading.VarianceDefined, ShouldBeFalse)
			So(reading.Account, ShouldEqual, "real")
		})

		Convey("One outcome defines a mean but cannot justify authority", func() {
			meter.Observe(0.01, 1, at)
			reading := meter.Reading()
			So(reading.Defined, ShouldBeTrue)
			So(reading.VarianceDefined, ShouldBeFalse)
			So(meter.Mode(), ShouldEqual, ModeLearning)
			So(reading.Reason, ShouldEqual, "dispersion is not estimable yet")
		})

		Convey("A handful of identical outcomes cannot read as certainty", func() {
			for index := range 4 {
				meter.Observe(0.01, 1, at.Add(time.Duration(index)*time.Second))
			}

			reading := meter.Reading()
			So(reading.VarianceDefined, ShouldBeTrue)
			So(reading.Support, ShouldBeLessThanOrEqualTo, skillSigma*skillSigma)
			So(meter.Mode(), ShouldEqual, ModeLearning)
			So(reading.Reason, ShouldEqual, "effective evidence is thinner than the confidence bound assumes")
		})

		Convey("A noisy but genuine edge starts trading the configured account", func() {
			seen := []Mode{}

			// Alternating magnitudes give a real dispersion to measure, so the
			// bound has to be cleared rather than handed over by a degenerate
			// run of identical outcomes.
			for index := range 64 {
				target := 0.010

				if index%2 == 0 {
					target = 0.014
				}

				meter.Observe(target, 1, at.Add(time.Duration(index)*time.Second))
				seen = append(seen, meter.Mode())
			}

			So(meter.Reading().LowerBound, ShouldBeGreaterThan, 0)
			So(meter.Mode(), ShouldEqual, ModeTrading)
			So(meter.Reading().Promotions, ShouldEqual, 1)

			// Where it trades is configuration, not something it earned: the
			// account never changes, only whether it is being traded.
			So(meter.Reading().Account, ShouldEqual, "real")

			live := -1

			for index, mode := range seen {
				if mode == ModeTrading && live < 0 {
					live = index
				}
			}

			So(live, ShouldBeGreaterThan, 0)
			So(seen[live-1], ShouldEqual, ModeLearning)
		})

		Convey("An edge inside its own measurement error cannot promote", func() {
			for index := range 32 {
				target := 0.01

				if index%2 == 0 {
					target = -0.0099
				}

				meter.Observe(target, 1, at.Add(time.Duration(index)*time.Second))
			}

			reading := meter.Reading()
			So(reading.Mean, ShouldBeGreaterThan, 0)
			So(reading.LowerBound, ShouldBeLessThan, 0)
			So(meter.Mode(), ShouldEqual, ModeLearning)
			So(reading.Reason, ShouldEqual, "edge is positive but inside its measurement error")
		})
	})

	Convey("Given a trading agent whose edge disappears", t, func() {
		meter := NewSkillMeter(AccountReal, at)

		for index := range 16 {
			meter.Observe(0.01, 1, at.Add(time.Duration(index)*time.Second))
		}

		So(meter.Mode(), ShouldEqual, ModeTrading)

		Convey("A non-positive mean falls back without needing to clear a bound", func() {
			for index := range 2048 {
				meter.Observe(-0.02, 1, at.Add(time.Duration(index)*time.Minute))
			}

			So(meter.Reading().Mean, ShouldBeLessThan, 0)
			So(meter.Mode(), ShouldEqual, ModeLearning)
			So(meter.Reading().Demotions, ShouldEqual, 1)
		})
	})

}

func TestSkillMeterForgets(t *testing.T) {
	at := time.Unix(1000, 0)

	Convey("Given competence that was earned in an earlier regime", t, func() {
		meter := NewSkillMeter(AccountPaper, at)

		for index := range 256 {
			meter.Observe(0.02, 1, at.Add(time.Duration(index)*time.Second))
		}

		So(meter.Mode(), ShouldEqual, ModeTrading)

		Convey("Recent losses outweigh a stale edge instead of averaging into it", func() {
			for index := range 1024 {
				meter.Observe(-0.02, 1, at.Add(time.Duration(index)*time.Minute))
			}

			So(meter.Reading().Mean, ShouldBeLessThan, 0)
			So(meter.Mode(), ShouldEqual, ModeLearning)
		})

		Convey("Support stays bounded by the declared retention window", func() {
			for index := range 4096 {
				meter.Observe(0.02, 1, at.Add(time.Duration(index)*time.Minute))
			}

			// Exponential weights at rate 1/memory settle at about twice the
			// retention figure in Kish effective size, and never accumulate
			// past it however long the agent runs.
			So(meter.Reading().Support, ShouldBeLessThanOrEqualTo, 2*skillMemory+1)
		})
	})
}

func TestAttributionCreditsPresentQuantities(t *testing.T) {
	Convey("Given resolved outcomes under two different hot quantities", t, func() {
		store := &attribution{}
		columns := [][2]string{{"hawkes", "Intensity"}, {"cvd", "Divergence"}}

		So(store.observe([]uint64{1}, types.ActionEnter, 0.05, 1), ShouldBeNil)
		So(store.observe([]uint64{1}, types.ActionEnter, 0.03, 1), ShouldBeNil)
		So(store.observe([]uint64{2}, types.ActionEnter, -0.04, 1), ShouldBeNil)
		So(store.observe([]uint64{2}, types.ActionEnter, -0.02, 1), ShouldBeNil)

		report := store.report(columns)

		Convey("Each quantity carries its own evidence, named by its column", func() {
			So(len(report), ShouldEqual, 2)

			byToken := map[uint64]MetricInfluence{}
			for _, entry := range report {
				byToken[entry.Token] = entry
			}

			So(byToken[1].Source, ShouldEqual, "hawkes")
			So(byToken[1].Label, ShouldEqual, "Intensity")
			So(byToken[1].Prior.Mean, ShouldBeGreaterThan, 0)
			So(byToken[2].Source, ShouldEqual, "cvd")
			So(byToken[2].Prior.Mean, ShouldBeLessThan, 0)
		})

		Convey("A token with no matching column is reported without inventing one", func() {
			So(store.observe([]uint64{99}, types.ActionExit, 0.01, 1), ShouldBeNil)
			for _, entry := range store.report(columns) {
				if entry.Token == 99 {
					So(entry.Source, ShouldEqual, "")
					So(entry.Label, ShouldEqual, "")
				}
			}
		})
	})
}

func TestSkillNeedsAnAttachedAccount(t *testing.T) {
	at := time.Unix(1000, 0)

	Convey("Given a measured edge but no account attached", t, func() {
		meter := NewSkillMeter(AccountNone, at)

		for index := range 64 {
			target := 0.010

			if index%2 == 0 {
				target = 0.014
			}

			meter.Observe(target, 1, at.Add(time.Duration(index)*time.Second))
		}

		reading := meter.Reading()

		Convey("It stays calibrating, however good the measurement looks", func() {
			So(reading.Qualified, ShouldBeTrue)
			So(reading.LowerBound, ShouldBeGreaterThan, 0)
			So(meter.Mode(), ShouldEqual, ModeLearning)
			So(reading.Account, ShouldEqual, "none")
			So(reading.Reason, ShouldEqual, "measured edge is positive but no account is attached")
		})
	})
}
