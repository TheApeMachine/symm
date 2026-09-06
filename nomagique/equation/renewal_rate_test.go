package equation_test

import (
	"errors"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestNewRenewalRate(t *testing.T) {
	Convey("Given a quantity clock with a configured span target", t, func() {
		target := store.NewRetained(core.From(4.0))
		rate := equation.NewRenewalRate(transport.NewApply(target, nil))

		Convey("only completed spans advance the rate and price-change baseline", func() {
			for _, observation := range []struct {
				at                                        int64
				increment, sample, rate, change, maturity float64
				closed                                    bool
			}{
				{0, 2, 100, 0, 0, 0, false},
				{1e9, 2, 100, 4, 0, .5, true},
				{2e9, 1, 105, 4, 0, .5, false},
				{3e9, 3, 110, 2, math.Log(1.1), 2.0 / 3, true},
			} {
				output := tests.Drain(t, rate, transport.NewIO(tests.Record(map[string]any{
					"increment": observation.increment, "sample": observation.sample, "at": observation.at,
				})))
				So(rate.Error(), ShouldBeNil)
				So(output, ShouldHaveLength, 1)
				fields := tests.Fields(t, output[0])
				So(core.To[bool](fields["closed"]), ShouldEqual, observation.closed)
				So(tests.Number(t, fields, "rate"), ShouldAlmostEqual, observation.rate)
				So(tests.Number(t, fields, "change"), ShouldAlmostEqual, observation.change)
				So(tests.Number(t, fields, "maturity"), ShouldAlmostEqual, observation.maturity)
			}
		})

		Convey("a missing quantity cannot reuse the preceding observation", func() {
			tests.Drain(t, rate, transport.NewIO(tests.Record(map[string]any{
				"increment": 2.0, "sample": 100.0, "at": int64(0),
			})))
			output := tests.Drain(t, rate, transport.NewIO(tests.Record(map[string]any{
				"sample": 100.0, "at": int64(1e9),
			})))
			So(output, ShouldBeEmpty)
			So(errors.Is(rate.Error(), core.ErrShape), ShouldBeTrue)
		})

		Convey("an invalid configured target is rejected", func() {
			tests.Drain(t, target, tests.Values(0.0))
			output := tests.Drain(t, rate, transport.NewIO(tests.Record(map[string]any{
				"increment": 2.0, "sample": 100.0, "at": int64(0),
			})))
			So(output, ShouldBeEmpty)
			So(errors.Is(rate.Error(), core.ErrDomain), ShouldBeTrue)
		})
	})
}

func BenchmarkNewRenewalRate(b *testing.B) {
	rate := equation.NewRenewalRate(store.NewConstant(core.From(4.0)))
	var timestamp int64
	b.ReportAllocs()

	for b.Loop() {
		input := transport.NewIO(tests.Record(map[string]any{
			"increment": 2.0, "sample": 100.0, "at": timestamp,
		}))

		if rate.Next(input) == nil || rate.Next(input) != nil {
			b.Fatal("expected one renewal record")
		}

		timestamp += 1e9
	}

	if err := rate.Error(); err != nil {
		b.Fatal(err)
	}
}
