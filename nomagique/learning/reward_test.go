package learning

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRewardLedgerMeasure(t *testing.T) {
	Convey("Given a cumulative numerical objective", t, func() {
		ledger := &RewardLedger{}
		at := time.Unix(1, 0)
		initial := RewardMark{At: at, Version: 1, Value: 100}
		first, err := ledger.Measure(initial)
		So(err, ShouldBeNil)
		So(first.HasRate, ShouldBeFalse)
		So(first.Transitions, ShouldEqual, 0)

		Convey("increases, decreases and recovery retain their signed rewards", func() {
			previous := initial.Value

			for index, value := range []float64{110, 80, 100} {
				outcome, err := ledger.Measure(RewardMark{
					At:      at.Add(time.Duration(index+1) * time.Second),
					Version: uint64(index + 2), Value: value,
				})
				So(err, ShouldBeNil)
				So(outcome.Reward, ShouldEqual, value-previous)
				previous = value
			}

			So(ledger.outcome.TotalReward, ShouldEqual, 0)
			So(ledger.outcome.TotalElapsed, ShouldEqual, 3*time.Second)
		})

		Convey("unchanged values compare only with the previously observed rate", func() {
			gain, err := ledger.Measure(RewardMark{At: at.Add(time.Second), Version: 2, Value: 110})
			So(err, ShouldBeNil)
			So(gain.HasPriorRate, ShouldBeFalse)
			So(gain.Rate, ShouldEqual, 10)
			unchanged, err := ledger.Measure(RewardMark{At: at.Add(3 * time.Second), Version: 3, Value: 110})
			So(err, ShouldBeNil)
			So(unchanged.Reward, ShouldEqual, 0)
			So(unchanged.PriorRate, ShouldEqual, 10)
			So(unchanged.Differential, ShouldEqual, -20)
			So(unchanged.Rate, ShouldAlmostEqual, 10.0/3)
		})

		Convey("a constant sequence has an observed zero rate", func() {
			unchanged, err := ledger.Measure(RewardMark{At: at.Add(time.Hour), Version: 2, Value: 100})
			So(err, ShouldBeNil)
			So(unchanged.Reward, ShouldEqual, 0)
			So(unchanged.HasRate, ShouldBeTrue)
			So(unchanged.Rate, ShouldEqual, 0)
		})

		Convey("negative values and rewards remain distinct from relative feedback", func() {
			_, err := ledger.Measure(RewardMark{At: at.Add(time.Second), Version: 2, Value: 0})
			So(err, ShouldBeNil)
			outcome, err := ledger.Measure(RewardMark{At: at.Add(2 * time.Second), Version: 3, Value: -5})
			So(err, ShouldBeNil)
			So(outcome.Reward, ShouldEqual, -5)
			So(outcome.TotalReward, ShouldEqual, -105)
			So(outcome.Differential, ShouldEqual, 95)
		})

		Convey("same-instant revisions change reward without inventing elapsed time", func() {
			revision, err := ledger.Measure(RewardMark{At: at, Version: 2, Value: 99})
			So(err, ShouldBeNil)
			So(revision.Reward, ShouldEqual, -1)
			So(revision.HasRate, ShouldBeFalse)
			later, err := ledger.Measure(RewardMark{At: at.Add(time.Second), Version: 3, Value: 99})
			So(err, ShouldBeNil)
			So(later.Rate, ShouldEqual, -1)
		})

		Convey("missing intermediate samples preserve total reward", func() {
			outcome, err := ledger.Measure(RewardMark{At: at.Add(3 * time.Second), Version: 5, Value: 115})
			So(err, ShouldBeNil)
			So(outcome.Reward, ShouldEqual, 15)
			So(outcome.Transitions, ShouldEqual, 1)
			So(outcome.Rate, ShouldEqual, 5)
		})

		Convey("invalid and rewritten samples cannot mutate prior evidence", func() {
			before := *ledger

			for _, invalid := range []RewardMark{
				{}, {At: at}, {Version: 2},
				{At: at, Version: 1, Value: 101},
				{At: at.Add(-time.Second), Version: 2, Value: 100},
			} {
				_, err := ledger.Measure(invalid)
				So(err, ShouldNotBeNil)
				So(*ledger, ShouldResemble, before)
			}
		})

		Convey("an identical timestamp in another zone is the same observation", func() {
			redelivered := initial
			redelivered.At = initial.At.In(time.FixedZone("other", 3600))
			outcome, err := ledger.Measure(redelivered)
			So(err, ShouldBeNil)
			So(outcome, ShouldResemble, first)
		})

		Convey("repeated delivery cannot count a reward twice", func() {
			mark := RewardMark{At: at.Add(time.Second), Version: 2, Value: 110}
			first, err := ledger.Measure(mark)
			So(err, ShouldBeNil)
			repeated, err := ledger.Measure(mark)
			So(err, ShouldBeNil)
			So(repeated, ShouldResemble, first)
		})
	})

	Convey("Given different reporting frequencies and reference offsets", t, func() {
		at := time.Unix(1, 0)
		coarse, fine := &RewardLedger{}, &RewardLedger{}
		_, err := coarse.Measure(RewardMark{At: at, Version: 1, Value: -100})
		So(err, ShouldBeNil)
		_, err = fine.Measure(RewardMark{At: at, Version: 1, Value: 100})
		So(err, ShouldBeNil)

		// Unequal durations distinguish a cumulative rate from a mean of rates.
		durations := []time.Duration{time.Second, 3 * time.Second, 10 * time.Second}

		for index, value := range []float64{110, 90, 130} {
			_, err := fine.Measure(RewardMark{
				At: at.Add(durations[index]), Version: uint64(index + 2), Value: value,
			})
			So(err, ShouldBeNil)
		}

		_, err = coarse.Measure(RewardMark{At: at.Add(10 * time.Second), Version: 4, Value: -70})
		So(err, ShouldBeNil)
		So(coarse.outcome.TotalReward, ShouldEqual, fine.outcome.TotalReward)
		So(coarse.outcome.Rate, ShouldEqual, fine.outcome.Rate)
		So(coarse.outcome.Rate, ShouldEqual, 3)
	})
}

func BenchmarkRewardLedgerMeasure(b *testing.B) {
	ledger := &RewardLedger{}
	at := time.Unix(1, 0)
	version := uint64(0)
	// The repeated fixture spans positive, negative and unchanged differences.
	values := [...]float64{100, 110, 80, 80, 100}
	b.ReportAllocs()

	for b.Loop() {
		version++
		at = at.Add(time.Second)

		if _, err := ledger.Measure(RewardMark{
			At: at, Version: version, Value: values[(version-1)%uint64(len(values))],
		}); err != nil {
			b.Fatal(err)
		}
	}
}
