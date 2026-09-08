package reward

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLedgerMeasure(t *testing.T) {
	Convey("Given a cumulative objective with losses, recovery and same-instant updates", t, func() {
		ledger := &Ledger{}
		start := time.Unix(1788795593, 0)
		marks := []Mark{
			{At: start, Version: 1, Value: 100},
			{At: start.Add(time.Second), Version: 2, Value: 110},
			{At: start.Add(3 * time.Second), Version: 3, Value: 90},
			{At: start.Add(3 * time.Second), Version: 3, Value: 90},
			{At: start.Add(3 * time.Second), Version: 4, Value: 95},
			{At: start.Add(10 * time.Second), Version: 5, Value: 130},
		}
		expected := [...]float64{0, 10, -20, -20, 5, 35}
		var outcome Outcome

		for index, mark := range marks {
			var err error
			outcome, err = ledger.Measure(mark)
			So(err, ShouldBeNil)
			So(outcome.Reward, ShouldEqual, expected[index])
			So(outcome.TotalReward, ShouldEqual, mark.Value-100)
			So(outcome.Through.At.Equal(mark.At), ShouldBeTrue)
		}

		So(outcome.Transitions, ShouldEqual, 4)
		So(outcome.Rate, ShouldEqual, 3)
		So(outcome.TotalElapsed, ShouldEqual, 10*time.Second)
		So(outcome.Differential, ShouldAlmostEqual, 35+35.0/3)

		Convey("Invalid marks leave the ledger unchanged and the next valid mark still resolves", func() {
			previous := *ledger

			for _, mark := range []Mark{
				{At: outcome.Through.At, Version: 0, Value: 130},
				{At: outcome.Through.At, Version: 4, Value: 130},
				{At: outcome.Through.At, Version: 5, Value: 131},
				{At: outcome.Through.At.Add(time.Nanosecond), Version: 5, Value: 130},
				{At: outcome.Through.At.Add(-time.Nanosecond), Version: 6, Value: 130},
			} {
				_, err := ledger.Measure(mark)
				So(err, ShouldNotBeNil)
				So(*ledger, ShouldResemble, previous)
			}

			next, err := ledger.Measure(Mark{At: start.Add(11 * time.Second), Version: 6, Value: 120})
			So(err, ShouldBeNil)
			So(next.Reward, ShouldEqual, -10)
			So(next.Differential, ShouldEqual, -13)
		})
	})

	Convey("Given high version identities and nanosecond-separated modern timestamps", t, func() {
		ledger := &Ledger{}
		start := time.Now()
		version := uint64(1) << 63
		_, err := ledger.Measure(Mark{At: start, Version: version, Value: 200})
		So(err, ShouldBeNil)
		So(ledger.initial.At == start, ShouldBeTrue)

		for index, value := range []float64{210, 190, 220} {
			at := start.Add(time.Duration(index+1) * time.Nanosecond)
			outcome, err := ledger.Measure(Mark{At: at, Version: version + uint64(index+1), Value: value})
			So(err, ShouldBeNil)
			So(ledger.last.At == at, ShouldBeTrue)
			So(outcome.Elapsed, ShouldEqual, time.Nanosecond)
			So(outcome.Rate, ShouldAlmostEqual, (value-200)/at.Sub(start).Seconds())
		}
	})
}

func BenchmarkLedgerMeasure(b *testing.B) {
	ledger := &Ledger{}
	mark := Mark{At: time.Unix(1788795593, 0)}
	values := [...]float64{100, 110, 80, 130, 100}
	b.ReportAllocs()

	for b.Loop() {
		mark.At = mark.At.Add(time.Millisecond)
		mark.Version++
		mark.Value = values[mark.Version%uint64(len(values))]

		if _, err := ledger.Measure(mark); err != nil {
			b.Fatal(err)
		}
	}
}
